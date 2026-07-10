package middleware

import (
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lestrrat-go/jwx/v3/jwa"
	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/lestrrat-go/jwx/v3/jwt"
)

func newTestRSAKey(t *testing.T) (jwk.Key, jwk.Set) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	key, err := jwk.Import(priv)
	if err != nil {
		t.Fatalf("import jwk: %v", err)
	}
	key.Set(jwk.KeyIDKey, "test-kid")
	key.Set(jwk.AlgorithmKey, jwa.RS256())

	privSet := jwk.NewSet()
	privSet.AddKey(key)
	pubSet, err := jwk.PublicSetOf(privSet)
	if err != nil {
		t.Fatalf("public set: %v", err)
	}
	return key, pubSet
}

func signTestJWT(t *testing.T, key jwk.Key, claims map[string]interface{}, expiry time.Duration) string {
	t.Helper()
	builder := jwt.NewBuilder().
		Issuer("https://auth.example.com/application/o/test/").
		Subject("test-user").
		IssuedAt(time.Now()).
		Expiration(time.Now().Add(expiry))

	for k, v := range claims {
		builder.Claim(k, v)
	}

	tok, err := builder.Build()
	if err != nil {
		t.Fatalf("build jwt: %v", err)
	}

	signed, err := jwt.Sign(tok, jwt.WithKey(jwa.RS256(), key))
	if err != nil {
		t.Fatalf("sign jwt: %v", err)
	}
	return string(signed)
}

// validateSessionTokenWithKeyset validates a token against a pre-fetched key set.
func validateSessionTokenWithKeyset(keyset jwk.Set, issuerURL, tokenString string) (*SessionClaims, error) {
	parsed, err := jwt.Parse([]byte(tokenString),
		jwt.WithKeySet(keyset),
		jwt.WithIssuer(issuerURL),
		jwt.WithAcceptableSkew(30*time.Second),
	)
	if err != nil {
		return nil, err
	}

	claims := &SessionClaims{}
	var sub string
	if err := parsed.Get("sub", &sub); err == nil {
		claims.Sub = sub
	}
	var email string
	if err := parsed.Get("email", &email); err == nil {
		claims.Email = email
	}
	var name string
	if err := parsed.Get("name", &name); err == nil {
		claims.Name = name
	}
	var groups interface{}
	if err := parsed.Get("groups", &groups); err == nil {
		switch g := groups.(type) {
		case []interface{}:
			for _, v := range g {
				claims.Groups = append(claims.Groups, fmt.Sprint(v))
			}
		case []string:
			claims.Groups = g
		}
	}
	if claims.Sub == "" {
		return nil, fmt.Errorf("jwt: missing sub claim")
	}
	return claims, nil
}

func TestRequireAuth_ValidToken(t *testing.T) {
	key, pubSet := newTestRSAKey(t)
	token := signTestJWT(t, key, map[string]interface{}{
		"email":  "test@example.com",
		"name":   "Test User",
		"groups": []string{"developers", "viewers"},
	}, time.Hour)

	claims, err := validateSessionTokenWithKeyset(pubSet,
		"https://auth.example.com/application/o/test/", token)
	if err != nil {
		t.Fatalf("validate token: %v", err)
	}
	if claims.Sub != "test-user" {
		t.Errorf("expected sub=test-user, got %s", claims.Sub)
	}
	if claims.Email != "test@example.com" {
		t.Errorf("expected email=test@example.com, got %s", claims.Email)
	}
	if len(claims.Groups) != 2 {
		t.Errorf("expected 2 groups, got %d", len(claims.Groups))
	}
}

func TestRequireAuth_ExpiredToken(t *testing.T) {
	key, pubSet := newTestRSAKey(t)
	token := signTestJWT(t, key, map[string]interface{}{
		"email": "test@example.com",
	}, -time.Hour)

	_, err := validateSessionTokenWithKeyset(pubSet,
		"https://auth.example.com/application/o/test/", token)
	if err == nil {
		t.Fatal("expected error for expired token")
	}
}

func TestRequireAuth_BadSignature(t *testing.T) {
	key, _ := newTestRSAKey(t)
	_, wrongPubSet := newTestRSAKey(t)
	token := signTestJWT(t, key, map[string]interface{}{
		"email": "test@example.com",
	}, time.Hour)

	_, err := validateSessionTokenWithKeyset(wrongPubSet,
		"https://auth.example.com/application/o/test/", token)
	if err == nil {
		t.Fatal("expected error for bad signature")
	}
}

func TestRequireAuth_NoSessionCookie_APIReturns401(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	config := AuthConfig{
		JWKSFetcher: &JWKSFetcher{},
		IssuerURL:   "https://auth.example.com/application/o/test/",
		AuthorizeURL: "https://auth.example.com/application/o/authorize/",
		SessionName: "test_session",
	}
	router.Use(RequireAuth(config))
	router.GET("/api/test", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for API without session, got %d", w.Code)
	}
}

func TestRequireAuth_NoSessionCookie_PageRedirects(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	config := AuthConfig{
		JWKSFetcher:  &JWKSFetcher{},
		IssuerURL:    "https://auth.example.com/application/o/test/",
		AuthorizeURL: "https://auth.example.com/application/o/authorize/",
		ClientID:     "test-client",
		RedirectURI:  "http://localhost:8080/callback",
		SessionName:  "test_session",
	}
	router.Use(RequireAuth(config))
	router.GET("/page", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/page", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("expected 302 redirect for page without session, got %d", w.Code)
	}
	location := w.Header().Get("Location")
	if location == "" {
		t.Error("expected Location header in redirect")
	}
}

func TestRequireAdmin_WithAdminGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(sessionClaimsKey), &SessionClaims{
			Sub:    "admin-user",
			Groups: []string{"admin", "developers"},
		})
		c.Next()
	})
	router.Use(RequireAdmin())
	router.GET("/api/admin/test", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/admin/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for admin user, got %d", w.Code)
	}
}

func TestRequireAdmin_WithoutAdminGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(sessionClaimsKey), &SessionClaims{
			Sub:    "regular-user",
			Groups: []string{"developers"},
		})
		c.Next()
	})
	router.Use(RequireAdmin())
	router.GET("/api/admin/test", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/admin/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for non-admin user, got %d", w.Code)
	}
}

func TestRequireAdmin_NoSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequireAdmin())
	router.GET("/api/admin/test", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/admin/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without session, got %d", w.Code)
	}
}

func TestGenerateState(t *testing.T) {
	state1, err := GenerateState()
	if err != nil {
		t.Fatalf("generate state: %v", err)
	}
	state2, err := GenerateState()
	if err != nil {
		t.Fatalf("generate state: %v", err)
	}
	if state1 == state2 {
		t.Error("expected different random states")
	}
	if len(state1) < 32 {
		t.Errorf("state too short: %d", len(state1))
	}
}

func TestBuildAuthorizeURL(t *testing.T) {
	config := AuthConfig{
		ClientID:     "test-client",
		RedirectURI:  "http://localhost:8080/callback",
		AuthorizeURL: "https://auth.example.com/application/o/authorize/",
	}
	url := BuildAuthorizeURL(config, "test-state-123")
	if url == "" {
		t.Fatal("expected non-empty URL")
	}
	if !contains(url, "response_type=code") {
		t.Error("missing response_type=code")
	}
	if !contains(url, "client_id=test-client") {
		t.Error("missing client_id")
	}
	if !contains(url, "state=test-state-123") {
		t.Error("missing state")
	}
	if !contains(url, "scope=openid") {
		t.Error("missing scope")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}