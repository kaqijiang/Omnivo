// Package contentarchive implements bounded, asynchronous request content retention.
package contentarchive

import (
	"context"
	"time"
)

const (
	EventVersion      = 1
	EventStart        = "start"
	EventData         = "data"
	EventEnd          = "end"
	DirectionRequest  = "request"
	DirectionResponse = "response"
)

// Event contains application HTTP entity bytes, never authentication headers.
// Sequence increases across all events of one capture, starting at zero.
type Event struct {
	Version    int    `json:"version"`
	RequestID  string `json:"request_id"`
	Kind       string `json:"kind"`
	Sequence   int64  `json:"sequence"`
	Direction  string `json:"direction,omitempty"`
	Data       []byte `json:"data,omitempty"`
	UserID     int    `json:"user_id"`
	TokenID    int    `json:"token_id,omitempty"`
	Model      string `json:"model,omitempty"`
	Path       string `json:"path,omitempty"`
	CreatedAt  int64  `json:"created_at"`
	ExpiresAt  int64  `json:"expires_at"`
	HTTPStatus int    `json:"http_status,omitempty"`
	Stream     bool   `json:"stream,omitempty"`
	Outcome    string `json:"outcome,omitempty"`
	Truncated  bool   `json:"truncated,omitempty"`
	Incomplete bool   `json:"incomplete,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

type PublisherStats struct {
	QueuedBytes int64  `json:"queued_bytes"`
	Accepted    uint64 `json:"accepted"`
	Delivered   uint64 `json:"delivered"`
	Failed      uint64 `json:"failed"`
	Dropped     uint64 `json:"dropped"`
}

type Publisher interface {
	// TryPublish must not wait for network I/O. It owns a copy when accepted.
	TryPublish(Event) bool
	Stats() PublisherStats
	Close(context.Context) error
}

type Config struct {
	Brokers          []string
	Topic            string
	Group            string
	EncryptionKey    []byte
	IndexDSN         string
	S3Endpoint       string
	S3Region         string
	S3Bucket         string
	S3AccessKey      string
	S3SecretKey      string
	S3PathStyle      bool
	KafkaTLS         bool
	KafkaCAFile      string
	SASLMechanism    string
	SASLUsername     string
	SASLPassword     string
	MaxBufferedBytes int64
	MaxActive        int
	ChunkBytes       int
	FlushInterval    time.Duration
	MaxBatchBytes    int
	MaxPollRecords   int
}

// StoredBatch identifies immutable archived Kafka records. All records in a batch
// share one topic and partition; offsets need not be adjacent.
type StoredBatch struct {
	Key         string
	Topic       string
	Partition   int32
	FirstOffset int64
	LastOffset  int64
	Checksum    string
	ExpiresAt   int64
	Events      []Event
}

type ObjectStore interface {
	Put(context.Context, string, []byte) error
	Get(context.Context, string, int64) ([]byte, error)
	Delete(context.Context, string) error
}

// ContentPage returns bounded ordered events. The caller still enforces root
// session authorization. No caller may use a request ID as an access credential.
type ContentPage struct {
	RequestID    string  `json:"request_id"`
	State        string  `json:"state"`
	UserID       int     `json:"user_id"`
	Model        string  `json:"model"`
	CreatedAt    int64   `json:"created_at"`
	ExpiresAt    int64   `json:"expires_at"`
	HTTPStatus   int     `json:"http_status"`
	Stream       bool    `json:"stream"`
	Outcome      string  `json:"outcome"`
	Truncated    bool    `json:"truncated"`
	Incomplete   bool    `json:"incomplete"`
	Events       []Event `json:"events"`
	NextSequence *int64  `json:"next_sequence"`
}
