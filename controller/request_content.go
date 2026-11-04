package controller

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// Kept as a controller dependency so permission and mid-read revocation are
// tested without coupling authentication tests to a live object store.
var readRequestContentPage = service.ReadRequestContentPage

func requestContentSession(c *gin.Context) (service.AuthIdentity, bool) {
	identity, ok := middleware.GetSessionAuthIdentity(c)
	if !ok || c.GetBool("use_access_token") || c.GetInt("role") != common.RoleRootUser {
		common.ApiErrorStatus(c, http.StatusForbidden, common.NewMessage("Permission denied"))
		return service.AuthIdentity{}, false
	}
	return identity, true
}

func GetRequestContentStatus(c *gin.Context) {
	if _, ok := requestContentSession(c); !ok {
		return
	}
	common.ApiSuccess(c, service.RequestContentArchiveStatus())
}

func GetRequestContent(c *gin.Context) {
	identity, ok := requestContentSession(c)
	if !ok {
		return
	}
	requestID := c.Param("request_id")
	validID := len(requestID) > 0 && len(requestID) <= 64
	for _, char := range requestID {
		if !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '-' || char == '_') {
			validID = false
			break
		}
	}
	after, afterErr := strconv.ParseInt(c.DefaultQuery("after", "-1"), 10, 64)
	limit, limitErr := strconv.Atoi(c.DefaultQuery("limit", "32"))
	if !validID || strings.TrimSpace(requestID) != requestID || afterErr != nil || after < -1 || limitErr != nil || limit < 1 || limit > 32 {
		common.ApiErrorStatus(c, http.StatusBadRequest, common.NewMessage("Invalid request content query"))
		return
	}
	release, err := service.AcquireRequestContentRead()
	if err != nil {
		common.ApiErrorStatus(c, http.StatusServiceUnavailable, common.NewMessage("Request content storage is unavailable"))
		return
	}
	defer release()
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	page, err := readRequestContentPage(ctx, requestID, after, limit)
	if err != nil {
		common.ApiErrorStatus(c, http.StatusServiceUnavailable, common.NewMessage("Request content storage is unavailable"))
		return
	}
	// Object reads may take seconds. Re-check the live session and role before
	// disclosing plaintext, including sessions revoked while storage was read.
	_, user, err := service.ValidateLoginSession(identity)
	if err != nil || user.Role != common.RoleRootUser {
		common.ApiErrorStatus(c, http.StatusForbidden, common.NewMessage("Permission denied"))
		return
	}
	if page != nil && page.ExpiresAt <= time.Now().Unix() {
		page = nil
	}
	if page != nil {
		model.RecordAuditLog(c, model.AuditLog{
			UserId: identity.UserID, ActorRole: common.RoleRootUser,
			Category: model.AuditCategoryOperation, Action: "request_content.view",
			Status: http.StatusOK, Success: true,
			Other: model.AuditOther{Op: &model.AuditOperation{Action: "request_content.view", Params: model.AuditFields{"request_id": requestID, "after": after}}},
		})
	}
	// Audit storage can also block. Revalidate after every I/O step and just
	// before disclosing plaintext so revocation during the audit is respected.
	_, user, err = service.ValidateLoginSession(identity)
	if err != nil || user.Role != common.RoleRootUser {
		common.ApiErrorStatus(c, http.StatusForbidden, common.NewMessage("Permission denied"))
		return
	}
	if page != nil && page.ExpiresAt <= time.Now().Unix() {
		page = nil
	}
	common.ApiSuccess(c, page)
}
