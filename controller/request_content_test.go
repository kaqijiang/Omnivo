package controller

import (
	"context"
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/contentarchive"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type requestContentResponseRecorder struct {
	*httptest.ResponseRecorder
	beforeWrite func()
}

func (r *requestContentResponseRecorder) Write(body []byte) (int, error) {
	if r.beforeWrite != nil {
		beforeWrite := r.beforeWrite
		r.beforeWrite = nil
		beforeWrite()
	}
	return r.ResponseRecorder.Write(body)
}

func TestRequestContentSessionAuthorization(t *testing.T) {
	root, legacy := setupAccessTokenAudit(t)
	common.OptionMapRWMutex.Lock()
	previousOptions := common.OptionMap
	common.OptionMap = maps.Clone(previousOptions)
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptions
		common.OptionMapRWMutex.Unlock()
	})
	root.Role = common.RoleRootUser
	require.NoError(t, model.DB.Model(root).Update("role", root.Role).Error)
	identity, rootJWT := createAccessTokenTestSession(t, root.Id, "content-root-session")
	rootPAT, _ := createScopedAccessToken(t, root.Id, 0, "log:read", "option:read", "option:write")
	users := []struct {
		name string
		role int
	}{{"content-user", common.RoleCommonUser}, {"content-admin", common.RoleAdminUser}}
	credentials := map[string]string{"root": rootJWT, "root PAT": rootPAT, "legacy root PAT": legacy, "anonymous": "", "model key": "sk-not-a-dashboard-session"}
	for _, item := range users {
		user := &model.User{Username: item.name, Password: "placeholder", Role: item.role, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, AffCode: item.name}
		require.NoError(t, model.DB.Create(user).Error)
		_, credentials[item.name] = createAccessTokenTestSession(t, user.Id, item.name+"-session")
	}
	previousReader := readRequestContentPage
	t.Cleanup(func() { readRequestContentPage = previousReader })
	reads := 0
	readRequestContentPage = func(ctx context.Context, id string, after int64, limit int) (*contentarchive.ContentPage, error) {
		reads++
		return &contentarchive.ContentPage{RequestID: id, State: "complete", UserID: 999, ExpiresAt: time.Now().Add(time.Hour).Unix(), Events: []contentarchive.Event{{Kind: "data", Direction: "response", Data: []byte("private-content-sentinel")}}}, nil
	}
	router := gin.New()
	router.Use(middleware.RequestId())
	router.GET("/api/log/content/:request_id", middleware.DisableCache(), middleware.RootAuth(), GetRequestContent)
	router.GET("/api/option/request_content/status", middleware.DisableCache(), middleware.RootAuth(), GetRequestContentStatus)
	router.PUT("/api/option/", middleware.RootAuth(), UpdateOption)
	for _, test := range []struct {
		name   string
		status int
	}{
		{"root", 200}, {"root PAT", 403}, {"legacy root PAT", 403}, {"anonymous", 401}, {"model key", 401}, {"content-user", 403}, {"content-admin", 403},
	} {
		t.Run(test.name, func(t *testing.T) {
			before := reads
			for _, path := range []string{"/api/log/content/test-request-id?after=-1&limit=32", "/api/option/request_content/status"} {
				request := httptest.NewRequest(http.MethodGet, path, nil)
				if credentials[test.name] != "" {
					request.Header.Set("Authorization", "Bearer "+credentials[test.name])
				}
				request.Header.Set("X-Role", "100")
				response := httptest.NewRecorder()
				router.ServeHTTP(response, request)
				assert.Equal(t, test.status, response.Code, response.Body.String())
				assert.Contains(t, response.Header().Get("Cache-Control"), "no-store")
				if test.status != 200 {
					assert.NotContains(t, response.Body.String(), "cHJpdmF0ZS1jb250ZW50")
				}
			}
			if test.status != 200 {
				assert.Equal(t, before, reads, "unauthorized calls must not fetch objects")
			}
		})
	}
	var audit model.AuditLog
	require.NoError(t, model.LOG_DB.Where("action = ?", "request_content.view").First(&audit).Error)
	assert.Equal(t, root.Id, audit.UserId)
	encodedAudit, err := common.Marshal(audit.Other.Op.Params)
	require.NoError(t, err)
	assert.JSONEq(t, `{"request_id":"test-request-id","after":-1}`, string(encodedAudit))
	assert.NotContains(t, audit.Content, "private-content-sentinel")
	assert.NotContains(t, audit.Content, rootJWT)

	t.Run("read capacity covers audit and response writes", func(t *testing.T) {
		var releases []func()
		defer func() {
			for _, release := range releases {
				release()
			}
		}()
		for range 4 {
			release, err := service.AcquireRequestContentRead()
			require.NoError(t, err)
			releases = append(releases, release)
		}
		before := reads
		full := accessTokenRequest(router, http.MethodGet, "/api/log/content/id", rootJWT, "", "")
		assert.Equal(t, http.StatusServiceUnavailable, full.Code)
		assert.Equal(t, before, reads, "capacity exhaustion must not fetch or decrypt content")
		releases[len(releases)-1]()
		releases = releases[:len(releases)-1]

		assertNoSlot := func() {
			release, err := service.AcquireRequestContentRead()
			assert.ErrorIs(t, err, service.ErrRequestContentUnavailable)
			if release != nil {
				release()
			}
		}
		audited, wrote := false, false
		const callback = "test:content_read_lease_during_audit"
		require.NoError(t, model.LOG_DB.Callback().Create().After("gorm:commit_or_rollback_transaction").Register(callback, func(tx *gorm.DB) {
			if tx.Statement.Table == "audit_logs" {
				assertNoSlot()
				audited = true
			}
		}))
		defer func() { require.NoError(t, model.LOG_DB.Callback().Create().Remove(callback)) }()
		request := httptest.NewRequest(http.MethodGet, "/api/log/content/id", nil)
		request.Header.Set("Authorization", "Bearer "+rootJWT)
		response := &requestContentResponseRecorder{ResponseRecorder: httptest.NewRecorder(), beforeWrite: func() {
			assertNoSlot()
			wrote = true
		}}
		router.ServeHTTP(response, request)
		assert.Equal(t, http.StatusOK, response.Code)
		assert.Equal(t, before+1, reads)
		assert.True(t, audited, "the handler must retain the page lease during audit I/O")
		assert.True(t, wrote, "the handler must retain the page lease during response I/O")
		release, err := service.AcquireRequestContentRead()
		require.NoError(t, err, "handler completion must release its page lease")
		release()
	})
	t.Run("validation never reads objects", func(t *testing.T) {
		before := reads
		for _, path := range []string{"/api/log/content/bad.id", "/api/log/content/" + strings.Repeat("a", 65), "/api/log/content/id?after=-2", "/api/log/content/id?limit=33", "/api/log/content/id?after=overflow"} {
			response := accessTokenRequest(router, http.MethodGet, path, rootJWT, "", "")
			assert.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
		}
		assert.Equal(t, before, reads)
	})
	t.Run("PAT cannot enable retention", func(t *testing.T) {
		response := accessTokenRequest(router, http.MethodPut, "/api/option/", rootPAT, "", `{"key":"request_content.enabled","value":"true"}`)
		assert.Equal(t, http.StatusForbidden, response.Code)
	})
	t.Run("settings validate and persist without enabling by default", func(t *testing.T) {
		for _, body := range []string{
			`{"key":"request_content.enabled","value":"true"}`,
			`{"key":"request_content.retention_days","value":0}`,
			`{"key":"request_content.retention_days","value":366}`,
			`{"key":"request_content.max_body_bytes","value":67108865}`,
			`{"key":"request_content.unknown","value":"true"}`,
		} {
			response := accessTokenRequest(router, http.MethodPut, "/api/option/", rootJWT, "", body)
			assert.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
		}
		for _, body := range []string{
			`{"key":"request_content.enabled","value":false}`,
			`{"key":"request_content.retention_days","value":14}`,
			`{"key":"request_content.max_body_bytes","value":2097152}`,
		} {
			response := accessTokenRequest(router, http.MethodPut, "/api/option/", rootJWT, "", body)
			assert.Equal(t, http.StatusOK, response.Code, response.Body.String())
		}
		var saved model.Option
		require.NoError(t, model.DB.Where("key = ?", "request_content.max_body_bytes").Take(&saved).Error)
		assert.Equal(t, "2097152", saved.Value)
		assert.False(t, service.CurrentRequestContentPolicy().Enabled)
	})
	t.Run("expired content is not disclosed", func(t *testing.T) {
		readRequestContentPage = func(context.Context, string, int64, int) (*contentarchive.ContentPage, error) {
			return &contentarchive.ContentPage{RequestID: "expired", ExpiresAt: time.Now().Unix() - 1, Events: []contentarchive.Event{{Data: []byte("expired-secret")}}}, nil
		}
		response := accessTokenRequest(router, http.MethodGet, "/api/log/content/expired", rootJWT, "", "")
		assert.Equal(t, http.StatusOK, response.Code)
		assert.Contains(t, response.Body.String(), `"data":null`)
	})
	t.Run("storage errors are generic", func(t *testing.T) {
		readRequestContentPage = func(context.Context, string, int64, int) (*contentarchive.ContentPage, error) {
			return nil, errors.New("secret connection credential")
		}
		response := accessTokenRequest(router, http.MethodGet, "/api/log/content/id", rootJWT, "", "")
		assert.Equal(t, http.StatusServiceUnavailable, response.Code)
		assert.NotContains(t, response.Body.String(), "secret connection credential")
	})
	t.Run("downgrade during object read", func(t *testing.T) {
		defer func() { require.NoError(t, model.DB.Model(root).Update("role", common.RoleRootUser).Error) }()
		readRequestContentPage = func(context.Context, string, int64, int) (*contentarchive.ContentPage, error) {
			require.NoError(t, model.DB.Model(root).Update("role", common.RoleAdminUser).Error)
			return &contentarchive.ContentPage{RequestID: "id", ExpiresAt: time.Now().Add(time.Hour).Unix(), Events: []contentarchive.Event{{Data: []byte("late-private-content")}}}, nil
		}
		response := accessTokenRequest(router, http.MethodGet, "/api/log/content/id", rootJWT, "", "")
		assert.Equal(t, http.StatusForbidden, response.Code)
		assert.NotContains(t, response.Body.String(), "bGF0ZS1wcml2YXRl")
	})
	t.Run("downgrade during audit write", func(t *testing.T) {
		defer func() { require.NoError(t, model.DB.Model(root).Update("role", common.RoleRootUser).Error) }()
		readRequestContentPage = func(context.Context, string, int64, int) (*contentarchive.ContentPage, error) {
			return &contentarchive.ContentPage{RequestID: "id", ExpiresAt: time.Now().Add(time.Hour).Unix(), Events: []contentarchive.Event{{Data: []byte("late-private-content")}}}, nil
		}
		changedRole := false
		const callback = "test:content_downgrade_during_audit"
		require.NoError(t, model.LOG_DB.Callback().Create().After("gorm:commit_or_rollback_transaction").Register(callback, func(tx *gorm.DB) {
			if tx.Statement.Table == "audit_logs" {
				require.NoError(t, model.DB.Model(root).Update("role", common.RoleAdminUser).Error)
				changedRole = true
			}
		}))
		defer func() { require.NoError(t, model.LOG_DB.Callback().Create().Remove(callback)) }()
		response := accessTokenRequest(router, http.MethodGet, "/api/log/content/id", rootJWT, "", "")
		require.True(t, changedRole, "the audit write must finish after permission changes")
		assert.Equal(t, http.StatusForbidden, response.Code)
		assert.NotContains(t, response.Body.String(), "bGF0ZS1wcml2YXRl")
	})
	t.Run("revocation during object read", func(t *testing.T) {
		readRequestContentPage = func(context.Context, string, int64, int) (*contentarchive.ContentPage, error) {
			_, err := model.RevokeUserSession(root.Id, identity.SessionID, "test revoke during archive read")
			require.NoError(t, err)
			return &contentarchive.ContentPage{RequestID: "id", ExpiresAt: time.Now().Add(time.Hour).Unix(), Events: []contentarchive.Event{{Data: []byte("late-private-content")}}}, nil
		}
		response := accessTokenRequest(router, http.MethodGet, "/api/log/content/id", rootJWT, "", "")
		assert.Equal(t, http.StatusForbidden, response.Code)
		assert.NotContains(t, response.Body.String(), "bGF0ZS1wcml2YXRl")
		response = accessTokenRequest(router, http.MethodGet, "/api/option/request_content/status", rootJWT, "", "")
		assert.Equal(t, http.StatusUnauthorized, response.Code)
	})
}
