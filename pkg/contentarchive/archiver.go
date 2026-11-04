package contentarchive

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/twmb/franz-go/pkg/kgo"
)

// Reads expire immediately. A short physical-deletion grace keeps an already
// bounded in-flight object write from racing expiry cleanup.
const archiveDeletionGrace = 5 * time.Minute

type archiveIndex interface {
	ReserveBatch(context.Context, StoredBatch) error
	SaveBatch(context.Context, StoredBatch) error
	ExpiredBatches(context.Context, int64, int) ([]BatchReference, error)
	DeleteBatch(context.Context, string, int64) error
}

type Archiver struct {
	cfg     Config
	client  *kgo.Client
	index   archiveIndex
	objects ObjectStore
}

// NewArchiver does not migrate the business database or create Kafka topics.
// Provisioning and the separate index connection are explicit deployment steps.
func NewArchiver(cfg Config, index *Index, objects ObjectStore) (*Archiver, error) {
	if index == nil || objects == nil {
		return nil, errors.New("content archive index and object storage are required")
	}
	opts, err := KafkaOptions(cfg)
	if err != nil {
		return nil, err
	}
	opts = append(opts,
		kgo.ConsumerGroup(cfg.Group), kgo.ConsumeTopics(cfg.Topic),
		kgo.DisableAutoCommit(), kgo.BlockRebalanceOnPoll(),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
		kgo.FetchMaxBytes(int32(cfg.MaxBatchBytes)),
		kgo.FetchMaxPartitionBytes(int32(cfg.MaxBatchBytes)),
		kgo.BrokerMaxReadBytes(int32(cfg.MaxBatchBytes*2+1<<20)),
		kgo.MaxConcurrentFetches(1), kgo.FetchMaxWait(time.Second),
	)
	client, err := kgo.NewClient(opts...)
	if err != nil {
		return nil, errors.New("could not initialize content archive consumer")
	}
	return &Archiver{cfg: cfg, client: client, index: index, objects: objects}, nil
}

// Run processes finite batches, not whole requests. An unfinished SSE response
// cannot hold back the durable offset of its already-archived fragments.
func (a *Archiver) Run(ctx context.Context) error {
	defer a.client.AllowRebalance()
	cleanupCtx, stopCleanup := context.WithCancel(ctx)
	var cleanup sync.WaitGroup
	cleanup.Go(func() { a.runCleanup(cleanupCtx) })
	defer func() { stopCleanup(); cleanup.Wait() }()
	for {
		fetches := a.client.PollRecords(ctx, a.cfg.MaxPollRecords)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if len(fetches.Errors()) != 0 {
			a.client.AllowRebalance()
			return errors.New("content archive Kafka fetch failed; offsets were not advanced")
		}
		var failed error
		// Keep processing below the consumer group's poll timeout even when
		// dependencies are slow. Unfinished fetched records remain uncommitted
		// and are replayed after restart; no per-request completion is awaited.
		pollCtx, cancelPoll := context.WithTimeout(ctx, time.Minute)
		fetches.EachPartition(func(partition kgo.FetchTopicPartition) {
			if failed != nil || len(partition.Records) == 0 {
				return
			}
			failed = a.archivePartition(pollCtx, partition.Records)
		})
		cancelPoll()
		a.client.AllowRebalance()
		if failed != nil {
			return failed
		}
	}
}

func (a *Archiver) Close() {
	if a.client != nil {
		a.client.CloseAllowingRebalance()
	}
}

func (a *Archiver) archivePartition(ctx context.Context, records []*kgo.Record) error {
	for len(records) != 0 {
		batchRecords := records[:0]
		var events []Event
		plainBytes := 2
		for _, record := range records {
			if record.Topic != a.cfg.Topic || record.Partition != records[0].Partition || len(record.Value) > a.cfg.MaxBatchBytes {
				return errors.New("invalid content archive Kafka record; offsets were not advanced")
			}
			decoded, err := DecodeEvents(record.Value, a.cfg.EncryptionKey, int64(a.cfg.MaxBatchBytes))
			if err != nil || len(decoded) != 1 || string(record.Key) != decoded[0].RequestID {
				return errors.New("invalid encrypted content archive record; offsets were not advanced")
			}
			encoded, err := common.Marshal(decoded[0])
			if err != nil || len(encoded)+2 > a.cfg.MaxBatchBytes {
				return errors.New("content archive record exceeds the configured batch limit")
			}
			if len(batchRecords) != 0 && plainBytes+len(encoded)+1 > a.cfg.MaxBatchBytes {
				break
			}
			batchRecords = append(batchRecords, record)
			plainBytes += len(encoded) + 1
			if decoded[0].ExpiresAt > time.Now().Unix() {
				events = append(events, decoded[0])
			}
		}
		var persistErr error
		for attempt := range 3 {
			if attempt != 0 {
				timer := time.NewTimer(time.Duration(attempt) * time.Second)
				select {
				case <-ctx.Done():
					timer.Stop()
					return ctx.Err()
				case <-timer.C:
				}
			}
			persistErr = a.persistEvents(ctx, batchRecords, events)
			if persistErr == nil || ctx.Err() != nil {
				break
			}
		}
		if persistErr != nil {
			return persistErr
		}
		commitCtx, cancel := context.WithTimeout(ctx, objectOperationTimeout)
		err := a.client.CommitRecords(commitCtx, batchRecords[len(batchRecords)-1])
		cancel()
		if err != nil {
			return errors.New("content archive offset commit failed; durable records will be replayed")
		}
		records = records[len(batchRecords):]
	}
	return nil
}

func (a *Archiver) persistEvents(ctx context.Context, records []*kgo.Record, events []Event) error {
	// Keep short-retention records out of objects retained by a later policy.
	// A minute bucket limits shared-object expiry skew to less than a minute.
	byExpiry := make(map[int64][]Event)
	now := time.Now().Unix()
	for _, event := range events {
		if event.ExpiresAt > now {
			bucket := event.ExpiresAt / 60
			byExpiry[bucket] = append(byExpiry[bucket], event)
		}
	}
	buckets := make([]int64, 0, len(byExpiry))
	for bucket := range byExpiry {
		buckets = append(buckets, bucket)
	}
	slices.Sort(buckets)
	for _, bucket := range buckets {
		if err := a.persistBatch(ctx, records, byExpiry[bucket]); err != nil {
			return err
		}
	}
	return nil
}

// All expiry buckets in the Kafka batch must finish before archivePartition
// commits its offset. A failure after an earlier bucket is safely replayable.
func (a *Archiver) persistBatch(ctx context.Context, records []*kgo.Record, events []Event) error {
	canonical, err := common.Marshal(events)
	if err != nil {
		return errors.New("could not encode content archive batch")
	}
	digest := sha256.Sum256(canonical)
	topicDigest := sha256.Sum256([]byte(records[0].Topic))
	checksum := hex.EncodeToString(digest[:])
	batch := StoredBatch{
		Key:   fmt.Sprintf("content/v1/%x/%d/%d-%d-%s.bin", topicDigest[:8], records[0].Partition, records[0].Offset, records[len(records)-1].Offset, checksum),
		Topic: records[0].Topic, Partition: records[0].Partition,
		FirstOffset: records[0].Offset, LastOffset: records[len(records)-1].Offset,
		Checksum: checksum, Events: events,
	}
	for _, event := range events {
		batch.ExpiresAt = max(batch.ExpiresAt, event.ExpiresAt)
	}
	encoded, err := EncodeEvents(events, a.cfg.EncryptionKey)
	if err != nil {
		return errors.New("could not encrypt content archive batch")
	}
	// The invisible reservation makes even a failed or interrupted Put eligible
	// for eventual cleanup. No request points to the object before SaveBatch.
	operationCtx, cancel := context.WithTimeout(ctx, objectOperationTimeout)
	err = a.index.ReserveBatch(operationCtx, batch)
	cancel()
	if err != nil {
		return errors.New("content archive batch reservation failed")
	}
	if err := a.objects.Put(ctx, batch.Key, encoded); err != nil {
		return errors.New("content archive object write failed; offsets were not advanced")
	}
	if batch.ExpiresAt <= time.Now().Unix() {
		if err := a.objects.Delete(ctx, batch.Key); err != nil {
			return errors.New("expired content archive object deletion failed")
		}
		operationCtx, cancel = context.WithTimeout(ctx, objectOperationTimeout)
		err = a.index.DeleteBatch(operationCtx, batch.Key, time.Now().Unix())
		cancel()
		return err
	}
	operationCtx, cancel = context.WithTimeout(ctx, objectOperationTimeout)
	err = a.index.SaveBatch(operationCtx, batch)
	cancel()
	if err != nil {
		return errors.New("content archive index write failed; offsets were not advanced")
	}
	return nil
}

func (a *Archiver) runCleanup(ctx context.Context) {
	delay := time.Duration(0)
	failures := 0
	var lastWarning time.Time
	for {
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		found, deleted, err := a.cleanExpired(ctx, time.Now())
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			if time.Since(lastWarning) >= time.Minute {
				common.SysError(common.LogText("Content archive expiry cleanup failed; pending objects will be retried"))
				lastWarning = time.Now()
			}
			if found == 100 && deleted > 0 {
				// One failed object must not prevent draining later pages.
				failures, delay = 0, 250*time.Millisecond
				continue
			}
			failures++
			delay = min(time.Minute, 5*time.Second*time.Duration(1<<min(failures-1, 4)))
			continue
		}
		failures = 0
		delay = time.Minute
		if found == 100 {
			// Backlog is serviced continuously, rather than limiting sustained
			// deletion throughput to one page per minute.
			delay = 100 * time.Millisecond
		}
	}
}

// cleanExpired processes one bounded page with four workers. There is no
// goroutine per object and failed objects remain in the index for later retries.
func (a *Archiver) cleanExpired(ctx context.Context, now time.Time) (int, int, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	batches, err := a.index.ExpiredBatches(ctx, now.Add(-archiveDeletionGrace).Unix(), 100)
	if err != nil {
		return 0, 0, err
	}
	var deleted atomic.Int64
	var failed atomic.Bool
	jobs := make(chan BatchReference)
	var workers sync.WaitGroup
	for range min(4, len(batches)) {
		workers.Go(func() {
			for batch := range jobs {
				if ctx.Err() != nil {
					return
				}
				if err := a.objects.Delete(ctx, batch.Key); err != nil {
					failed.Store(true)
					continue
				}
				if err := a.index.DeleteBatch(ctx, batch.Key, now.Unix()); err != nil {
					failed.Store(true)
					continue
				}
				deleted.Add(1)
			}
		})
	}
enqueue:
	for _, batch := range batches {
		select {
		case <-ctx.Done():
			break enqueue
		case jobs <- batch:
		}
	}
	close(jobs)
	workers.Wait()
	if ctx.Err() != nil {
		return len(batches), int(deleted.Load()), ctx.Err()
	}
	if failed.Load() {
		return len(batches), int(deleted.Load()), errors.New("some content archive expiry deletions failed")
	}
	return len(batches), int(deleted.Load()), nil
}
