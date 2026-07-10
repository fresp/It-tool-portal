package middleware

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lestrrat-go/jwx/v3/jwt"
)

// SessionClaims holds the user identity extracted from a validated Authentik JWT.
type SessionClaims struct {
	Sub    string   `json:"sub"`
	Email  string   `json:"email"`
	Name   string   `json:"name"`
	Groups []string `json:"groups"`
}

// AuthConfig holds configuration for the auth middleware and OIDC flow.
type AuthConfig struct {
	JWKSFetcher  *JWKSFetcher
	ClientID     string
	ClientSecret string
	RedirectURI  string
	IssuerURL    string
	AuthorizeURL string
	TokenURL     string
	SessionName  string
}

type contextKey string

const sessionClaimsKey contextKey = "session_claims"

// GetSessionClaims extracts the authenticated user claims from the gin context.
func GetSessionClaims(c *gin.Context) (*SessionClaims, bool) {
	claimsVal, exists := c.Get(string(sessionClaimsKey))
	if !exists {
		return nil, false
	}
	claims, ok := claimsVal.(*SessionClaims)
	return claims, ok
}

// RequireAuth is a Gin middleware that validates the session cookie JWT
// against Authentik's JWKS. On failure, it redirects to login for page
// routes and returns 401 for API routes.
func RequireAuth(config AuthConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString, err := c.Cookie(config.SessionName)
		if err != nil || tokenString == "" {
			deny(c, config, "no session cookie")
			return
		}

		claims, err := ValidateSessionToken(c.Request.Context(), config, tokenString)
		if err != nil {
			slog.Warn("auth: invalid session token", "error", err)
			deny(c, config, "invalid session")
			return
		}

		c.Set(string(sessionClaimsKey), claims)
		c.Next()
	}
}

// RequireAdmin is a Gin middleware that checks the authenticated user
// has the "admin" group. Must be used after RequireAuth.
func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		claimsVal, exists := c.Get(string(sessionClaimsKey))
		if !exists {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		claims, ok := claimsVal.(*SessionClaims)
		if !ok {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		for _, g := range claims.Groups {
			if strings.EqualFold(g, "admin") {
				c.Next()
				return
			}
		}
		c.AbortWithStatus(http.StatusForbidden)
	}
}

// GenerateState creates a random OIDC state parameter.
func GenerateState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// BuildAuthorizeURL constructs the Authentik authorize URL with required OIDC params.
func BuildAuthorizeURL(config AuthConfig, state string) string {
	params := url.Values{}
	params.Set("response_type", "code")
	params.Set("client_id", config.ClientID)
	params.Set("redirect_uri", config.RedirectURI)
	params.Set("scope", "openid profile email")
	params.Set("state", state)
	return config.AuthorizeURL + "?" + params.Encode()
}

func ValidateSessionToken(ctx context.Context, config AuthConfig, tokenString string) (*SessionClaims, error) {
	keyset, err := config.JWKSFetcher.KeySet(ctx)
	if err != nil {
		return nil, fmt.Errorf("jwks fetch: %w", err)
	}

	parsed, err := jwt.Parse([]byte(tokenString),
		jwt.WithKeySet(keyset),
		jwt.WithIssuer(config.IssuerURL),
		jwt.WithAcceptableSkew(30*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("jwt parse: %w", err)
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
func deny(c *gin.Context, config AuthConfig, reason string) {
	if isAPIRequest(c) {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	state, err := GenerateState()
	if err != nil {
		slog.Error("auth: generate state", "error", err)
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	// Store state in a short-lived cookie for CSRF protection.
	c.SetCookie("oidc_state", state, 300, "/", "", true, true)
	redirectURL := BuildAuthorizeURL(config, state)
	c.Redirect(http.StatusFound, redirectURL)
	c.Abort()
}

func isAPIRequest(c *gin.Context) bool {
	return strings.HasPrefix(c.Request.URL.Path, "/api/")
}