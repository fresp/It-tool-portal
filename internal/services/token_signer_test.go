package services

import (
	"crypto/rand"
	"crypto/rsa"
	"os"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v3/jwa"
	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/lestrrat-go/jwx/v3/jwt"
)

func newTestTokenSigner(t *testing.T) *TokenSigner {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return &TokenSigner{
		privateKey: privateKey,
		publicKey:  &privateKey.PublicKey,
		kid:        "test-kid",
		expiry:     90 * time.Second,
	}
}

func TestSignToken_ReturnsValidJWT(t *testing.T) {
	signer := newTestTokenSigner(t)

	tokenString, err := signer.SignToken(ToolScopedClaims{
		Sub:   "user-123",
		Email: "user@example.com",
		Name:  "Test User",
		Role:  "user",
		Aud:   "tool-456",
	})
	if err != nil {
		t.Fatalf("SignToken: %v", err)
	}

	if tokenString == "" {
		t.Fatal("expected non-empty token")
	}

	// Parse and verify the token.
	key, err := jwk.Import(signer.publicKey)
	if err != nil {
		t.Fatalf("import public key: %v", err)
	}
	_ = key.Set(jwk.KeyIDKey, "test-kid")

	parsed, err := jwt.Parse([]byte(tokenString), jwt.WithKey(jwa.RS256(), key))
	if err != nil {
		t.Fatalf("parse token: %v", err)
	}

	sub, ok := parsed.Subject()
	if !ok || sub != "user-123" {
		t.Errorf("sub = %q, want %q", sub, "user-123")
	}

	var email string
	if err := parsed.Get("email", &email); err != nil || email != "user@example.com" {
		t.Errorf("email = %q, want %q", email, "user@example.com")
	}

	var name string
	if err := parsed.Get("name", &name); err != nil || name != "Test User" {
		t.Errorf("name = %q, want %q", name, "Test User")
	}

	var role string
	if err := parsed.Get("role", &role); err != nil || role != "user" {
		t.Errorf("role = %q, want %q", role, "user")
	}

	aud, ok := parsed.Audience()
	if !ok || len(aud) != 1 || aud[0] != "tool-456" {
		t.Errorf("aud = %v, want [tool-456]", aud)
	}
}

func TestSignToken_UniqueJTI(t *testing.T) {
	signer := newTestTokenSigner(t)

	claims := ToolScopedClaims{
		Sub:   "user-123",
		Email: "user@example.com",
		Name:  "Test User",
		Role:  "user",
		Aud:   "tool-456",
	}

	token1, err := signer.SignToken(claims)
	if err != nil {
		t.Fatalf("SignToken 1: %v", err)
	}

	token2, err := signer.SignToken(claims)
	if err != nil {
		t.Fatalf("SignToken 2: %v", err)
	}

	if token1 == token2 {
		t.Error("expected unique tokens (different jti)")
	}
}

func TestSignToken_ExpiryWithinRange(t *testing.T) {
	signer := newTestTokenSigner(t)

	before := time.Now().Add(89 * time.Second)
	tokenString, err := signer.SignToken(ToolScopedClaims{
		Sub: "user-123",
		Aud: "tool-456",
	})
	if err != nil {
		t.Fatalf("SignToken: %v", err)
	}
	after := time.Now().Add(91 * time.Second)

	key, err := jwk.Import(signer.publicKey)
	if err != nil {
		t.Fatalf("import public key: %v", err)
	}
	_ = key.Set(jwk.KeyIDKey, "test-kid")

	parsed, err := jwt.Parse([]byte(tokenString), jwt.WithKey(jwa.RS256(), key))
	if err != nil {
		t.Fatalf("parse token: %v", err)
	}

	exp, ok := parsed.Expiration()
	if !ok || exp.Before(before) || exp.After(after) {
		t.Errorf("exp = %v, want between %v and %v", exp, before, after)
	}
}

func TestJWKS_ReturnsValidSet(t *testing.T) {
	signer := newTestTokenSigner(t)

	set, err := signer.JWKS()
	if err != nil {
		t.Fatalf("JWKS: %v", err)
	}

	if set.Len() != 1 {
		t.Fatalf("set.Len() = %d, want 1", set.Len())
	}

	key, ok := set.Key(0)
	if !ok {
		t.Fatal("expected key at index 0")
	}

	var kid string
	if err := key.Get(jwk.KeyIDKey, &kid); err != nil {
		t.Fatalf("get kid: %v", err)
	}
	if kid != "test-kid" {
		t.Errorf("kid = %q, want %q", kid, "test-kid")
	}

	var alg jwa.SignatureAlgorithm
	if err := key.Get(jwk.AlgorithmKey, &alg); err != nil {
		t.Fatalf("get alg: %v", err)
	}
	if alg != jwa.RS256() {
		t.Errorf("alg = %v, want %v", alg, jwa.RS256())
	}
}

func TestNewTokenSigner_GeneratesKeyWhenNoneExists(t *testing.T) {
	// Clear env vars to force key generation.
	os.Unsetenv("JWT_PRIVATE_KEY_PATH")
	os.Unsetenv("JWT_KID")
	os.Unsetenv("JWT_EXPIRY_SECONDS")

	signer, err := NewTokenSigner()
	if err != nil {
		t.Fatalf("NewTokenSigner: %v", err)
	}

	if signer.privateKey == nil {
		t.Error("expected generated private key")
	}
	if signer.kid != "v1" {
		t.Errorf("kid = %q, want %q", signer.kid, "v1")
	}
	if signer.expiry != 90*time.Second {
		t.Errorf("expiry = %v, want %v", signer.expiry, 90*time.Second)
	}
}
