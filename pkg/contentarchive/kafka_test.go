package contentarchive

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"net"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"
)

func archiveTestConfig() Config {
	return Config{
		Brokers: []string{"127.0.0.1:9092"}, Topic: "omnivo.request-content.test", Group: "omnivo-test",
		EncryptionKey: bytes.Repeat([]byte{0x37}, 32), S3Region: "us-east-1", MaxBufferedBytes: 1 << 20,
		MaxActive: 8, ChunkBytes: 32 << 10, FlushInterval: 10 * time.Millisecond,
		MaxBatchBytes: 4 << 20, MaxPollRecords: 256,
	}
}

func archiveTestEvent(kind string, seq int64) Event {
	event := Event{Version: EventVersion, RequestID: "capture-test", Kind: kind, Sequence: seq,
		UserID: 42, TokenID: 7, Model: "test-model", Path: "/v1/chat/completions", CreatedAt: 1700000000, ExpiresAt: 1700604800}
	if kind == EventData {
		event.Direction = DirectionResponse
		event.Data = []byte("data: test\n\n")
	}
	return event
}

// Measure the actual allocation cost of encoding the gateway's small SSE flush
// and its configured 32 KiB chunk. This is not an end-to-end throughput claim.
func BenchmarkArchiveCodecBatch(b *testing.B) {
	for _, fixture := range []struct {
		name string
		size int
	}{{"sse_fragment", 256}, {"full_chunk", 32 << 10}} {
		b.Run(fixture.name, func(b *testing.B) {
			event := archiveTestEvent(EventData, 1)
			const line = "data: synthetic model output\n\n"
			event.Data = bytes.Repeat([]byte(line), fixture.size/len(line)+1)[:fixture.size]
			events := []Event{event}
			key := archiveTestConfig().EncryptionKey
			b.ReportAllocs()
			b.SetBytes(int64(len(event.Data)))
			for b.Loop() {
				encoded, err := EncodeEvents(events, key)
				if err != nil || len(encoded) == 0 {
					b.Fatal("encoding failed", err)
				}
			}
		})
	}
}

func TestArchiveCodecAuthenticatedRoundTrip(t *testing.T) {
	key := archiveTestConfig().EncryptionKey
	events := []Event{archiveTestEvent(EventStart, 0), archiveTestEvent(EventData, 1), archiveTestEvent(EventEnd, 2)}
	events[1].Data = []byte{0, 1, 255, '\n', 'x'}
	encoded, err := EncodeEvents(events, key)
	require.NoError(t, err)
	decoded, err := DecodeEvents(encoded, key, 1<<20)
	require.NoError(t, err)
	assert.Equal(t, events, decoded)
	assert.NotContains(t, string(encoded), "test-model")
	other, err := EncodeEvents(events, key)
	require.NoError(t, err)
	assert.NotEqual(t, encoded, other, "every envelope requires a fresh nonce")

	// Reusing a compression workspace must not carry one request's content into
	// a later request. The complete decoded envelope must match only its input.
	next := archiveTestEvent(EventData, 1)
	next.RequestID = "another-request"
	next.Data = []byte("a separate synthetic payload")
	nextEncoded, err := EncodeEvents([]Event{next}, key)
	require.NoError(t, err)
	nextDecoded, err := DecodeEvents(nextEncoded, key, 1<<20)
	require.NoError(t, err)
	assert.Equal(t, []Event{next}, nextDecoded)

	for _, test := range []struct {
		name  string
		value []byte
		key   []byte
		limit int64
	}{
		{"wrong-key", encoded, bytes.Repeat([]byte{1}, 32), 1 << 20},
		{"truncated", encoded[:20], key, 1 << 20},
		{"small-plaintext-budget", encoded, key, 32},
		{"invalid-budget", encoded, key, MaxPlainBytes + 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := DecodeEvents(test.value, test.key, test.limit)
			require.Error(t, err)
			assert.NotContains(t, err.Error(), "test-model")
		})
	}
	tampered := slices.Clone(encoded)
	tampered[len(tampered)-1] ^= 1
	_, err = DecodeEvents(tampered, key, 1<<20)
	require.Error(t, err)
	_, err = EncodeEvents(nil, key)
	require.Error(t, err)
	events[1].Version = 2
	_, err = EncodeEvents(events, key)
	require.Error(t, err)
}

func TestArchiveCodecBoundsDecompressionAndEvents(t *testing.T) {
	key := archiveTestConfig().EncryptionKey
	event := archiveTestEvent(EventData, 1)
	event.Data = bytes.Repeat([]byte("x"), 256<<10)
	encoded, err := EncodeEvents([]Event{event}, key)
	require.NoError(t, err)
	require.Less(t, len(encoded), 16<<10)
	_, err = DecodeEvents(encoded, key, 32<<10)
	require.Error(t, err, "highly compressible input must still honor decoded-size limits")
	for _, change := range []func(*Event){
		func(e *Event) { e.RequestID = "" },
		func(e *Event) { e.Sequence = -1 },
		func(e *Event) { e.Direction = "headers" },
		func(e *Event) { e.ExpiresAt = e.CreatedAt },
		func(e *Event) { e.Data = make([]byte, (256<<10)+1) },
	} {
		changed := event
		change(&changed)
		require.Error(t, ValidateEvent(changed))
	}

	// Construct an authenticated envelope that the encoder would reject. Its
	// plaintext is small, but decoding it into []Event before counting would
	// allocate an unnecessarily large struct slice.
	plain := []byte("[" + strings.Repeat("{},", maxCodecEvents) + "{}]")
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	_, err = writer.Write(plain)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	block, err := aes.NewCipher(key)
	require.NoError(t, err)
	aead, err := cipher.NewGCM(block)
	require.NoError(t, err)
	nonce := make([]byte, aead.NonceSize()) // Deterministic synthetic test fixture.
	malformed := append([]byte(codecHeader), nonce...)
	malformed = aead.Seal(malformed, nonce, compressed.Bytes(), []byte(codecHeader))
	_, err = DecodeEvents(malformed, key, MaxPlainBytes)
	require.ErrorContains(t, err, "event count")
}

func TestArchiveConfigValidation(t *testing.T) {
	for _, name := range []string{"BROKERS", "KEY", "TOPIC", "GROUP", "INDEX_DSN", "S3_ENDPOINT", "S3_REGION", "S3_BUCKET", "S3_ACCESS_KEY", "S3_SECRET_KEY", "S3_PATH_STYLE", "KAFKA_TLS", "KAFKA_CA_FILE", "SASL_MECHANISM", "SASL_USERNAME", "SASL_PASSWORD", "MAX_BUFFERED_BYTES", "MAX_ACTIVE", "CHUNK_BYTES", "FLUSH_INTERVAL_MS", "MAX_BATCH_BYTES", "MAX_POLL_RECORDS"} {
		name = "CONTENT_ARCHIVE_" + name
		old, present := os.LookupEnv(name)
		require.NoError(t, os.Unsetenv(name))
		t.Cleanup(func() {
			if present {
				_ = os.Setenv(name, old)
			} else {
				_ = os.Unsetenv(name)
			}
		})
	}
	cfg, err := ConfigFromEnv()
	require.NoError(t, err)
	assert.False(t, cfg.Enabled())
	t.Setenv("CONTENT_ARCHIVE_BROKERS", "127.0.0.1:9092")
	_, err = ConfigFromEnv()
	require.Error(t, err)
	t.Setenv("CONTENT_ARCHIVE_KEY", base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x37}, 32)))
	cfg, err = ConfigFromEnv()
	require.NoError(t, err)
	assert.True(t, cfg.Enabled())
	assert.Equal(t, 32<<10, cfg.ChunkBytes)
	for _, test := range []struct{ name, value string }{
		{"CONTENT_ARCHIVE_CHUNK_BYTES", "0"},
		{"CONTENT_ARCHIVE_MAX_BUFFERED_BYTES", "-1"},
		{"CONTENT_ARCHIVE_FLUSH_INTERVAL_MS", "9223372036854775807"},
		{"CONTENT_ARCHIVE_KAFKA_TLS", "perhaps"},
		{"CONTENT_ARCHIVE_TOPIC", "../invalid/topic"},
		{"CONTENT_ARCHIVE_BROKERS", "https://secret:password@host:9092"},
		{"CONTENT_ARCHIVE_KEY", "secret-not-a-key"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(test.name, test.value)
			_, err := ConfigFromEnv()
			require.Error(t, err)
			assert.NotContains(t, err.Error(), "secret")
		})
	}
}

func TestArchivePublisherAdmissionOwnsBytesAndIsBounded(t *testing.T) {
	cfg := archiveTestConfig()
	// Hold the encoder queue without running it to deterministically simulate a
	// stalled downstream. Admission must remain bounded and own accepted bytes.
	publisher := &KafkaPublisher{cfg: cfg, queues: []chan queuedEvent{make(chan queuedEvent, 100)}}
	event := archiveTestEvent(EventData, 1)
	event.Data = bytes.Repeat([]byte("x"), cfg.ChunkBytes)
	require.True(t, publisher.TryPublish(event))
	event.Data[0] = 'z'
	queued := <-publisher.queues[0]
	assert.Equal(t, byte('x'), queued.event.Data[0])
	// The first reservation remains charged while the encoder owns this item.
	accepted := uint64(1)
	for publisher.TryPublish(event) {
		accepted++
	}
	stats := publisher.Stats()
	assert.Equal(t, accepted, stats.Accepted)
	assert.Equal(t, uint64(1), stats.Dropped)
	assert.LessOrEqual(t, stats.QueuedBytes, cfg.MaxBufferedBytes)
	assert.Greater(t, stats.QueuedBytes, int64(0))
	publisher.closing = true
	before := publisher.Stats().QueuedBytes
	assert.False(t, publisher.TryPublish(archiveTestEvent(EventStart, 0)))
	assert.Equal(t, before, publisher.Stats().QueuedBytes)
}

func TestArchivePublisherUnavailableCloseReleasesReservations(t *testing.T) {
	// An accepting socket that never speaks Kafka simulates a network blackhole
	// without interrupting the shared integration broker or relying on sleeps.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	connected := make(chan net.Conn, 1)
	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		conn, err := listener.Accept()
		if err == nil {
			connected <- conn
		}
	}()
	cfg := archiveTestConfig()
	cfg.Brokers = []string{listener.Addr().String()}
	pub, err := NewPublisher(cfg)
	require.NoError(t, err)
	publisher := pub.(*KafkaPublisher)
	require.True(t, publisher.TryPublish(archiveTestEvent(EventStart, 0)))
	require.True(t, publisher.TryPublish(archiveTestEvent(EventData, 1)))
	select {
	case conn := <-connected:
		t.Cleanup(func() { _ = conn.Close() })
	case <-time.After(5 * time.Second):
		t.Fatal("producer never attempted its broker connection")
	}
	<-serverDone
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.Error(t, publisher.Close(ctx))
	select {
	case <-publisher.done:
	case <-time.After(5 * time.Second):
		t.Fatal("producer did not terminate after cancelled shutdown")
	}
	stats := publisher.Stats()
	assert.Zero(t, stats.QueuedBytes)
	assert.Equal(t, stats.Accepted, stats.Failed)
	assert.Zero(t, stats.Delivered)
	assert.False(t, publisher.TryPublish(archiveTestEvent(EventEnd, 2)))
}

func TestArchivePublisherKafkaIntegration(t *testing.T) {
	brokers := os.Getenv("CONTENT_ARCHIVE_TEST_BROKERS")
	if brokers == "" {
		t.Skip("CONTENT_ARCHIVE_TEST_BROKERS is not configured")
	}
	cfg := archiveTestConfig()
	cfg.Brokers = strings.Split(brokers, ",")
	cfg.Topic = "omnivo-capture-test-" + time.Now().UTC().Format("20060102150405.000000000")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	client, err := kgo.NewClient(kgo.SeedBrokers(cfg.Brokers...))
	require.NoError(t, err)
	t.Cleanup(client.Close)
	create := kmsg.NewPtrCreateTopicsRequest()
	topic := kmsg.NewCreateTopicsRequestTopic()
	topic.Topic, topic.NumPartitions, topic.ReplicationFactor = cfg.Topic, 1, 1
	create.Topics = append(create.Topics, topic)
	result, err := client.Request(ctx, create)
	require.NoError(t, err)
	require.Zero(t, result.(*kmsg.CreateTopicsResponse).Topics[0].ErrorCode)
	t.Cleanup(func() {
		deleteCtx, deleteCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer deleteCancel()
		deleteRequest := kmsg.NewPtrDeleteTopicsRequest()
		deleteRequest.TopicNames = []string{cfg.Topic}
		_, _ = client.Request(deleteCtx, deleteRequest)
	})
	publisher, err := NewPublisher(cfg)
	require.NoError(t, err)
	require.NoError(t, publisher.(*KafkaPublisher).Ping(ctx))
	events := []Event{archiveTestEvent(EventStart, 0), archiveTestEvent(EventData, 1), archiveTestEvent(EventEnd, 2)}
	for _, event := range events {
		require.True(t, publisher.TryPublish(event))
	}
	require.NoError(t, publisher.Close(ctx))
	assert.Equal(t, uint64(3), publisher.Stats().Delivered)
	assert.Zero(t, publisher.Stats().QueuedBytes)
	assert.False(t, publisher.TryPublish(events[0]))
	consumer, err := kgo.NewClient(kgo.SeedBrokers(cfg.Brokers...), kgo.ConsumePartitions(map[string]map[int32]kgo.Offset{cfg.Topic: {0: kgo.NewOffset().AtStart()}}))
	require.NoError(t, err)
	defer consumer.Close()
	var received []Event
	for len(received) < len(events) {
		fetches := consumer.PollRecords(ctx, 10)
		require.Empty(t, fetches.Errors())
		for _, record := range fetches.Records() {
			assert.Equal(t, "capture-test", string(record.Key))
			assert.NotContains(t, string(record.Value), "data: test")
			decoded, err := DecodeEvents(record.Value, cfg.EncryptionKey, 1<<20)
			require.NoError(t, err)
			received = append(received, decoded...)
		}
	}
	assert.Equal(t, events, received)
}
