package handlers

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/fresp/it-tools-portal/internal/middleware"
	"github.com/fresp/it-tools-portal/internal/models"
	"github.com/fresp/it-tools-portal/internal/services"
	"github.com/gin-gonic/gin"
	"github.com/lestrrat-go/jwx/v3/jwa"
	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/lestrrat-go/jwx/v3/jwt"
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

// --- helpers for edge-case tests ---

// testRSAKey creates a test RSA key pair and returns the private JWK key + public JWK set.
func testRSAKey(t *testing.T) (jwk.Key, jwk.Set) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	key, err := jwk.Import(priv)
	if err != nil {
		t.Fatal(err)
	}
	key.Set(jwk.KeyIDKey, "test-kid")
	key.Set(jwk.AlgorithmKey, jwa.RS256())
	privSet := jwk.NewSet()
	privSet.AddKey(key)
	pubSet, err := jwk.PublicSetOf(privSet)
	if err != nil {
		t.Fatal(err)
	}
	return key, pubSet
}

// signExpiredJWT signs a JWT that expired 1 hour ago.
func signExpiredJWT(t *testing.T, key jwk.Key, claims map[string]interface{}) string {
	t.Helper()
	builder := jwt.NewBuilder().
		Issuer("https://auth.example.com/application/o/test/").
		Subject("test-user").
		IssuedAt(time.Now().Add(-2 * time.Hour)).
		Expiration(time.Now().Add(-1 * time.Hour))
	for k, v := range claims {
		builder.Claim(k, v)
	}
	tok, err := builder.Build()
	if err != nil {
		t.Fatal(err)
	}
	signed, err := jwt.Sign(tok, jwt.WithKey(jwa.RS256(), key))
	if err != nil {
		t.Fatal(err)
	}
	return string(signed)
}

// jwksTestServer starts an httptest.Server that serves the given JWK set as JWKS.
func jwksTestServer(t *testing.T, pubSet jwk.Set) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		err := json.NewEncoder(w).Encode(pubSet)
		if err != nil {
			t.Logf("jwks server encode error: %v", err)
		}
	}))
	return srv
}

// setupTestRouterWithMiddleware creates a gin router with real RequireAuth middleware.
func setupTestRouterWithMiddleware(h *exchangeHandlers, authConfig *middleware.AuthConfig) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	exchange := router.Group("/api/auth/exchange")
	if authConfig != nil {
		exchange.Use(middleware.RequireAuth(*authConfig))
	}
	exchange.POST("", h.exchange)
	return router
}

// extractJTI parses a launch_url and extracts the jti claim from the embedded JWT.
func extractJTI(t *testing.T, launchURL string) string {
	t.Helper()
	parsed, err := url.Parse(launchURL)
	if err != nil {
		t.Fatalf("parse launch_url: %v", err)
	}
	tokenStr := parsed.Query().Get("token")
	if tokenStr == "" {
		t.Fatal("launch_url missing token param")
	}
	tok, err := jwt.Parse([]byte(tokenStr), jwt.WithVerify(false))
	if err != nil {
		t.Fatalf("parse jwt: %v", err)
	}
	var jti string
	if err := tok.Get("jti", &jti); err != nil {
		t.Fatalf("extract jti: %v", err)
	}
	return jti
}

// --- edge-case tests ---

// TestExchange_ExpiredToken verifies that an expired session JWT is rejected
// by the auth middleware before reaching the handler.
func TestExchange_ExpiredToken(t *testing.T) {
	h, _, _ := newTestExchangeHandler(t)

	// Create test RSA key and JWKS server.
	privKey, pubSet := testRSAKey(t)
	jwksSrv := jwksTestServer(t, pubSet)
	defer jwksSrv.Close()

	// Initialize JWKS fetcher against the test server.
	fetcher := middleware.NewJWKSFetcher(jwksSrv.URL, time.Hour)
	if err := fetcher.Init(context.Background()); err != nil {
		t.Fatal(err)
	}

	authConfig := &middleware.AuthConfig{
		JWKSFetcher:  fetcher,
		IssuerURL:    "https://auth.example.com/application/o/test/",
		AuthorizeURL: "https://auth.example.com/application/o/authorize/",
		ClientID:     "test-client",
		RedirectURI:  "http://localhost:8080/callback",
		SessionName:  "it_tools_session",
	}

	router := setupTestRouterWithMiddleware(h, authConfig)

	// Sign an expired JWT.
	expiredToken := signExpiredJWT(t, privKey, map[string]interface{}{
		"email":  "user@example.com",
		"name":   "Test User",
		"groups": []string{"group-a"},
	})

	body := `{"tool_id":"tool-1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/exchange", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "it_tools_session", Value: expiredToken})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// The middleware should reject the expired token with 401 (for API routes).
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d, body: %s", w.Code, http.StatusUnauthorized, w.Body.String())
	}
}

// TestExchange_RevokedUser verifies that a user whose Authentik userinfo
// check returns non-200 is denied access.
func TestExchange_RevokedUser(t *testing.T) {
	h, _, auditStore := newTestExchangeHandler(t)

	// Mock userinfo server that returns 401 (user is revoked).
	userinfoSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer userinfoSrv.Close()

	h.userInfoURL = userinfoSrv.URL

	// Set a session cookie so checkRevocation has something to send.
	router := setupTestRouter(h, []string{"group-a"})

	body := `{"tool_id":"tool-1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/exchange", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "it_tools_session", Value: "dummy-session-token"})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d, body: %s", w.Code, http.StatusForbidden, w.Body.String())
	}

	// Verify audit log records the revocation denial.
	if len(auditStore.logs) != 1 {
		t.Fatalf("audit logs = %d, want 1", len(auditStore.logs))
	}
	if auditStore.logs[0].Result != "denied" {
		t.Errorf("audit result = %q, want %q", auditStore.logs[0].Result, "denied")
	}
}

// TestExchange_ReplayPrevention verifies that successive exchange calls
// produce tokens with different jtis, preventing replay attacks.
func TestExchange_ReplayPrevention(t *testing.T) {
	h, _, _ := newTestExchangeHandler(t)
	router := setupTestRouter(h, []string{"group-a"})

	body := `{"tool_id":"tool-1"}`

	// First exchange.
	req1 := httptest.NewRequest(http.MethodPost, "/api/auth/exchange", bytes.NewBufferString(body))
	req1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	router.ServeHTTP(w1, req1)

	if w1.Code != http.StatusOK {
		t.Fatalf("first exchange: status = %d, want %d", w1.Code, http.StatusOK)
	}

	// Second exchange.
	req2 := httptest.NewRequest(http.MethodPost, "/api/auth/exchange", bytes.NewBufferString(body))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("second exchange: status = %d, want %d", w2.Code, http.StatusOK)
	}

	// Parse launch URLs and extract jtis.
	var resp1, resp2 map[string]string
	if err := json.Unmarshal(w1.Body.Bytes(), &resp1); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(w2.Body.Bytes(), &resp2); err != nil {
		t.Fatal(err)
	}

	jti1 := extractJTI(t, resp1["launch_url"])
	jti2 := extractJTI(t, resp2["launch_url"])

	if jti1 == jti2 {
		t.Errorf("expected different jtis, both got %q", jti1)
	}
}
