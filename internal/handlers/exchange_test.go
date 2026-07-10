package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fresp/it-tools-portal/internal/middleware"
	"github.com/fresp/it-tools-portal/internal/models"
	"github.com/fresp/it-tools-portal/internal/services"
	"github.com/gin-gonic/gin"
)

// mockToolStore implements ToolStore for testing.
type mockToolStore struct {
	tools map[string]models.Tool
}

func (m *mockToolStore) Create(ctx context.Context, request models.CreateToolRequest) (models.Tool, error) {
	return models.Tool{}, nil
}
func (m *mockToolStore) List(ctx context.Context) ([]models.Tool, error) {
	return nil, nil
}
func (m *mockToolStore) ListAvailable(ctx context.Context, groups []string) ([]models.Tool, error) {
	return nil, nil
}
func (m *mockToolStore) Get(ctx context.Context, id string) (models.Tool, error) {
	tool, ok := m.tools[id]
	if !ok {
		return models.Tool{}, nil
	}
	return tool, nil
}
func (m *mockToolStore) Update(ctx context.Context, id string, request models.UpdateToolRequest) (models.Tool, error) {
	return models.Tool{}, nil
}
func (m *mockToolStore) Delete(ctx context.Context, id string) error {
	return nil
}

// mockAuditStore implements AuditStore for testing.
type mockAuditStore struct {
	logs []models.AuditLog
}

func (m *mockAuditStore) Record(ctx context.Context, log models.AuditLog) error {
	m.logs = append(m.logs, log)
	return nil
}

func newTestExchangeHandler(t *testing.T) (*exchangeHandlers, *mockToolStore, *mockAuditStore) {
	t.Helper()

	t.Setenv("JWT_PRIVATE_KEY_PATH", "")
	t.Setenv("JWT_KID", "test")
	t.Setenv("JWT_EXPIRY_SECONDS", "90")

	signer, err := services.NewTokenSigner()
	if err != nil {
		t.Fatal(err)
	}

	toolStore := &mockToolStore{
		tools: map[string]models.Tool{
			"tool-1": {
				ID:            "tool-1",
				Name:          "Test Tool",
				BaseURL:       "https://tool.example.com",
				AllowedGroups: []string{"group-a", "group-b"},
				IsActive:      true,
			},
			"tool-inactive": {
				ID:            "tool-inactive",
				Name:          "Inactive Tool",
				BaseURL:       "https://inactive.example.com",
				AllowedGroups: []string{"group-a"},
				IsActive:      false,
			},
		},
	}

	auditStore := &mockAuditStore{}

	h := &exchangeHandlers{
		toolStore:   toolStore,
		auditStore:  auditStore,
		signer:      signer,
		userInfoURL: "", // Disable revocation check for unit tests.
	}

	return h, toolStore, auditStore
}

func setupTestRouter(h *exchangeHandlers, groups []string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/auth/exchange", func(c *gin.Context) {
		c.Set("session_claims", &middleware.SessionClaims{
			Sub:    "user-123",
			Email:  "user@example.com",
			Name:   "Test User",
			Groups: groups,
		})
		h.exchange(c)
	})
	return router
}

func TestExchange_Success(t *testing.T) {
	h, _, auditStore := newTestExchangeHandler(t)
	router := setupTestRouter(h, []string{"group-a"})

	body := `{"tool_id":"tool-1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/exchange", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	launchURL, ok := resp["launch_url"]
	if !ok {
		t.Fatal("expected launch_url in response")
	}
	if launchURL == "" {
		t.Error("expected non-empty launch_url")
	}

	// Verify audit log.
	if len(auditStore.logs) != 1 {
		t.Fatalf("audit logs = %d, want 1", len(auditStore.logs))
	}
	if auditStore.logs[0].Result != "success" {
		t.Errorf("audit result = %q, want %q", auditStore.logs[0].Result, "success")
	}
}

func TestExchange_ToolNotFound(t *testing.T) {
	h, _, auditStore := newTestExchangeHandler(t)
	router := setupTestRouter(h, []string{"group-a"})

	body := `{"tool_id":"nonexistent"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/exchange", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
	}

	if len(auditStore.logs) != 1 || auditStore.logs[0].Result != "denied" {
		t.Error("expected denied audit log")
	}
}

func TestExchange_InactiveTool(t *testing.T) {
	h, _, auditStore := newTestExchangeHandler(t)
	router := setupTestRouter(h, []string{"group-a"})

	body := `{"tool_id":"tool-inactive"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/exchange", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
	}

	if len(auditStore.logs) != 1 || auditStore.logs[0].Result != "denied" {
		t.Error("expected denied audit log")
	}
}

func TestExchange_GroupMismatch(t *testing.T) {
	h, _, auditStore := newTestExchangeHandler(t)
	router := setupTestRouter(h, []string{"group-z"})

	body := `{"tool_id":"tool-1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/exchange", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
	}

	if len(auditStore.logs) != 1 || auditStore.logs[0].Result != "denied" {
		t.Error("expected denied audit log")
	}
}

func TestExchange_MissingToolID(t *testing.T) {
	h, _, _ := newTestExchangeHandler(t)
	router := setupTestRouter(h, []string{"group-a"})

	body := `{}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/exchange", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestGroupsOverlap(t *testing.T) {
	tests := []struct {
		name     string
		user     []string
		tool     []string
		expected bool
	}{
		{"match", []string{"a", "b"}, []string{"b", "c"}, true},
		{"no match", []string{"a"}, []string{"b", "c"}, false},
		{"case insensitive", []string{"Group-A"}, []string{"group-a"}, true},
		{"empty user", []string{}, []string{"a"}, false},
		{"empty tool", []string{"a"}, []string{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := groupsOverlap(tt.user, tt.tool); got != tt.expected {
				t.Errorf("groupsOverlap(%v, %v) = %v, want %v", tt.user, tt.tool, got, tt.expected)
			}
		})
	}
}
