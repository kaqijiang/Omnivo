package contentarchive

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kgo"
)

type archiveMemoryObjects struct {
	mu             sync.Mutex
	data           map[string][]byte
	failPut        bool
	failDelete     bool
	failDeleteKeys map[string]bool
	putCount       int
	failAtPut      int
}

func (s *archiveMemoryObjects) Put(_ context.Context, key string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failPut || (s.failAtPut != 0 && s.putCount+1 == s.failAtPut) {
		return errors.New("unavailable")
	}
	s.data[key] = bytes.Clone(data)
	s.putCount++
	return nil
}

func (s *archiveMemoryObjects) Get(_ context.Context, key string, limit int64) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, exists := s.data[key]
	if !exists || int64(len(data)) > limit {
		return nil, errors.New("unavailable")
	}
	return bytes.Clone(data), nil
}

func (s *archiveMemoryObjects) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failDelete || s.failDeleteKeys[key] {
		return errors.New("unavailable")
	}
	delete(s.data, key)
	return nil
}

type archiveFailingIndex struct {
	*Index
	failSave bool
}

func (i *archiveFailingIndex) SaveBatch(ctx context.Context, batch StoredBatch) error {
	if i.failSave {
		return errors.New("unavailable")
	}
	return i.Index.SaveBatch(ctx, batch)
}

type archiveStaticPage struct {
	page ContentPage
	refs []EventReference
}

type archiveBlockingDeletes struct {
	ObjectStore
	entered chan struct{}
	active  atomic.Int32
	maximum atomic.Int32
}

func (s *archiveBlockingDeletes) Delete(ctx context.Context, _ string) error {
	active := s.active.Add(1)
	defer s.active.Add(-1)
	for previous := s.maximum.Load(); active > previous; previous = s.maximum.Load() {
		if s.maximum.CompareAndSwap(previous, active) {
			break
		}
	}
	select {
	case s.entered <- struct{}{}:
	default:
	}
	<-ctx.Done()
	return ctx.Err()
}

func (i archiveStaticPage) ListContent(context.Context, string, int64, int, int64) (ContentPage, []EventReference, error) {
	return i.page, i.refs, nil
}

func TestArchiveDurableBatchesReplayAndExpiry(t *testing.T) {
	ctx := context.Background()
	index, err := OpenIndex("sqlite://" + filepath.Join(t.TempDir(), "archive.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, index.Close()) })
	cfg := Config{Topic: "archive-test", EncryptionKey: bytes.Repeat([]byte{7}, 32), MaxBatchBytes: 4 << 20}
	objects := &archiveMemoryObjects{data: make(map[string][]byte), failPut: true}
	failingIndex := &archiveFailingIndex{Index: index}
	worker := &Archiver{cfg: cfg, index: failingIndex, objects: objects}
	now := time.Now().Unix()
	start := Event{Version: 1, Kind: EventStart, RequestID: "long-stream", UserID: 8, Model: "test-model", CreatedAt: now, ExpiresAt: now + 3600}
	request := start
	request.Kind, request.Sequence, request.Direction, request.Data = EventData, 1, DirectionRequest, []byte(`{"messages":[{"role":"user","content":"hello"}]}`)
	first := []*kgo.Record{{Topic: cfg.Topic, Partition: 0, Offset: 0}, {Topic: cfg.Topic, Partition: 0, Offset: 1}}

	// Failed uploads leave a cleanup reservation but no readable request.
	require.Error(t, worker.persistEvents(ctx, first, []Event{start, request}))
	page, refs, err := index.ListContent(ctx, start.RequestID, -1, 32, now)
	require.NoError(t, err)
	assert.Empty(t, page.RequestID)
	assert.Empty(t, refs)
	reserved, err := index.ExpiredBatches(ctx, now+3601, 100)
	require.NoError(t, err)
	require.Len(t, reserved, 1)

	// An interrupted index write also remains invisible until replay succeeds.
	objects.failPut, failingIndex.failSave = false, true
	require.Error(t, worker.persistEvents(ctx, first, []Event{start, request}))
	require.Len(t, objects.data, 1)
	page, _, err = index.ListContent(ctx, start.RequestID, -1, 32, now)
	require.NoError(t, err)
	assert.Empty(t, page.RequestID)
	failingIndex.failSave = false
	require.NoError(t, worker.persistEvents(ctx, first, []Event{start, request}))
	reader := NewReader(cfg, index, objects)
	partial, err := reader.ReadPage(ctx, start.RequestID, -1, 32)
	require.NoError(t, err)
	require.NotNil(t, partial)
	assert.NotEqual(t, "complete", partial.State)
	require.Len(t, partial.Events, 2, "a long stream must be readable before its end event")
	assert.Equal(t, request.Data, partial.Events[1].Data)

	response := start
	response.Kind, response.Sequence, response.Direction, response.Data = EventData, 2, DirectionResponse, []byte("data: hello\n\ndata: [DONE]\n\n")
	end := start
	end.Kind, end.Sequence, end.Stream, end.HTTPStatus, end.Outcome = EventEnd, 3, true, 200, "completed"
	second := []*kgo.Record{{Topic: cfg.Topic, Partition: 0, Offset: 2}, {Topic: cfg.Topic, Partition: 0, Offset: 3}}
	require.NoError(t, worker.persistEvents(ctx, second, []Event{response, end}))
	// Replay may use different poll boundaries and fresh encryption nonces.
	require.NoError(t, worker.persistEvents(ctx, append(slices.Clone(first), second...), []Event{start, request, response, end}))
	require.NoError(t, worker.persistEvents(ctx, first, []Event{start, request}))
	complete, err := reader.ReadPage(ctx, start.RequestID, -1, 32)
	require.NoError(t, err)
	require.NotNil(t, complete)
	assert.Equal(t, "complete", complete.State)
	require.Len(t, complete.Events, 4)
	assert.Equal(t, response.Data, complete.Events[2].Data)

	// Expired reads stop immediately; physical cleanup can retry independently.
	page, _, err = index.ListContent(ctx, start.RequestID, -1, 32, now+3601)
	require.NoError(t, err)
	assert.Empty(t, page.RequestID)
	objects.failDelete = true
	_, _, err = worker.cleanExpired(ctx, time.Unix(now+3901, 0))
	require.Error(t, err)
	remaining, err := index.ExpiredBatches(ctx, now+3901, 100)
	require.NoError(t, err)
	assert.NotEmpty(t, remaining)
	objects.failDelete = false
	_, _, err = worker.cleanExpired(ctx, time.Unix(now+3901, 0))
	require.NoError(t, err)
	assert.Empty(t, objects.data)
	remaining, err = index.ExpiredBatches(ctx, now+3901, 100)
	require.NoError(t, err)
	assert.Empty(t, remaining)

	// Expired queue replay advances without recreating deleted content.
	expired := start
	expired.CreatedAt, expired.ExpiresAt = now-3600, now-1
	before := objects.putCount
	require.NoError(t, worker.persistEvents(ctx, first[:1], []Event{expired}))
	assert.Equal(t, before, objects.putCount)
}

func TestArchiveExpiryBucketsSurvivePartialFailure(t *testing.T) {
	ctx := context.Background()
	index, err := OpenIndex("sqlite://" + filepath.Join(t.TempDir(), "archive.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, index.Close()) })
	cfg := Config{Topic: "archive-test", EncryptionKey: bytes.Repeat([]byte{6}, 32), MaxBatchBytes: 4 << 20}
	objects := &archiveMemoryObjects{data: make(map[string][]byte), failAtPut: 2}
	worker := &Archiver{cfg: cfg, index: index, objects: objects}
	now := time.Now().Truncate(time.Minute).Unix()
	var events []Event
	for _, policy := range []struct {
		id   string
		days int64
	}{{"short-policy", 7}, {"long-policy", 365}} {
		start := Event{Version: 1, Kind: EventStart, RequestID: policy.id, UserID: 9, CreatedAt: now, ExpiresAt: now + policy.days*86400 + 5}
		body := start
		body.Kind, body.Sequence, body.Direction, body.Data = EventData, 1, DirectionResponse, []byte(policy.id)
		end := start
		end.Kind, end.Sequence, end.HTTPStatus, end.Outcome = EventEnd, 2, 200, "completed"
		events = append(events, start, body, end)
	}
	records := []*kgo.Record{{Topic: cfg.Topic, Partition: 0, Offset: 10}, {Topic: cfg.Topic, Partition: 0, Offset: 15}}
	// The first expiry bucket is durable; the second upload fails. The caller
	// must not acknowledge this Kafka batch and can safely replay both buckets.
	require.Error(t, worker.persistEvents(ctx, records, events))
	reader := NewReader(cfg, index, objects)
	short, err := reader.ReadPage(ctx, "short-policy", -1, 32)
	require.NoError(t, err)
	require.NotNil(t, short)
	assert.Equal(t, "complete", short.State)
	long, err := reader.ReadPage(ctx, "long-policy", -1, 32)
	require.NoError(t, err)
	assert.Nil(t, long)
	objects.failAtPut = 0
	require.NoError(t, worker.persistEvents(ctx, records, events))
	require.Len(t, objects.data, 2, "seven-day and year-long records must use different objects")
	for _, requestID := range []string{"short-policy", "long-policy"} {
		page, err := reader.ReadPage(ctx, requestID, -1, 32)
		require.NoError(t, err)
		require.NotNil(t, page)
		assert.Equal(t, "complete", page.State)
		require.Len(t, page.Events, 3)
	}
	_, _, err = worker.cleanExpired(ctx, time.Unix(now+7*86400+6*60, 0))
	require.NoError(t, err)
	require.Len(t, objects.data, 1, "short-retention content is physically deleted without waiting a year")
	short, err = reader.ReadPage(ctx, "short-policy", -1, 32)
	require.NoError(t, err)
	assert.Nil(t, short)
	long, err = reader.ReadPage(ctx, "long-policy", -1, 32)
	require.NoError(t, err)
	require.NotNil(t, long)
	assert.Equal(t, "complete", long.State)
}

func TestArchiveCleanupDrainsBacklogAndCancels(t *testing.T) {
	for _, scenario := range []struct {
		name  string
		count int
		block bool
	}{{"more than two pages with one failed object", 205, false}, {"cancel four in-flight deletions", 8, true}} {
		t.Run(scenario.name, func(t *testing.T) {
			index, err := OpenIndex("sqlite://" + filepath.Join(t.TempDir(), "cleanup.db"))
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, index.Close()) })
			objects := &archiveMemoryObjects{data: make(map[string][]byte), failDeleteKeys: map[string]bool{"content/v1/cleanup-000.bin": true}}
			now := time.Now().Unix()
			for n := range scenario.count {
				key := fmt.Sprintf("content/v1/cleanup-%03d.bin", n)
				event := Event{Version: 1, Kind: EventStart, RequestID: fmt.Sprintf("cleanup-%d", n), UserID: 8, CreatedAt: now - 3600, ExpiresAt: now - 600}
				require.NoError(t, index.ReserveBatch(context.Background(), StoredBatch{
					Key: key, Topic: "cleanup-test", Partition: 0, FirstOffset: int64(n), LastOffset: int64(n),
					Checksum: archiveDigest(key), ExpiresAt: event.ExpiresAt, Events: []Event{event},
				}))
				objects.data[key] = []byte("expired object")
			}
			worker := &Archiver{index: index, objects: objects}
			blocked := &archiveBlockingDeletes{ObjectStore: objects, entered: make(chan struct{}, 4)}
			if scenario.block {
				worker.objects = blocked
			}
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})
			go func() { defer close(done); worker.runCleanup(ctx) }()
			t.Cleanup(func() {
				cancel()
				select {
				case <-done:
				case <-time.After(3 * time.Second):
					t.Error("cleanup workers did not stop")
				}
			})
			if scenario.block {
				for range 4 {
					select {
					case <-blocked.entered:
					case <-time.After(3 * time.Second):
						t.Fatal("cleanup did not start four bounded workers")
					}
				}
				cancel()
				select {
				case <-done:
				case <-time.After(3 * time.Second):
					t.Fatal("cancelled object operations kept cleanup running")
				}
				assert.EqualValues(t, 4, blocked.maximum.Load())
				assert.Zero(t, blocked.active.Load())
				return
			}
			// More than 100 entries must drain without waiting another minute.
			// A failed object remains retryable while the other 204 are removed.
			require.Eventually(t, func() bool {
				objects.mu.Lock()
				remainingObjects := len(objects.data)
				objects.mu.Unlock()
				remaining, err := index.ExpiredBatches(context.Background(), now, 100)
				return remainingObjects == 1 && err == nil && len(remaining) == 1
			}, 5*time.Second, 20*time.Millisecond)
			cancel()
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				t.Fatal("cleanup did not cancel its retry delay")
			}
			remaining, err := index.ExpiredBatches(context.Background(), now, 100)
			require.NoError(t, err)
			require.Len(t, remaining, 1)
			assert.Equal(t, "content/v1/cleanup-000.bin", remaining[0].Key)
		})
	}
}

func TestArchiveReaderIsolationIntegrityAndPagination(t *testing.T) {
	ctx := context.Background()
	index, err := OpenIndex("sqlite://" + filepath.Join(t.TempDir(), "archive.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, index.Close()) })
	cfg := Config{Topic: "archive-test", EncryptionKey: bytes.Repeat([]byte{9}, 32), MaxBatchBytes: 16 << 20}
	objects := &archiveMemoryObjects{data: make(map[string][]byte)}
	worker := &Archiver{cfg: cfg, index: index, objects: objects}
	now := time.Now().Unix()
	start := Event{Version: 1, Kind: EventStart, RequestID: "request-a", UserID: 3, CreatedAt: now, ExpiresAt: now + 3600}
	other := start
	other.RequestID, other.UserID = "request-b", 4
	events := []Event{start, other}
	for n := range 5 {
		event := start
		event.Kind, event.Sequence, event.Direction = EventData, int64(n+1), DirectionResponse
		event.Data = bytes.Repeat([]byte{byte('a' + n)}, 256<<10)
		events = append(events, event)
	}
	end := start
	end.Kind, end.Sequence, end.HTTPStatus, end.Outcome = EventEnd, 6, 200, "completed"
	events = append(events, end)
	records := []*kgo.Record{{Topic: cfg.Topic, Partition: 0, Offset: 0}, {Topic: cfg.Topic, Partition: 0, Offset: 7}}
	require.NoError(t, worker.persistEvents(ctx, records, events))
	readerConfig := cfg
	readerConfig.MaxBatchBytes = 256 << 10 // New writes were tuned down after this object was archived.
	reader := NewReader(readerConfig, index, objects)
	page, err := reader.ReadPage(ctx, start.RequestID, -1, 32)
	require.NoError(t, err)
	require.NotNil(t, page)
	require.Len(t, page.Events, 5)
	require.NotNil(t, page.NextSequence)
	assert.EqualValues(t, 4, *page.NextSequence)
	for _, event := range page.Events {
		assert.Equal(t, start.RequestID, event.RequestID)
	}
	next, err := reader.ReadPage(ctx, start.RequestID, *page.NextSequence, 32)
	require.NoError(t, err)
	require.Len(t, next.Events, 2)
	assert.Nil(t, next.NextSequence)

	metadata, refs, err := index.ListContent(ctx, start.RequestID, -1, 1, now)
	require.NoError(t, err)
	require.Len(t, refs, 1)
	ref := refs[0]
	ref.EventIndex = 1 // Correct encrypted object and checksum; wrong tenant event.
	reader.index = archiveStaticPage{page: metadata, refs: []EventReference{ref}}
	_, err = reader.ReadPage(ctx, start.RequestID, -1, 1)
	assert.ErrorIs(t, err, ErrContentUnavailable)
	reader.index = index
	object := objects.data[refs[0].ObjectKey]
	object[len(object)-1] ^= 1
	_, err = reader.ReadPage(ctx, start.RequestID, -1, 1)
	assert.ErrorIs(t, err, ErrContentUnavailable)
}

func TestArchiveS3BoundedIO(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			_, _ = io.Copy(io.Discard, r.Body)
			w.WriteHeader(http.StatusOK)
		case http.MethodDelete:
			if strings.HasSuffix(r.URL.Path, "missing.bin") {
				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(http.StatusNotFound)
				_, _ = io.WriteString(w, "<Error><Code>NoSuchKey</Code></Error>")
				return
			}
			w.WriteHeader(http.StatusNoContent)
		case http.MethodGet:
			if strings.HasSuffix(r.URL.Path, "unknown.bin") {
				w.WriteHeader(http.StatusOK)
				w.(http.Flusher).Flush()
			} else {
				w.Header().Set("Content-Length", "16")
			}
			_, _ = w.Write([]byte("0123456789abcdef"))
		}
	}))
	t.Cleanup(server.Close)
	store, err := NewS3Store(Config{S3Endpoint: server.URL, S3Bucket: "archive-test", S3Region: "us-east-1", S3AccessKey: "test-access", S3SecretKey: "test-secret", S3PathStyle: true})
	require.NoError(t, err)
	ctx := context.Background()
	require.NoError(t, store.Put(ctx, "content/v1/test.bin", []byte("encrypted")))
	data, err := store.Get(ctx, "content/v1/test.bin", 16)
	require.NoError(t, err)
	assert.Equal(t, []byte("0123456789abcdef"), data)
	for _, key := range []string{"content/v1/test.bin", "content/v1/unknown.bin"} {
		_, err = store.Get(ctx, key, 4)
		assert.Error(t, err)
	}
	require.NoError(t, store.Delete(ctx, "content/v1/test.bin"))
	require.NoError(t, store.Delete(ctx, "content/v1/missing.bin"))
	_, err = store.Get(ctx, "../../another-bucket", 100)
	assert.Error(t, err)
}

// This is opt-in and must use isolated Kafka, S3 and index database resources.
// It exercises actual acknowledgements and object reads, not mocked services.
func TestArchiveKafkaS3Integration(t *testing.T) {
	if os.Getenv("CONTENT_ARCHIVE_INTEGRATION_TEST") != "1" {
		t.Skip("set CONTENT_ARCHIVE_INTEGRATION_TEST=1 with isolated archive services")
	}
	cfg, err := ConfigFromEnv()
	require.NoError(t, err)
	require.True(t, cfg.Enabled())
	cfg.Group += "-it-" + uuid.NewString()
	index, err := OpenIndex(cfg.IndexDSN)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, index.Close()) })
	objects, err := NewS3Store(cfg)
	require.NoError(t, err)
	publisher, err := NewPublisher(cfg)
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		require.NoError(t, publisher.Close(ctx))
	})
	worker, err := NewArchiver(cfg, index, objects)
	require.NoError(t, err)
	workerCtx, stopWorker := context.WithCancel(context.Background())
	workerDone := make(chan error, 1)
	go func() { workerDone <- worker.Run(workerCtx) }()
	t.Cleanup(func() {
		stopWorker()
		select {
		case <-workerDone:
		case <-time.After(20 * time.Second):
			t.Error("archive worker did not stop")
		}
		worker.Close()
	})
	now := time.Now().Unix()
	start := Event{Version: 1, Kind: EventStart, RequestID: "archive-it-" + uuid.NewString(), UserID: 77, CreatedAt: now, ExpiresAt: now + 3600}
	request := start
	request.Kind, request.Sequence, request.Direction, request.Data = EventData, 1, DirectionRequest, []byte(`{"model":"test-model","stream":true}`)
	response := start
	response.Kind, response.Sequence, response.Direction, response.Data = EventData, 2, DirectionResponse, []byte("data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n")
	for _, event := range []Event{start, request, response} {
		require.True(t, publisher.TryPublish(event))
	}
	reader := NewReader(cfg, index, objects)
	var partial *ContentPage
	var workerFailure error
	require.Eventually(t, func() bool {
		select {
		case workerFailure = <-workerDone:
			workerDone <- workerFailure
			return true
		default:
		}
		partial, err = reader.ReadPage(context.Background(), start.RequestID, -1, 32)
		return err == nil && partial != nil && len(partial.Events) == 3
	}, 45*time.Second, 100*time.Millisecond)
	require.NoError(t, workerFailure)
	require.NotNil(t, partial)
	assert.NotEqual(t, "complete", partial.State)
	assert.Equal(t, response.Data, partial.Events[2].Data)
	var lastEvent archiveEvent
	require.NoError(t, index.db.Where("request_key = ? AND sequence = ?", archiveDigest(start.RequestID), 2).Take(&lastEvent).Error)
	var stored archiveBatch
	require.NoError(t, index.db.Where("batch_id = ?", lastEvent.BatchID).Take(&stored).Error)
	require.Eventually(t, func() bool {
		return worker.client.CommittedOffsets()[cfg.Topic][stored.Partition].Offset > stored.LastOffset
	}, 10*time.Second, 50*time.Millisecond, "a stream must advance Kafka offsets before its end event")

	end := start
	end.Kind, end.Sequence, end.HTTPStatus, end.Stream, end.Outcome = EventEnd, 3, 200, true, "completed"
	require.True(t, publisher.TryPublish(end))
	require.Eventually(t, func() bool {
		partial, err = reader.ReadPage(context.Background(), start.RequestID, -1, 32)
		return err == nil && partial != nil && partial.State == "complete" && len(partial.Events) == 4
	}, 20*time.Second, 100*time.Millisecond)

	// A fresh consumer group replays the same events using different poll sizes.
	// The already visible records must remain singular and decryptable.
	replayCfg := cfg
	replayCfg.Group += "-replay"
	replayCfg.MaxPollRecords = 1
	replay, err := NewArchiver(replayCfg, index, objects)
	require.NoError(t, err)
	replayCtx, stopReplay := context.WithCancel(context.Background())
	replayDone := make(chan error, 1)
	go func() { replayDone <- replay.Run(replayCtx) }()
	t.Cleanup(func() {
		stopReplay()
		select {
		case <-replayDone:
		case <-time.After(20 * time.Second):
			t.Error("replay worker did not stop")
		}
		replay.Close()
	})
	lastEvent = archiveEvent{}
	stored = archiveBatch{}
	require.NoError(t, index.db.Where("request_key = ? AND sequence = ?", archiveDigest(start.RequestID), 3).Take(&lastEvent).Error)
	require.NoError(t, index.db.Where("batch_id = ?", lastEvent.BatchID).Take(&stored).Error)
	require.Eventually(t, func() bool {
		return replay.client.CommittedOffsets()[cfg.Topic][stored.Partition].Offset > stored.LastOffset
	}, 30*time.Second, 100*time.Millisecond)
	var count int64
	require.NoError(t, index.db.Model(&archiveEvent{}).Where("request_key = ?", archiveDigest(start.RequestID)).Count(&count).Error)
	assert.EqualValues(t, 4, count)
	page, err := reader.ReadPage(context.Background(), start.RequestID, -1, 32)
	require.NoError(t, err)
	require.NotNil(t, page)
	assert.Equal(t, "complete", page.State)
	require.Len(t, page.Events, 4)
	assert.Equal(t, response.Data, page.Events[2].Data)
}
