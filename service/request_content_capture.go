package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/contentarchive"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

const requestContentCaptureKey = "request_content_capture"
const captureSSEEventLimit = 8 << 10

type CapturePolicy struct {
	Enabled       bool
	RetentionDays int
	MaxBodyBytes  int
}

type CaptureStats struct {
	Active        int64  `json:"active"`
	BufferedBytes int64  `json:"buffered_bytes"`
	Started       uint64 `json:"started"`
	Skipped       uint64 `json:"skipped"`
	Dropped       uint64 `json:"dropped"`
}

type requestContentManager struct {
	mu       sync.Mutex
	captures map[*requestContentCapture]struct{}
	closed   bool
	pub      contentarchive.Publisher
	cfg      contentarchive.Config
	policy   func() CapturePolicy
	stop     chan struct{}
	done     chan struct{}
	bytes    atomic.Int64
	started  atomic.Uint64
	skipped  atomic.Uint64
	dropped  atomic.Uint64
}

var requestContentManagerState atomic.Pointer[requestContentManager]

// SetRequestContentCapture installs a single bounded capture manager. Cleanup
// stops capture before the caller closes the publisher; no request owns a worker.
func SetRequestContentCapture(pub contentarchive.Publisher, cfg contentarchive.Config, policy func() CapturePolicy) (func(context.Context) error, error) {
	if pub == nil || policy == nil || cfg.MaxActive < 1 || cfg.ChunkBytes < 1 || cfg.FlushInterval <= 0 || cfg.MaxBufferedBytes < int64(cfg.ChunkBytes) {
		return nil, errors.New("invalid request content capture configuration")
	}
	m := &requestContentManager{pub: pub, cfg: cfg, policy: policy, captures: make(map[*requestContentCapture]struct{}), stop: make(chan struct{}), done: make(chan struct{})}
	if !requestContentManagerState.CompareAndSwap(nil, m) {
		return nil, errors.New("request content capture is already configured")
	}
	go m.run()
	return m.close, nil
}

func RequestContentCaptureStats() CaptureStats {
	m := requestContentManagerState.Load()
	if m == nil {
		return CaptureStats{}
	}
	m.mu.Lock()
	active := len(m.captures)
	m.mu.Unlock()
	return CaptureStats{Active: int64(active), BufferedBytes: m.bytes.Load(), Started: m.started.Load(), Skipped: m.skipped.Load(), Dropped: m.dropped.Load()}
}

func (m *requestContentManager) reserve(size int) bool {
	for {
		used := m.bytes.Load()
		if int64(size) > m.cfg.MaxBufferedBytes-used {
			return false
		}
		if m.bytes.CompareAndSwap(used, used+int64(size)) {
			return true
		}
	}
}

func (m *requestContentManager) run() {
	defer close(m.done)
	ticker := time.NewTicker(m.cfg.FlushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-m.stop:
			return
		case <-ticker.C:
			m.flush()
		}
	}
}

func (m *requestContentManager) flush() {
	m.mu.Lock()
	captures := make([]*requestContentCapture, 0, len(m.captures))
	for capture := range m.captures {
		captures = append(captures, capture)
	}
	m.mu.Unlock()
	for _, capture := range captures {
		capture.mu.Lock()
		if !capture.closed {
			capture.flushLocked(0)
			capture.flushLocked(1)
		}
		capture.mu.Unlock()
	}
}

func (m *requestContentManager) close(ctx context.Context) error {
	m.mu.Lock()
	if !m.closed {
		m.closed = true
		close(m.stop)
	}
	captures := make([]*requestContentCapture, 0, len(m.captures))
	for capture := range m.captures {
		captures = append(captures, capture)
	}
	m.mu.Unlock()
	for _, capture := range captures {
		capture.finish(0, false, "", "shutdown")
	}
	requestContentManagerState.CompareAndSwap(m, nil)
	select {
	case <-m.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

type requestContentCapture struct {
	mu            sync.Mutex
	manager       *requestContentManager
	meta          contentarchive.Event
	sequence      int64
	limit         int
	buffers       [2][]byte
	retained      [2]int
	closed        bool
	truncated     bool
	incomplete    bool
	failedPublish bool
	reason        string
	readComplete  bool
	readFailed    bool
	writeFailed   bool
	stream        bool
	snapshot      relaycommon.StreamOutcome
	relayFailed   bool
	sse           *captureSSEParser
}

// RequestContentCaptureMiddleware belongs after TokenAuth and before any relay
// middleware that reads the body. Only the three text HTTP routes opt in.
func RequestContentCaptureMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method != http.MethodPost {
			c.Next()
			return
		}
		switch c.Request.URL.Path {
		case "/v1/chat/completions", "/v1/responses", "/v1/messages":
		default:
			c.Next()
			return
		}
		m := requestContentManagerState.Load()
		if m == nil {
			c.Next()
			return
		}
		policy := m.policy()
		if !policy.Enabled || policy.RetentionDays < 1 || policy.RetentionDays > 365 || policy.MaxBodyBytes < 1 {
			c.Next()
			return
		}
		id := c.GetString(common.RequestIdKey)
		userID := common.GetContextKeyInt(c, constant.ContextKeyUserId)
		if id == "" || len(id) > 64 || userID <= 0 {
			m.skipped.Add(1)
			c.Next()
			return
		}
		now := time.Now()
		capture := &requestContentCapture{
			manager: m, limit: policy.MaxBodyBytes,
			meta:         contentarchive.Event{Version: contentarchive.EventVersion, RequestID: id, UserID: userID, TokenID: common.GetContextKeyInt(c, constant.ContextKeyTokenId), Path: c.Request.URL.Path, CreatedAt: now.Unix(), ExpiresAt: now.Add(time.Duration(policy.RetentionDays) * 24 * time.Hour).Unix()},
			readComplete: c.Request.Body == nil || c.Request.Body == http.NoBody,
		}
		m.mu.Lock()
		if m.closed || len(m.captures) >= m.cfg.MaxActive {
			m.mu.Unlock()
			m.skipped.Add(1)
			c.Next()
			return
		}
		m.captures[capture] = struct{}{}
		m.mu.Unlock()
		capture.mu.Lock()
		accepted := capture.publishLocked(contentarchive.EventStart, "", nil)
		capture.mu.Unlock()
		if !accepted {
			capture.finish(0, false, "", "queue_full")
			c.Next()
			return
		}
		m.started.Add(1)
		c.Set(requestContentCaptureKey, capture)
		if c.Request.Body != nil {
			c.Request.Body = &captureRequestReader{ReadCloser: c.Request.Body, capture: capture, expected: c.Request.ContentLength}
		}
		writer := &captureResponseWriter{ResponseWriter: c.Writer, capture: capture}
		c.Writer = writer
		defer func() {
			recovered := recover()
			writer.mu.Lock()
			status := writer.ResponseWriter.Status()
			stream := strings.HasPrefix(writer.ResponseWriter.Header().Get("Content-Type"), "text/event-stream")
			writer.mu.Unlock()
			reason := ""
			if recovered != nil {
				reason = "handler_panic"
			}
			capture.finish(status, stream, common.GetContextKeyString(c, constant.ContextKeyOriginalModel), reason)
			if recovered != nil {
				panic(recovered)
			}
		}()
		c.Next()
	}
}

// SetRequestContentRelayResult copies classification facts after relay goroutines
// have stopped. The manager never retains RelayInfo or Gin context.
func SetRequestContentRelayResult(c *gin.Context, model string, snapshot relaycommon.StreamOutcome, failed bool) {
	value, ok := c.Get(requestContentCaptureKey)
	if !ok {
		return
	}
	capture, ok := value.(*requestContentCapture)
	if !ok {
		return
	}
	capture.mu.Lock()
	defer capture.mu.Unlock()
	if !capture.closed {
		capture.meta.Model = retainedModelName(model)
		capture.snapshot = snapshot
		capture.relayFailed = failed
	}
}

func (capture *requestContentCapture) publishLocked(kind, direction string, data []byte) bool {
	event := capture.meta
	event.Kind, event.Direction, event.Sequence, event.Data = kind, direction, capture.sequence, data
	capture.sequence++
	if !capture.manager.pub.TryPublish(event) {
		capture.incomplete, capture.failedPublish = true, true
		capture.reason = "queue_full"
		capture.manager.dropped.Add(1)
		return false
	}
	return true
}

func (capture *requestContentCapture) flushLocked(direction int) {
	data := capture.buffers[direction]
	if len(data) == 0 {
		return
	}
	name := contentarchive.DirectionRequest
	if direction == 1 {
		name = contentarchive.DirectionResponse
	}
	capture.publishLocked(contentarchive.EventData, name, data)
	clear(data)
	capture.buffers[direction] = data[:0]
}

func (capture *requestContentCapture) appendLocked(direction int, data []byte) {
	if capture.closed || len(data) == 0 || capture.failedPublish {
		return
	}
	keep := min(len(data), capture.limit-capture.retained[direction])
	if keep < len(data) {
		capture.truncated = true
		if capture.reason == "" {
			capture.reason = "body_limit"
		}
	}
	data = data[:keep]
	if len(data) == 0 {
		return
	}
	if capture.buffers[direction] == nil {
		capacity := min(capture.manager.cfg.ChunkBytes, capture.limit)
		if !capture.manager.reserve(capacity) {
			capture.incomplete, capture.failedPublish = true, true
			capture.reason = "buffer_full"
			capture.manager.dropped.Add(1)
			return
		}
		capture.buffers[direction] = make([]byte, 0, capacity)
	}
	for len(data) > 0 && !capture.failedPublish {
		buffer := capture.buffers[direction]
		n := min(cap(buffer)-len(buffer), len(data))
		capture.buffers[direction] = append(buffer, data[:n]...)
		capture.retained[direction] += n
		data = data[n:]
		if len(capture.buffers[direction]) == cap(buffer) {
			capture.flushLocked(direction)
		}
	}
}

func (capture *requestContentCapture) finish(status int, stream bool, model, reason string) {
	capture.mu.Lock()
	if capture.closed {
		capture.mu.Unlock()
		return
	}
	capture.flushLocked(0)
	capture.flushLocked(1)
	capture.closed = true
	capture.stream = capture.stream || stream
	if model != "" {
		capture.meta.Model = retainedModelName(model)
	}
	if reason != "" {
		capture.reason, capture.incomplete = reason, true
	}
	if !capture.readComplete || capture.readFailed || capture.writeFailed {
		capture.incomplete = true
	}
	outcome := "completed"
	if capture.stream {
		outcome = "unknown"
		if capture.sse != nil && capture.sse.outcome != "" {
			outcome = capture.sse.outcome
		}
		if capture.snapshot.Response != "" {
			outcome = string(capture.snapshot.Response)
		}
		if outcome == "unknown" {
			switch capture.snapshot.EndReason {
			case relaycommon.StreamEndReasonClientGone:
				outcome = "cancelled"
			case relaycommon.StreamEndReasonTimeout, relaycommon.StreamEndReasonScannerErr, relaycommon.StreamEndReasonPanic, relaycommon.StreamEndReasonPingFail:
				outcome = "incomplete"
			}
		}
	}
	if status >= http.StatusBadRequest || capture.relayFailed || capture.writeFailed || capture.readFailed || (capture.sse != nil && capture.sse.outcome == "failed") {
		outcome = "failed"
	}
	if reason == "shutdown" || reason == "handler_panic" {
		outcome = "unknown"
	}
	capture.meta.HTTPStatus, capture.meta.Stream, capture.meta.Outcome = status, capture.stream, outcome
	capture.meta.Truncated, capture.meta.Incomplete, capture.meta.Reason = capture.truncated, capture.incomplete, capture.reason
	capture.publishLocked(contentarchive.EventEnd, "", nil)
	for i, buffer := range capture.buffers {
		clear(buffer)
		capture.manager.bytes.Add(-int64(cap(buffer)))
		capture.buffers[i] = nil
	}
	if capture.sse != nil {
		clear(capture.sse.event)
		capture.manager.bytes.Add(-2 * captureSSEEventLimit)
		capture.sse = nil
	}
	capture.mu.Unlock()
	capture.manager.mu.Lock()
	delete(capture.manager.captures, capture)
	capture.manager.mu.Unlock()
}

type captureRequestReader struct {
	io.ReadCloser
	capture  *requestContentCapture
	expected int64
	read     int64
}

func (reader *captureRequestReader) Read(data []byte) (int, error) {
	n, err := reader.ReadCloser.Read(data)
	reader.read += int64(n)
	reader.capture.mu.Lock()
	reader.capture.appendLocked(0, data[:n])
	if err == io.EOF || (reader.expected > 0 && reader.read >= reader.expected) {
		reader.capture.readComplete = true
		reader.capture.flushLocked(0)
		buffer := reader.capture.buffers[0]
		reader.capture.manager.bytes.Add(-int64(cap(buffer)))
		reader.capture.buffers[0] = nil
	}
	if err != nil && err != io.EOF {
		reader.capture.readFailed = true
		reader.capture.reason = "request_read_error"
	}
	reader.capture.mu.Unlock()
	return n, err
}

type captureResponseWriter struct {
	gin.ResponseWriter
	mu      sync.Mutex
	capture *requestContentCapture
}

func (writer *captureResponseWriter) Unwrap() http.ResponseWriter { return writer.ResponseWriter }

func (writer *captureResponseWriter) Write(data []byte) (int, error) {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	n, err := writer.ResponseWriter.Write(data)
	writer.record(data[:n], err != nil || n < len(data))
	return n, err
}

func (writer *captureResponseWriter) record(data []byte, failed bool) {
	capture := writer.capture
	capture.mu.Lock()
	defer capture.mu.Unlock()
	if capture.closed {
		return
	}
	capture.appendLocked(1, data)
	stream := strings.HasPrefix(writer.ResponseWriter.Header().Get("Content-Type"), "text/event-stream")
	if stream {
		if !capture.stream && capture.manager.reserve(2*captureSSEEventLimit) {
			capture.sse = &captureSSEParser{event: make([]byte, 0, captureSSEEventLimit)}
		}
		capture.stream = true
		capture.meta.Stream = true
		if capture.sse != nil {
			capture.sse.write(data)
		}
	}
	if failed {
		capture.writeFailed = true
		capture.reason = "response_write_error"
	}
}

func (writer *captureResponseWriter) WriteString(data string) (int, error) {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	n, err := writer.ResponseWriter.WriteString(data)
	// Do not materialize another full copy of a large non-streaming response.
	for offset := 0; offset < n; {
		end := min(offset+writer.capture.manager.cfg.ChunkBytes, n)
		writer.record([]byte(data[offset:end]), false)
		offset = end
	}
	if err != nil || n < len(data) {
		writer.record(nil, true)
	}
	return n, err
}
func (writer *captureResponseWriter) WriteHeader(status int) {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	writer.ResponseWriter.WriteHeader(status)
}
func (writer *captureResponseWriter) WriteHeaderNow() {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	writer.ResponseWriter.WriteHeaderNow()
}
func (writer *captureResponseWriter) Flush() {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	writer.ResponseWriter.Flush()
}

func retainedModelName(model string) string {
	return strings.ReplaceAll(strings.ToValidUTF8(model[:min(len(model), 256)], ""), "\x00", "")
}

// captureSSEParser recognizes complete event envelopes, not substrings in model
// text. Large events are skipped; relay's outcome snapshot remains authoritative.
type captureSSEParser struct {
	event    []byte
	overflow bool
	lineSize int
	outcome  string
}

func (parser *captureSSEParser) write(data []byte) {
	for _, b := range data {
		if b != '\r' && b != '\n' {
			parser.lineSize++
		}
		if !parser.overflow {
			if len(parser.event) < captureSSEEventLimit {
				parser.event = append(parser.event, b)
			} else {
				parser.overflow = true
			}
		}
		if b != '\n' {
			continue
		}
		if parser.lineSize == 0 {
			if !parser.overflow {
				parser.finishEvent()
			}
			clear(parser.event)
			parser.event = parser.event[:0]
			parser.overflow = false
		}
		parser.lineSize = 0
	}
}

func (parser *captureSSEParser) finishEvent() {
	var data []byte
	for line := range bytes.SplitSeq(parser.event, []byte{'\n'}) {
		line = bytes.TrimSuffix(line, []byte{'\r'})
		if value, ok := bytes.CutPrefix(line, []byte("data:")); ok {
			value = bytes.TrimPrefix(value, []byte{' '})
			if data != nil {
				data = append(data, '\n')
			}
			data = append(data, value...)
		}
	}
	if bytes.Equal(bytes.TrimSpace(data), []byte("[DONE]")) {
		if parser.outcome == "" {
			parser.outcome = "completed"
		}
		return
	}
	var envelope struct {
		Type  string            `json:"type"`
		Error common.RawMessage `json:"error"`
	}
	if common.Unmarshal(data, &envelope) != nil {
		return
	}
	if envelope.Type == "error" || (len(envelope.Error) > 0 && !bytes.Equal(bytes.TrimSpace(envelope.Error), []byte("null"))) || envelope.Type == "response.failed" {
		parser.outcome = "failed"
		return
	}
	if parser.outcome != "" {
		return
	}
	switch envelope.Type {
	case "response.completed", "message_stop":
		parser.outcome = "completed"
	case "response.incomplete":
		parser.outcome = "incomplete"
	case "response.cancelled", "response.canceled":
		parser.outcome = "cancelled"
	}
}
