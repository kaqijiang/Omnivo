package service

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/contentarchive"
)

var (
	ErrRequestContentNotConfigured = errors.New("Request content retention is not configured")
	ErrRequestContentUnavailable   = errors.New("Request content storage is unavailable")
	ErrRequestContentSettings      = errors.New("Invalid request content settings")
	requestContentRuntime          atomic.Pointer[requestContentArchiveRuntime]
	requestContentReads            = make(chan struct{}, 4)
)

type requestContentArchiveRuntime struct {
	configured  bool
	ready       bool
	closed      atomic.Bool
	publisher   contentarchive.Publisher
	index       *contentarchive.Index
	reader      *contentarchive.Reader
	stopCapture func(context.Context) error
}

// CurrentRequestContentPolicy is snapshotted once per accepted capture. Changes
// stop new admissions; they do not change retention for an in-flight request.
func CurrentRequestContentPolicy() CapturePolicy {
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	policy := CapturePolicy{Enabled: common.OptionMap["request_content.enabled"] == "true", RetentionDays: 7, MaxBodyBytes: 1 << 20}
	if n, err := strconv.Atoi(common.OptionMap["request_content.retention_days"]); err == nil && n >= 1 && n <= 365 {
		policy.RetentionDays = n
	}
	if n, err := strconv.Atoi(common.OptionMap["request_content.max_body_bytes"]); err == nil && n >= 1024 && n <= 64<<20 {
		policy.MaxBodyBytes = n
	}
	return policy
}

// InitRequestContentArchive initializes only the optional archive clients. The
// caller keeps the gateway running with retention unavailable on configuration
// or storage failure. No customer content is captured until the option is on.
func InitRequestContentArchive() error {
	runtime := &requestContentArchiveRuntime{configured: strings.TrimSpace(os.Getenv("CONTENT_ARCHIVE_BROKERS")) != ""}
	requestContentRuntime.Store(runtime)
	cfg, err := contentarchive.ConfigFromEnv()
	if err != nil {
		return ErrRequestContentNotConfigured
	}
	if !cfg.Enabled() {
		return nil
	}
	if cfg.IndexDSN == "" {
		return ErrRequestContentNotConfigured
	}
	store, err := contentarchive.NewS3Store(cfg)
	if err != nil {
		return ErrRequestContentNotConfigured
	}
	index, err := contentarchive.OpenIndex(cfg.IndexDSN)
	if err != nil {
		return ErrRequestContentUnavailable
	}
	publisher, err := contentarchive.NewPublisher(cfg)
	if err != nil {
		_ = index.Close()
		return ErrRequestContentNotConfigured
	}
	stop, err := SetRequestContentCapture(publisher, cfg, CurrentRequestContentPolicy)
	if err != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = publisher.Close(ctx)
		_ = index.Close()
		return ErrRequestContentNotConfigured
	}
	requestContentRuntime.Store(&requestContentArchiveRuntime{
		configured: true, ready: true, publisher: publisher, index: index,
		reader: contentarchive.NewReader(cfg, index, store), stopCapture: stop,
	})
	return nil
}

func CloseRequestContentArchive(ctx context.Context) error {
	runtime := requestContentRuntime.Load()
	if runtime == nil || !runtime.ready || runtime.closed.Swap(true) {
		return nil
	}
	captureErr := runtime.stopCapture(ctx)
	publishErr := runtime.publisher.Close(ctx)
	indexErr := runtime.index.Close()
	return errors.Join(captureErr, publishErr, indexErr)
}

func RequestContentArchiveStatus() map[string]any {
	policy := CurrentRequestContentPolicy()
	status := map[string]any{
		"configured": false, "ready": false,
		"settings": map[string]any{"enabled": policy.Enabled, "retention_days": policy.RetentionDays, "max_body_bytes": policy.MaxBodyBytes},
		"capture":  RequestContentCaptureStats(), "publisher": contentarchive.PublisherStats{},
	}
	if runtime := requestContentRuntime.Load(); runtime != nil {
		status["configured"] = runtime.configured
		status["ready"] = runtime.ready && !runtime.closed.Load()
		if runtime.publisher != nil {
			status["publisher"] = runtime.publisher.Stats()
		}
	}
	return status
}

func ValidateRequestContentOption(ctx context.Context, key, value string) error {
	switch key {
	case "request_content.enabled":
		if value == "false" {
			return nil
		}
		if value != "true" {
			return ErrRequestContentSettings
		}
		runtime := requestContentRuntime.Load()
		if runtime == nil || !runtime.ready || runtime.closed.Load() {
			return ErrRequestContentNotConfigured
		}
		if probe, ok := runtime.publisher.(interface{ Ping(context.Context) error }); ok {
			probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			if err := probe.Ping(probeCtx); err != nil {
				return ErrRequestContentUnavailable
			}
		}
		return nil
	case "request_content.retention_days", "request_content.max_body_bytes":
		value, err := strconv.Atoi(value)
		if err != nil {
			return ErrRequestContentSettings
		}
		if key == "request_content.retention_days" && value >= 1 && value <= 365 {
			return nil
		}
		if key == "request_content.max_body_bytes" && value >= 1024 && value <= 64<<20 {
			return nil
		}
	}
	return ErrRequestContentSettings
}

// AcquireRequestContentRead bounds the entire lifetime of a plaintext page,
// including audit writes and slow HTTP clients. The handler must defer release
// until it has finished writing its response, not just until object I/O ends.
func AcquireRequestContentRead() (release func(), err error) {
	select {
	case requestContentReads <- struct{}{}:
		return func() { <-requestContentReads }, nil
	default:
		return nil, ErrRequestContentUnavailable
	}
}

func ReadRequestContentPage(ctx context.Context, requestID string, after int64, limit int) (*contentarchive.ContentPage, error) {
	runtime := requestContentRuntime.Load()
	if runtime == nil || !runtime.ready || runtime.closed.Load() {
		return nil, ErrRequestContentNotConfigured
	}
	page, err := runtime.reader.ReadPage(ctx, requestID, after, limit)
	if err != nil {
		return nil, ErrRequestContentUnavailable
	}
	return page, nil
}
