package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/fresp/it-tools-portal/internal/middleware"
	"github.com/fresp/it-tools-portal/internal/models"
	"github.com/fresp/it-tools-portal/internal/services"
	"github.com/gin-gonic/gin"
)

type exchangeHandlers struct {
	toolStore    ToolStore
	auditStore   AuditStore
	signer       *services.TokenSigner
	userInfoURL  string
}

func registerExchangeRoutes(router *gin.Engine, store ToolStore, auditStore AuditStore, signer *services.TokenSigner, authConfig *middleware.AuthConfig) {
	userInfoURL := os.Getenv("AUTHENTIK_USERINFO_URL")
	if userInfoURL == "" && authConfig != nil && authConfig.IssuerURL != "" {
		userInfoURL = strings.TrimSuffix(authConfig.IssuerURL, "/") + "/userinfo/"
	}

	h := exchangeHandlers{
		toolStore:   store,
		auditStore:  auditStore,
		signer:      signer,
		userInfoURL: userInfoURL,
	}

	exchange := router.Group("/api/auth/exchange")
	if authConfig != nil {
		exchange.Use(middleware.RequireAuth(*authConfig))
	}
	exchange.Use(middleware.RateLimitPerUser())
	exchange.POST("", h.exchange)
}

type exchangeRequest struct {
	ToolID string `json:"tool_id" binding:"required"`
}

// exchange handles POST /api/auth/exchange — mints a tool-scoped JWT.
func (h exchangeHandlers) exchange(c *gin.Context) {
	var req exchangeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tool_id is required"})
		return
	}

	claims, ok := middleware.GetSessionClaims(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	// Look up the tool.
	tool, err := h.toolStore.Get(c.Request.Context(), req.ToolID)
	if err != nil {
		slog.Warn("exchange: tool not found", "tool_id", req.ToolID, "user_id", claims.Sub, "error", err)
		h.recordAudit(c, claims.Sub, req.ToolID, "denied", "tool not found")
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	// Check tool is active.
	if !tool.IsActive {
		slog.Warn("exchange: tool inactive", "tool_id", req.ToolID, "user_id", claims.Sub)
		h.recordAudit(c, claims.Sub, req.ToolID, "denied", "tool inactive")
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	// Check caller's groups include one of tool.allowed_groups.
	if !groupsOverlap(claims.Groups, tool.AllowedGroups) {
		slog.Warn("exchange: group mismatch", "tool_id", req.ToolID, "user_id", claims.Sub,
			"user_groups", claims.Groups, "tool_groups", tool.AllowedGroups)
		h.recordAudit(c, claims.Sub, req.ToolID, "denied", "group mismatch")
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	// Live revocation check against Authentik.
	if h.userInfoURL != "" {
		if err := h.checkRevocation(c.Request.Context(), c); err != nil {
			slog.Warn("exchange: revocation check failed", "tool_id", req.ToolID, "user_id", claims.Sub, "error", err)
			h.recordAudit(c, claims.Sub, req.ToolID, "denied", "revoked")
			c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
			return
		}
	}

	// Determine role from groups.
	role := "user"
	for _, g := range claims.Groups {
		if strings.EqualFold(g, "admin") {
			role = "admin"
			break
		}
	}

	// Mint scoped JWT.
	tokenString, err := h.signer.SignToken(services.ToolScopedClaims{
		Sub:   claims.Sub,
		Email: claims.Email,
		Name:  claims.Name,
		Role:  role,
		Aud:   tool.ID,
	})
	if err != nil {
		slog.Error("exchange: failed to sign token", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	// Build launch URL.
	launchURL := fmt.Sprintf("%s/sso-callback?token=%s", strings.TrimSuffix(tool.BaseURL, "/"), url.QueryEscape(tokenString))

	h.recordAudit(c, claims.Sub, req.ToolID, "success", "")
	c.JSON(http.StatusOK, gin.H{"launch_url": launchURL})
}

// checkRevocation calls Authentik's userinfo endpoint to verify the user is not revoked.
func (h exchangeHandlers) checkRevocation(ctx context.Context, c *gin.Context) error {
	tokenString, err := c.Cookie("it_tools_session")
	if err != nil || tokenString == "" {
		return fmt.Errorf("no session cookie for revocation check")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.userInfoURL, nil)
	if err != nil {
		return fmt.Errorf("create userinfo request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+tokenString)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("userinfo request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("userinfo returned %d", resp.StatusCode)
	}
	return nil
}

func (h exchangeHandlers) recordAudit(c *gin.Context, userID, toolID, result, reason string) {
	log := models.AuditLog{
		UserID:    userID,
		ToolID:    toolID,
		IP:        c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
		Result:    result,
		Reason:    reason,
	}
	if err := h.auditStore.Record(c.Request.Context(), log); err != nil {
		slog.Error("exchange: failed to record audit", "error", err)
	}
}

func groupsOverlap(userGroups, toolGroups []string) bool {
	set := make(map[string]struct{}, len(userGroups))
	for _, g := range userGroups {
		set[strings.ToLower(g)] = struct{}{}
	}
	for _, g := range toolGroups {
		if _, ok := set[strings.ToLower(g)]; ok {
			return true
		}
	}
	return false
}
