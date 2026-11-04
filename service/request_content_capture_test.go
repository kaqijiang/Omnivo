package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/contentarchive"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type captureTestPublisher struct {
	mu     sync.Mutex
	events []contentarchive.Event
	notify chan contentarchive.Event
	reject func(contentarchive.Event) bool
}

func (publisher *captureTestPublisher) TryPublish(event contentarchive.Event) bool {
	publisher.mu.Lock()
	defer publisher.mu.Unlock()
	if publisher.reject != nil && publisher.reject(event) {
		return false
	}
	event.Data = bytes.Clone(event.Data)
	publisher.events = append(publisher.events, event)
	select {
	case publisher.notify <- event:
	default:
	}
	return true
}

func (publisher *captureTestPublisher) Stats() contentarchive.PublisherStats {
	return contentarchive.PublisherStats{}
}

func (publisher *captureTestPublisher) Close(context.Context) error { return nil }

func (publisher *captureTestPublisher) snapshot() []contentarchive.Event {
	publisher.mu.Lock()
	defer publisher.mu.Unlock()
	return append([]contentarchive.Event(nil), publisher.events...)
}

func captureFixture(t *testing.T, config contentarchive.Config, policy CapturePolicy) (*captureTestPublisher, func(context.Context) error) {
	t.Helper()
	if config.MaxActive == 0 {
		config.MaxActive = 8
	}
	if config.ChunkBytes == 0 {
		config.ChunkBytes = 32
	}
	if config.FlushInterval == 0 {
		config.FlushInterval = time.Hour
	}
	if config.MaxBufferedBytes == 0 {
		config.MaxBufferedBytes = 256 << 10
	}
	publisher := &captureTestPublisher{notify: make(chan contentarchive.Event, 128)}
	closeCapture, err := SetRequestContentCapture(publisher, config, func() CapturePolicy { return policy })
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, closeCapture(context.Background())) })
	return publisher, closeCapture
}

func captureEngine(handler gin.HandlerFunc) *gin.Engine {
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		requestID := c.GetHeader("X-Test-Request-ID")
		if requestID == "" {
			requestID = "capture-test-001"
		}
		c.Set(common.RequestIdKey, requestID)
		common.SetContextKey(c, constant.ContextKeyUserId, 7)
		common.SetContextKey(c, constant.ContextKeyTokenId, 11)
	})
	engine.Use(RequestContentCaptureMiddleware())
	engine.POST("/v1/chat/completions", handler)
	engine.POST("/v1/responses", handler)
	engine.POST("/v1/messages", handler)
	engine.POST("/v1/embeddings", handler)
	return engine
}

func capturedBody(events []contentarchive.Event, direction string) string {
	var body strings.Builder
	for _, event := range events {
		if event.Kind == contentarchive.EventData && event.Direction == direction {
			body.Write(event.Data)
		}
	}
	return body.String()
}

func TestRequestContentCaptureJSONAndBodyReplay(t *testing.T) {
	publisher, _ := captureFixture(t, contentarchive.Config{ChunkBytes: 8}, CapturePolicy{Enabled: true, RetentionDays: 7, MaxBodyBytes: 1024})
	input := `{"model":"test-model","messages":[{"role":"user","content":"hello"}]}`
	output := `{"choices":[{"message":{"content":"world"}}]}`
	engine := captureEngine(func(c *gin.Context) {
		storage, err := common.GetBodyStorage(c)
		require.NoError(t, err)
		defer common.CleanupBodyStorage(c)
		for range 2 {
			_, err = storage.Seek(0, io.SeekStart)
			require.NoError(t, err)
			body, err := io.ReadAll(storage)
			require.NoError(t, err)
			assert.Equal(t, input, string(body))
		}
		SetRequestContentRelayResult(c, "test-model", relaycommon.StreamOutcome{}, false)
		c.Header("Content-Type", "application/json")
		_, err = c.Writer.WriteString(output)
		require.NoError(t, err)
	})
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions?key=query-secret", strings.NewReader(input))
	request.Header.Set("Authorization", "Bearer credential-secret")
	request.Header.Set("Cookie", "session=cookie-secret")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, output, recorder.Body.String())
	events := publisher.snapshot()
	require.NotEmpty(t, events)
	assert.Equal(t, contentarchive.EventStart, events[0].Kind)
	assert.Equal(t, input, capturedBody(events, contentarchive.DirectionRequest))
	assert.Equal(t, output, capturedBody(events, contentarchive.DirectionResponse))
	for i, event := range events {
		assert.Equal(t, int64(i), event.Sequence)
		assert.Equal(t, events[0].CreatedAt, event.CreatedAt)
		assert.Equal(t, events[0].ExpiresAt, event.ExpiresAt)
		assert.Equal(t, 7, event.UserID)
	}
	end := events[len(events)-1]
	assert.Equal(t, contentarchive.EventEnd, end.Kind)
	assert.Equal(t, "test-model", end.Model)
	assert.Equal(t, "completed", end.Outcome)
	assert.False(t, end.Incomplete)
	assert.False(t, end.Truncated)
	encoded, err := common.Marshal(events)
	require.NoError(t, err)
	for _, secret := range []string{"query-secret", "credential-secret", "cookie-secret"} {
		assert.NotContains(t, string(encoded), secret)
	}
	assert.Zero(t, RequestContentCaptureStats().Active)
	assert.Zero(t, RequestContentCaptureStats().BufferedBytes)
}

func TestRequestContentCaptureSSEBeforeCompletion(t *testing.T) {
	publisher, _ := captureFixture(t, contentarchive.Config{FlushInterval: 5 * time.Millisecond}, CapturePolicy{Enabled: true, RetentionDays: 7, MaxBodyBytes: 1024})
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseHandler := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseHandler()
	engine := captureEngine(func(c *gin.Context) {
		_, _ = io.Copy(io.Discard, c.Request.Body)
		c.Header("Content-Type", "text/event-stream")
		_, _ = c.Writer.WriteString("data: ")
		c.Writer.Flush()
		<-release
		for _, part := range []string{"{\"choices\":[]}\n\n", "data: [DO", "NE]\n\n"} {
			_, _ = c.Writer.WriteString(part)
			c.Writer.Flush()
		}
	})
	server := httptest.NewServer(engine)
	defer server.Close()
	client := server.Client()
	client.Timeout = 3 * time.Second
	response, err := client.Post(server.URL+"/v1/chat/completions", "application/json", strings.NewReader("{}"))
	require.NoError(t, err)
	defer response.Body.Close()
	first := make([]byte, len("data: "))
	_, err = io.ReadFull(response.Body, first)
	require.NoError(t, err)
	assert.Equal(t, "data: ", string(first))
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	for {
		select {
		case event := <-publisher.notify:
			if event.Kind == contentarchive.EventData && event.Direction == contentarchive.DirectionResponse {
				assert.Equal(t, "data: ", string(event.Data))
				for _, captured := range publisher.snapshot() {
					assert.NotEqual(t, contentarchive.EventEnd, captured.Kind)
				}
				releaseHandler()
				rest, readErr := io.ReadAll(response.Body)
				require.NoError(t, readErr)
				assert.Equal(t, "{\"choices\":[]}\n\ndata: [DONE]\n\n", string(rest))
				events := publisher.snapshot()
				end := events[len(events)-1]
				assert.Equal(t, contentarchive.EventEnd, end.Kind)
				assert.Equal(t, "completed", end.Outcome)
				assert.True(t, end.Stream)
				assert.False(t, end.Incomplete)
				return
			}
		case <-deadline.C:
			t.Fatal("response fragment was not queued while the SSE response was still open")
		}
	}
}

func TestRequestContentCaptureSSEOutcomes(t *testing.T) {
	for _, test := range []struct {
		name, path, body, outcome string
	}{
		{"chat terminal", "/v1/chat/completions", "data: [DONE]\r\n\r\n", "completed"},
		{"responses terminal", "/v1/responses", "event: response.completed\ndata: {\"type\":\"response.completed\"}\n\n", "completed"},
		{"messages terminal", "/v1/messages", "data: {\"type\":\"message_stop\"}\n\n", "completed"},
		{"terminal name in model text", "/v1/responses", "data: {\"type\":\"response.output_text.delta\",\"delta\":\"response.completed [DONE] message_stop\"}\n\n", "unknown"},
		{"protocol error then done", "/v1/chat/completions", "data: {\"error\":{\"message\":\"failure\"}}\n\ndata: [DONE]\n\n", "failed"},
		{"incomplete response", "/v1/responses", "data: {\"type\":\"response.incomplete\"}\n\n", "incomplete"},
		{"unfinished event", "/v1/responses", "data: {\"type\":\"response.completed\"}", "unknown"},
	} {
		t.Run(test.name, func(t *testing.T) {
			publisher, _ := captureFixture(t, contentarchive.Config{ChunkBytes: 7}, CapturePolicy{Enabled: true, RetentionDays: 7, MaxBodyBytes: 1024})
			engine := captureEngine(func(c *gin.Context) {
				_, _ = io.Copy(io.Discard, c.Request.Body)
				c.Header("Content-Type", "text/event-stream")
				for _, b := range []byte(test.body) {
					_, err := c.Writer.Write([]byte{b})
					require.NoError(t, err)
				}
			})
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, test.path, strings.NewReader("{}")))
			assert.Equal(t, test.body, recorder.Body.String())
			events := publisher.snapshot()
			assert.Equal(t, test.body, capturedBody(events, contentarchive.DirectionResponse))
			end := events[len(events)-1]
			assert.Equal(t, test.outcome, end.Outcome)
			assert.False(t, end.Incomplete)
		})
	}
}

func TestRequestContentCaptureLimitsAndFailure(t *testing.T) {
	for _, test := range []struct {
		name   string
		limit  int
		reject bool
	}{
		{"body limit", 4, false},
		{"queue full", 1024, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			publisher, _ := captureFixture(t, contentarchive.Config{ChunkBytes: 4}, CapturePolicy{Enabled: true, RetentionDays: 7, MaxBodyBytes: test.limit})
			if test.reject {
				publisher.reject = func(event contentarchive.Event) bool {
					return event.Kind == contentarchive.EventData && event.Direction == contentarchive.DirectionResponse
				}
			}
			engine := captureEngine(func(c *gin.Context) {
				_, _ = io.Copy(io.Discard, c.Request.Body)
				c.Header("Content-Type", "application/json")
				_, _ = c.Writer.WriteString(`{"ok":true}`)
			})
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"test"}`)))
			assert.Equal(t, `{"ok":true}`, recorder.Body.String())
			assert.Equal(t, http.StatusOK, recorder.Code)
			events := publisher.snapshot()
			end := events[len(events)-1]
			assert.Equal(t, "completed", end.Outcome)
			if test.reject {
				assert.True(t, end.Incomplete)
				assert.Equal(t, "queue_full", end.Reason)
				assert.Greater(t, RequestContentCaptureStats().Dropped, uint64(0))
			} else {
				assert.True(t, end.Truncated)
				assert.Equal(t, "body_limit", end.Reason)
				assert.Equal(t, `{"mo`, capturedBody(events, contentarchive.DirectionRequest))
				assert.Equal(t, `{"ok`, capturedBody(events, contentarchive.DirectionResponse))
			}
			assert.Zero(t, RequestContentCaptureStats().BufferedBytes)
		})
	}
}

type capturePartialWriter struct{ *httptest.ResponseRecorder }

func (writer *capturePartialWriter) Write(data []byte) (int, error) {
	n, _ := writer.ResponseRecorder.Write(data[:min(3, len(data))])
	return n, errors.New("synthetic write failure")
}

func TestRequestContentCaptureOnlyWrittenBytesAndSnapshot(t *testing.T) {
	publisher, _ := captureFixture(t, contentarchive.Config{}, CapturePolicy{Enabled: true, RetentionDays: 7, MaxBodyBytes: 1024})
	engine := captureEngine(func(c *gin.Context) {
		_, _ = io.Copy(io.Discard, c.Request.Body)
		_, err := c.Writer.Write([]byte("abcdef"))
		assert.Error(t, err)
		SetRequestContentRelayResult(c, "model", relaycommon.StreamOutcome{Response: relaycommon.ResponseOutcomeCompleted}, false)
	})
	writer := &capturePartialWriter{httptest.NewRecorder()}
	engine.ServeHTTP(writer, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader("{}")))
	events := publisher.snapshot()
	assert.Equal(t, "abc", capturedBody(events, contentarchive.DirectionResponse))
	end := events[len(events)-1]
	assert.Equal(t, "failed", end.Outcome)
	assert.True(t, end.Incomplete)
	assert.Equal(t, "response_write_error", end.Reason)
}

func TestRequestContentCaptureDisabledAndUnsupported(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "disabled", true: "unsupported path"}[enabled], func(t *testing.T) {
			publisher, _ := captureFixture(t, contentarchive.Config{}, CapturePolicy{Enabled: enabled, RetentionDays: 7, MaxBodyBytes: 1024})
			engine := captureEngine(func(c *gin.Context) { c.String(http.StatusOK, "normal response") })
			path := "/v1/chat/completions"
			if enabled {
				path = "/v1/embeddings"
			}
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, path, strings.NewReader("{}")))
			assert.Equal(t, "normal response", recorder.Body.String())
			assert.Empty(t, publisher.snapshot())
			assert.Zero(t, RequestContentCaptureStats().BufferedBytes)
		})
	}
}

func TestRequestContentCaptureActiveLimitAndShutdown(t *testing.T) {
	publisher, closeCapture := captureFixture(t, contentarchive.Config{MaxActive: 1}, CapturePolicy{Enabled: true, RetentionDays: 7, MaxBodyBytes: 1024})
	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})
	engine := captureEngine(func(c *gin.Context) {
		_, _ = io.Copy(io.Discard, c.Request.Body)
		if c.GetHeader("X-Test-Request-ID") == "long-request" {
			_, _ = c.Writer.WriteString("first")
			close(started)
			<-release
			_, _ = c.Writer.WriteString("last")
			return
		}
		c.String(http.StatusOK, "second")
	})
	firstRecorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader("{}"))
	request.Header.Set("X-Test-Request-ID", "long-request")
	go func() { defer close(done); engine.ServeHTTP(firstRecorder, request) }()
	<-started
	secondRecorder := httptest.NewRecorder()
	engine.ServeHTTP(secondRecorder, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader("{}")))
	assert.Equal(t, "second", secondRecorder.Body.String())
	assert.Equal(t, uint64(1), RequestContentCaptureStats().Skipped)
	require.NoError(t, closeCapture(context.Background()))
	close(release)
	<-done
	assert.Equal(t, "firstlast", firstRecorder.Body.String())
	events := publisher.snapshot()
	end := events[len(events)-1]
	assert.Equal(t, contentarchive.EventEnd, end.Kind)
	assert.Equal(t, "shutdown", end.Reason)
	assert.Equal(t, "unknown", end.Outcome)
	assert.True(t, end.Incomplete)
	assert.Equal(t, "first", capturedBody(events, contentarchive.DirectionResponse))
}

func TestRequestContentCaptureGlobalBufferBudget(t *testing.T) {
	publisher, _ := captureFixture(t, contentarchive.Config{ChunkBytes: 32, MaxBufferedBytes: 32, MaxActive: 2}, CapturePolicy{Enabled: true, RetentionDays: 7, MaxBodyBytes: 1024})
	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})
	var releaseOnce sync.Once
	releaseFirst := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseFirst()
	engine := captureEngine(func(c *gin.Context) {
		_, _ = io.Copy(io.Discard, c.Request.Body)
		if c.GetHeader("X-Test-Request-ID") == "first-request" {
			_, _ = c.Writer.WriteString("first")
			close(started)
			<-release
			_, _ = c.Writer.WriteString("last")
			return
		}
		_, _ = c.Writer.WriteString("second response")
	})
	firstRecorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader("{}"))
	request.Header.Set("X-Test-Request-ID", "first-request")
	go func() { defer close(done); engine.ServeHTTP(firstRecorder, request) }()
	<-started
	assert.Equal(t, int64(32), RequestContentCaptureStats().BufferedBytes)
	secondRecorder := httptest.NewRecorder()
	engine.ServeHTTP(secondRecorder, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader("{}")))
	assert.Equal(t, "second response", secondRecorder.Body.String())
	assert.Equal(t, int64(32), RequestContentCaptureStats().BufferedBytes)
	events := publisher.snapshot()
	secondEnd := events[len(events)-1]
	assert.Equal(t, contentarchive.EventEnd, secondEnd.Kind)
	assert.True(t, secondEnd.Incomplete)
	assert.Equal(t, "buffer_full", secondEnd.Reason)
	assert.Equal(t, "completed", secondEnd.Outcome)
	releaseFirst()
	<-done
	assert.Equal(t, "firstlast", firstRecorder.Body.String())
	assert.Zero(t, RequestContentCaptureStats().BufferedBytes)
}

type captureDeadlineWriter struct {
	*httptest.ResponseRecorder
	deadline time.Time
}

func (writer *captureDeadlineWriter) SetWriteDeadline(deadline time.Time) error {
	writer.deadline = deadline
	return nil
}

func TestRequestContentCaptureWriterAndStreamSnapshot(t *testing.T) {
	for _, test := range []struct {
		name, body, expected string
		snapshot             relaycommon.StreamOutcome
	}{
		{"cancelled", "data: {}\n\n", "cancelled", relaycommon.StreamOutcome{EndReason: relaycommon.StreamEndReasonClientGone}},
		{"completed before cancellation", "data: [DONE]\n\n", "completed", relaycommon.StreamOutcome{EndReason: relaycommon.StreamEndReasonClientGone, Response: relaycommon.ResponseOutcomeCompleted}},
		{"protocol incomplete despite transport done", "data: [DONE]\n\n", "incomplete", relaycommon.StreamOutcome{EndReason: relaycommon.StreamEndReasonDone, Response: relaycommon.ResponseOutcomeIncomplete}},
	} {
		t.Run(test.name, func(t *testing.T) {
			publisher, _ := captureFixture(t, contentarchive.Config{}, CapturePolicy{Enabled: true, RetentionDays: 7, MaxBodyBytes: 1024})
			deadline := time.Unix(1700000000, 0)
			engine := captureEngine(func(c *gin.Context) {
				_, _ = io.Copy(io.Discard, c.Request.Body)
				require.NoError(t, http.NewResponseController(c.Writer).SetWriteDeadline(deadline))
				c.Header("Content-Type", "text/event-stream")
				_, err := c.Writer.WriteString(test.body)
				require.NoError(t, err)
				require.NoError(t, http.NewResponseController(c.Writer).Flush())
				SetRequestContentRelayResult(c, "model", test.snapshot, false)
			})
			writer := &captureDeadlineWriter{ResponseRecorder: httptest.NewRecorder()}
			engine.ServeHTTP(writer, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader("{}")))
			assert.Equal(t, deadline, writer.deadline)
			assert.True(t, writer.Flushed)
			assert.Equal(t, test.body, writer.Body.String())
			events := publisher.snapshot()
			end := events[len(events)-1]
			assert.Equal(t, test.expected, end.Outcome)
			assert.False(t, end.Incomplete)
		})
	}
}
