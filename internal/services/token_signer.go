package services

import (
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/lestrrat-go/jwx/v3/jwa"
	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/lestrrat-go/jwx/v3/jwt"
)

// ToolScopedClaims holds the claims for a tool-scoped JWT.
type ToolScopedClaims struct {
	Sub   string `json:"sub"`
	Email string `json:"email"`
	Name  string `json:"name"`
	Role  string `json:"role"`
	Aud   string `json:"aud"`
}

// TokenSigner mints short-lived tool-scoped JWTs signed with this app's private key.
type TokenSigner struct {
	privateKey *rsa.PrivateKey
	publicKey  *rsa.PublicKey
	kid        string
	expiry     time.Duration
}

// NewTokenSigner creates a TokenSigner from environment configuration.
// Loads RSA key from JWT_PRIVATE_KEY_PATH or generates one for dev.
func NewTokenSigner() (*TokenSigner, error) {
	kid := os.Getenv("JWT_KID")
	if kid == "" {
		kid = "v1"
	}

	expirySeconds := 90
	if v := os.Getenv("JWT_EXPIRY_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			expirySeconds = n
		}
	}

	keyPath := os.Getenv("JWT_PRIVATE_KEY_PATH")
	var privateKey *rsa.PrivateKey

	if keyPath != "" {
		keyData, err := os.ReadFile(keyPath)
		if err == nil {
			key, err := jwk.ParseKey(keyData)
			if err == nil {
				var rawKey *rsa.PrivateKey
				if err := jwk.Export(key, &rawKey); err == nil {
					privateKey = rawKey
				}
			}
		}
	}

	if privateKey == nil {
		slog.Warn("token_signer: no valid RSA key found, generating ephemeral key for dev use")
		var err error
		privateKey, err = rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			return nil, fmt.Errorf("generate RSA key: %w", err)
		}
	}

	return &TokenSigner{
		privateKey: privateKey,
		publicKey:  &privateKey.PublicKey,
		kid:        kid,
		expiry:     time.Duration(expirySeconds) * time.Second,
	}, nil
}

// SignToken mints a new tool-scoped JWT with the given claims.
func (s *TokenSigner) SignToken(claims ToolScopedClaims) (string, error) {
	now := time.Now().UTC()
	jti, err := uuid.NewRandom()
	if err != nil {
		return "", fmt.Errorf("generate jti: %w", err)
	}

	token, err := jwt.NewBuilder().
		Subject(claims.Sub).
		IssuedAt(now).
		Expiration(now.Add(s.expiry)).
		Audience([]string{claims.Aud}).
		Claim("email", claims.Email).
		Claim("name", claims.Name).
		Claim("role", claims.Role).
		JwtID(jti.String()).
		Build()
	if err != nil {
		return "", fmt.Errorf("build token: %w", err)
	}

	key, err := jwk.Import(s.privateKey)
	if err != nil {
		return "", fmt.Errorf("import private key: %w", err)
	}
	_ = key.Set(jwk.KeyIDKey, s.kid)
	_ = key.Set(jwk.AlgorithmKey, "RS256")

	signed, err := jwt.Sign(token, jwt.WithKey(jwa.RS256(), key))
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}

	return string(signed), nil
}

// JWKS returns the public key set for the /.well-known/jwks.json endpoint.
func (s *TokenSigner) JWKS() (jwk.Set, error) {
	pubKey, err := jwk.Import(s.publicKey)
	if err != nil {
		return nil, fmt.Errorf("import public key: %w", err)
	}
	_ = pubKey.Set(jwk.KeyIDKey, s.kid)
	_ = pubKey.Set(jwk.AlgorithmKey, "RS256")
	_ = pubKey.Set(jwk.KeyUsageKey, "sig")

	set := jwk.NewSet()
	set.AddKey(pubKey)
	return set, nil
}
