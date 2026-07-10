package handlers

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/fresp/it-tools-portal/internal/middleware"
	"github.com/fresp/it-tools-portal/internal/models"
	"github.com/fresp/it-tools-portal/internal/repositories"
	"github.com/gin-gonic/gin"
)

type ToolStore interface {
	Create(ctx context.Context, request models.CreateToolRequest) (models.Tool, error)
	List(ctx context.Context) ([]models.Tool, error)
	ListAvailable(ctx context.Context, groups []string) ([]models.Tool, error)
	Get(ctx context.Context, id string) (models.Tool, error)
	Update(ctx context.Context, id string, request models.UpdateToolRequest) (models.Tool, error)
	Delete(ctx context.Context, id string) error
}

// AuditStore defines the interface for audit log persistence.
type AuditStore interface {
	Record(ctx context.Context, log models.AuditLog) error
}

type toolHandlers struct {
	store ToolStore
}

func registerToolRoutes(router *gin.Engine, store ToolStore, authConfig *middleware.AuthConfig) {
	handlers := toolHandlers{store: store}

	// Public: available tools for the authenticated user.
	public := router.Group("/api/tools")
	if authConfig != nil {
		public.Use(middleware.RequireAuth(*authConfig))
	}
	public.GET("", handlers.listAvailable)

	// Admin: CRUD endpoints.
	admin := router.Group("/api/admin/tools")
	if authConfig != nil {
		admin.Use(middleware.RequireAuth(*authConfig), middleware.RequireAdmin())
	} else {
		admin.Use(placeholderAdminAuth(os.Getenv("ADMIN_TOKEN")))
	}
	admin.POST("", handlers.create)
	admin.GET("", handlers.list)
	admin.GET("/:id", handlers.get)
	admin.PUT("/:id", handlers.update)
	admin.DELETE("/:id", handlers.delete)
}

func (h toolHandlers) create(c *gin.Context) {
	var request models.CreateToolRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON body"})
		return
	}
	tool, err := h.store.Create(c.Request.Context(), request)
	if err != nil {
		writeToolError(c, err)
		return
	}
	c.JSON(http.StatusCreated, tool)
}

func (h toolHandlers) list(c *gin.Context) {
	tools, err := h.store.List(c.Request.Context())
	if err != nil {
		writeToolError(c, err)
		return
	}
	c.JSON(http.StatusOK, tools)
}

func (h toolHandlers) listAvailable(c *gin.Context) {
	var groups []string
	if claims, ok := middleware.GetSessionClaims(c); ok {
		groups = claims.Groups
	} else {
		groups = groupsFromHeader(c.GetHeader("X-User-Groups"))
	}
	tools, err := h.store.ListAvailable(c.Request.Context(), groups)
	if err != nil {
		writeToolError(c, err)
		return
	}
	c.JSON(http.StatusOK, tools)
}

func (h toolHandlers) get(c *gin.Context) {
	tool, err := h.store.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeToolError(c, err)
		return
	}
	c.JSON(http.StatusOK, tool)
}

func (h toolHandlers) update(c *gin.Context) {
	var request models.UpdateToolRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON body"})
		return
	}
	tool, err := h.store.Update(c.Request.Context(), c.Param("id"), request)
	if err != nil {
		writeToolError(c, err)
		return
	}
	c.JSON(http.StatusOK, tool)
}

func (h toolHandlers) delete(c *gin.Context) {
	if err := h.store.Delete(c.Request.Context(), c.Param("id")); err != nil {
		writeToolError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func placeholderAdminAuth(adminToken string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// TODO(FRE-33): replace this static-token placeholder with Authentik admin-role middleware.
		token := c.GetHeader("X-Admin-Token")
		if token == "" {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		if adminToken == "" || token != adminToken {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		c.Next()
	}
}

func writeToolError(c *gin.Context, err error) {
	if errors.Is(err, models.ErrInvalidTool) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid tool"})
		return
	}
	if repositories.IsToolNotFound(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "tool not found"})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
}

func groupsFromHeader(value string) []string {
	parts := strings.Split(value, ",")
	groups := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		group := strings.TrimSpace(part)
		if group == "" {
			continue
		}
		if _, ok := seen[group]; ok {
			continue
		}
		seen[group] = struct{}{}
		groups = append(groups, group)
	}
	return groups
}
