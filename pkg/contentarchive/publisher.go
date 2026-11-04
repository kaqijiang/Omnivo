package contentarchive

import (
	"context"
	"errors"
	"hash/fnv"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

type queuedEvent struct {
	event       Event
	reservation int64
}

// KafkaPublisher bounds outstanding plaintext, encoding workspace and encrypted
// records with one reservation, held until delivery completes. Four fixed
// encoders preserve each request's order by deterministic queue assignment.
// Codec state, channel bookkeeping and Kafka connection buffers are separately
// bounded overhead; MaxBufferedBytes is not a limit on the process's total RSS.
type KafkaPublisher struct {
	client    *kgo.Client
	cfg       Config
	queues    []chan queuedEvent
	workers   sync.WaitGroup
	mu        sync.RWMutex
	closing   bool
	closeOnce sync.Once
	done      chan struct{}
	closeErr  error
	ctx       context.Context
	cancel    context.CancelFunc
	bytes     atomic.Int64
	accepted  atomic.Uint64
	delivered atomic.Uint64
	failed    atomic.Uint64
	dropped   atomic.Uint64
}

func NewPublisher(cfg Config) (Publisher, error) {
	opts, err := KafkaOptions(cfg)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	opts = append(opts,
		kgo.WithContext(ctx),
		kgo.DefaultProduceTopic(cfg.Topic),
		kgo.RequiredAcks(kgo.AllISRAcks()),
		kgo.MaxBufferedBytes(int(cfg.MaxBufferedBytes)),
		kgo.MaxBufferedRecords(int(min(cfg.MaxBufferedBytes/(16<<10), 65536))),
		kgo.ProducerBatchMaxBytes(1<<20),
		kgo.ProducerBatchCompression(kgo.NoCompression()),
		kgo.ProducerLinger(cfg.FlushInterval),
		kgo.RecordDeliveryTimeout(15*time.Second),
		kgo.RecordRetries(5),
		kgo.UnknownTopicRetries(3),
	)
	client, err := kgo.NewClient(opts...)
	if err != nil {
		cancel()
		return nil, errors.New("could not initialize content archive Kafka producer")
	}
	cfg.EncryptionKey = slices.Clone(cfg.EncryptionKey)
	cfg.Brokers = slices.Clone(cfg.Brokers)
	p := &KafkaPublisher{client: client, cfg: cfg, done: make(chan struct{}), ctx: ctx, cancel: cancel}
	const encoders = 4
	p.queues = make([]chan queuedEvent, encoders)
	queueCapacity := int(max(1, min(cfg.MaxBufferedBytes/(16<<10)/encoders, 4096)))
	for i := range encoders {
		p.queues[i] = make(chan queuedEvent, queueCapacity)
		queue := p.queues[i]
		p.workers.Go(func() { p.encode(queue) })
	}
	return p, nil
}

func (p *KafkaPublisher) TryPublish(event Event) bool {
	if err := ValidateEvent(event); err != nil || len(event.Data) > p.cfg.ChunkBytes {
		p.dropped.Add(1)
		return false
	}
	// Conservatively cover copied input, JSON/base64, compression output,
	// ciphertext and the record's lifetime. Four encoder workspaces are fixed.
	reservation := int64(len(event.Data)+len(event.RequestID)+len(event.Model)+len(event.Path)+len(event.Reason)+len(event.Outcome))*8 + 16<<10
	for {
		used := p.bytes.Load()
		if reservation > p.cfg.MaxBufferedBytes-used {
			p.dropped.Add(1)
			return false
		}
		if p.bytes.CompareAndSwap(used, used+reservation) {
			break
		}
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.closing {
		p.bytes.Add(-reservation)
		p.dropped.Add(1)
		return false
	}
	event.Data = slices.Clone(event.Data)
	// Metadata can be a substring backed by a much larger request body. Clone
	// it as well so admission owns only the bytes covered by its reservation.
	event.RequestID = strings.Clone(event.RequestID)
	event.Model = strings.Clone(event.Model)
	event.Path = strings.Clone(event.Path)
	event.Reason = strings.Clone(event.Reason)
	event.Outcome = strings.Clone(event.Outcome)
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(event.RequestID))
	queue := p.queues[int(hash.Sum32())%len(p.queues)]
	select {
	case queue <- queuedEvent{event: event, reservation: reservation}:
		p.accepted.Add(1)
		return true
	default:
		p.bytes.Add(-reservation)
		p.dropped.Add(1)
		return false
	}
}

func (p *KafkaPublisher) encode(queue <-chan queuedEvent) {
	for queued := range queue {
		if p.ctx.Err() != nil {
			p.bytes.Add(-queued.reservation)
			p.failed.Add(1)
			continue
		}
		value, err := EncodeEvents([]Event{queued.event}, p.cfg.EncryptionKey)
		if err != nil {
			p.bytes.Add(-queued.reservation)
			p.failed.Add(1)
			continue
		}
		reservation := queued.reservation
		p.client.TryProduce(p.ctx, &kgo.Record{Key: []byte(queued.event.RequestID), Value: value}, func(_ *kgo.Record, err error) {
			p.bytes.Add(-reservation)
			if err != nil {
				p.failed.Add(1)
			} else {
				p.delivered.Add(1)
			}
		})
	}
}

func (p *KafkaPublisher) Stats() PublisherStats {
	return PublisherStats{QueuedBytes: p.bytes.Load(), Accepted: p.accepted.Load(), Delivered: p.delivered.Load(), Failed: p.failed.Load(), Dropped: p.dropped.Load()}
}

// Ping checks broker connectivity only. It does not assert that an archiver or
// object store is healthy, nor that buffered events have already been archived.
func (p *KafkaPublisher) Ping(ctx context.Context) error {
	if err := p.client.Ping(ctx); err != nil {
		return errors.New("content archive Kafka is unavailable")
	}
	return nil
}

// Close stops acceptance immediately and drains only within the caller deadline.
// Timeout cancels pending delivery; counters distinguish those failures from
// messages acknowledged by Kafka. Concurrent callers observe the same shutdown.
func (p *KafkaPublisher) Close(ctx context.Context) error {
	p.closeOnce.Do(func() {
		p.mu.Lock()
		p.closing = true
		for _, queue := range p.queues {
			close(queue)
		}
		p.mu.Unlock()
		go func() {
			stop := context.AfterFunc(ctx, p.cancel)
			defer stop()
			p.workers.Wait()
			p.closeErr = p.client.Flush(ctx)
			p.cancel()
			p.client.Close()
			close(p.done)
		}()
	})
	select {
	case <-p.done:
		return p.closeErr
	case <-ctx.Done():
		return ctx.Err()
	}
}
