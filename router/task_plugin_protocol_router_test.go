package router

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/contentarchive"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHostProtocolRegistryDrivesProtocolRoutesOnce(t *testing.T) {
	engine := gin.New()
	SetTaskPluginProtocolRouter(engine)

	expected := []string{
		"POST /v1/responses",
		"GET /v1/responses/:response_id",
		"POST /v1/videos",
		"GET /v1/videos/:task_id",
		"GET /v1/videos/:task_id/content",
		"HEAD /v1/videos/:task_id/content",
		"POST /v1/images/generations",
		"POST /v1/images/edits",
	}
	actual := make([]string, 0, len(engine.Routes()))
	for _, route := range engine.Routes() {
		actual = append(actual, fmt.Sprintf("%s %s", route.Method, route.Path))
	}
	sort.Strings(expected)
	sort.Strings(actual)
	assert.Equal(t, expected, actual)
}

type protocolCapturePublisher struct {
	mu     sync.Mutex
	events []contentarchive.Event
}

func (p *protocolCapturePublisher) TryPublish(event contentarchive.Event) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	event.Data = append([]byte(nil), event.Data...)
	p.events = append(p.events, event)
	return true
}

func (p *protocolCapturePublisher) Stats() contentarchive.PublisherStats {
	return contentarchive.PublisherStats{}
}
func (p *protocolCapturePublisher) Close(context.Context) error { return nil }

func TestResponsesProtocolRouteCapturesOnlyAuthenticatedRequests(t *testing.T) {
	setupRelayRouterTestDB(t)
	require.NoError(t, i18n.Init())
	rateLimitEnabled := setting.ModelRequestRateLimitEnabled
	setting.ModelRequestRateLimitEnabled = false
	t.Cleanup(func() { setting.ModelRequestRateLimitEnabled = rateLimitEnabled })
	user := model.User{Username: "responses-capture-user", Status: common.UserStatusEnabled, Group: "default", Quota: 100}
	require.NoError(t, model.DB.Create(&user).Error)
	require.NoError(t, model.DB.Create(&model.Token{
		UserId: user.Id, Key: "responsescapturekey", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true,
	}).Error)
	publisher := &protocolCapturePublisher{}
	stop, err := service.SetRequestContentCapture(publisher, contentarchive.Config{
		MaxActive: 8, ChunkBytes: 32, MaxBufferedBytes: 64 << 10, FlushInterval: time.Hour,
	}, func() service.CapturePolicy {
		return service.CapturePolicy{Enabled: true, RetentionDays: 7, MaxBodyBytes: 4096}
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, stop(context.Background())) })
	engine := gin.New()
	engine.Use(middleware.RequestId())
	SetRelayRouter(engine)
	SetTaskPluginProtocolRouter(engine)

	const body = `{"model":`
	unauthorized := httptest.NewRecorder()
	engine.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body)))
	require.Equal(t, http.StatusUnauthorized, unauthorized.Code)
	publisher.mu.Lock()
	assert.Empty(t, publisher.events)
	publisher.mu.Unlock()

	request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer responsescapturekey")
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	publisher.mu.Lock()
	events := append([]contentarchive.Event(nil), publisher.events...)
	publisher.mu.Unlock()
	require.GreaterOrEqual(t, len(events), 4)
	assert.Equal(t, contentarchive.EventStart, events[0].Kind)
	assert.Equal(t, contentarchive.EventEnd, events[len(events)-1].Kind)
	assert.Equal(t, http.StatusBadRequest, events[len(events)-1].HTTPStatus)
	var requestBody, responseBody strings.Builder
	for i, event := range events {
		assert.Equal(t, int64(i), event.Sequence)
		assert.Equal(t, user.Id, event.UserID)
		assert.Equal(t, "/v1/responses", event.Path)
		switch event.Direction {
		case contentarchive.DirectionRequest:
			requestBody.Write(event.Data)
		case contentarchive.DirectionResponse:
			responseBody.Write(event.Data)
		}
	}
	assert.Equal(t, body, requestBody.String())
	assert.Equal(t, recorder.Body.String(), responseBody.String())
}
