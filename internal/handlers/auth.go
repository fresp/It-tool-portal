package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/fresp/it-tools-portal/internal/middleware"
	"github.com/gin-gonic/gin"
)

type authHandlers struct {
	config middleware.AuthConfig
}

func registerAuthRoutes(router *gin.Engine, config middleware.AuthConfig) {
	h := authHandlers{config: config}

	router.GET("/auth/login", h.login)
	router.GET("/callback", h.callback)
	router.POST("/auth/logout", h.logout)
}

// login redirects the user to Authentik's authorize endpoint.
func (h authHandlers) login(c *gin.Context) {
	state, err := middleware.GenerateState()
	if err != nil {
		slog.Error("auth: generate state", "error", err)
		c.String(http.StatusInternalServerError, "internal error")
		return
	}
	c.SetCookie("oidc_state", state, 300, "/", "", true, true)
	c.Redirect(http.StatusFound, middleware.BuildAuthorizeURL(h.config, state))
}

// callback completes the OIDC Authorization Code flow.
func (h authHandlers) callback(c *gin.Context) {
	// Verify state parameter to prevent CSRF.
	expectedState, err := c.Cookie("oidc_state")
	if err != nil || expectedState == "" {
		c.String(http.StatusBadRequest, "missing state")
		return
	}
	// Clear the state cookie immediately.
	c.SetCookie("oidc_state", "", -1, "/", "", true, true)

	state := c.Query("state")
	if state == "" || state != expectedState {
		c.String(http.StatusBadRequest, "state mismatch")
		return
	}

	code := c.Query("code")
	if code == "" {
		errDesc := c.Query("error_description")
		if errDesc == "" {
			errDesc = c.Query("error")
		}
		slog.Warn("auth: callback missing code", "error", errDesc)
		c.String(http.StatusBadRequest, "authorization failed")
		return
	}

	// Exchange the authorization code for tokens.
	tokenResponse, err := exchangeCode(c, h.config, code)
	if err != nil {
		slog.Error("auth: token exchange failed", "error", err)
		c.String(http.StatusInternalServerError, "authentication failed")
		return
	}

	// Validate the id_token against Authentik's JWKS.
	claims, err := middleware.ValidateSessionToken(c.Request.Context(), h.config, tokenResponse.IDToken)
	if err != nil {
		slog.Error("auth: id_token validation failed", "error", err)
		c.String(http.StatusInternalServerError, "authentication failed")
		return
	}

	// Set the session cookie with the id_token as the session value.
	// The id_token is already a JWT signed by Authentik, so we can reuse it
	// as the session token — the middleware will validate it against JWKS.
	maxAge := 86400 // 24 hours
	c.SetCookie(
		h.config.SessionName,
		tokenResponse.IDToken,
		maxAge,
		"/",
		"",
		true, // secure
		true, // httpOnly
	)

	slog.Info("auth: user logged in", "sub", claims.Sub, "email", claims.Email)

	// Redirect to the app root.
	c.Redirect(http.StatusFound, "/")
}

// logout clears the session cookie.
func (h authHandlers) logout(c *gin.Context) {
	c.SetCookie(h.config.SessionName, "", -1, "/", "", true, true)
	c.JSON(http.StatusOK, gin.H{"status": "logged_out"})
}

type tokenExchangeResponse struct {
	IDToken string `json:"id_token"`
}

func exchangeCode(c *gin.Context, config middleware.AuthConfig, code string) (*tokenExchangeResponse, error) {
	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", config.RedirectURI)
	data.Set("client_id", config.ClientID)
	data.Set("client_secret", config.ClientSecret)

	req, err := http.NewRequestWithContext(
		c.Request.Context(),
		http.MethodPost,
		config.TokenURL,
		strings.NewReader(data.Encode()),
	)
	if err != nil {
		return nil, fmt.Errorf("create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		IDToken string `json:"id_token"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse token response: %w", err)
	}
	if result.IDToken == "" {
		return nil, fmt.Errorf("token response missing id_token")
	}

	return &tokenExchangeResponse{IDToken: result.IDToken}, nil
}