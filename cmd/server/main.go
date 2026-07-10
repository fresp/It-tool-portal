package main

import (
	"context"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/fresp/it-tools-portal/internal/handlers"
	"github.com/fresp/it-tools-portal/internal/middleware"
	"github.com/fresp/it-tools-portal/internal/repositories"
	"github.com/fresp/it-tools-portal/internal/services"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	mongoURI := os.Getenv("MONGODB_URI")
	if mongoURI == "" {
		slog.Error("MONGODB_URI is required")
		os.Exit(1)
	}
	databaseName := os.Getenv("MONGODB_DB")
	if databaseName == "" {
		databaseName = "it_tools_portal"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := mongo.Connect(options.Client().ApplyURI(mongoURI))
	if err != nil {
		slog.Error("connect mongodb", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := client.Disconnect(context.Background()); err != nil {
			slog.Error("disconnect mongodb", "error", err)
		}
	}()

	toolRepository := repositories.NewMongoToolRepository(client.Database(databaseName).Collection("tools"))
	auditRepository := repositories.NewMongoAuditRepository(client.Database(databaseName).Collection("audit_logs"))
	if err := auditRepository.EnsureIndexes(ctx); err != nil {
		slog.Error("ensure audit indexes", "error", err)
		os.Exit(1)
	}

	var authConfig *middleware.AuthConfig
	if jwksURL := os.Getenv("AUTHENTIK_JWKS_URL"); jwksURL != "" {
		cacheTTL := envDuration("JWKS_CACHE_TTL_SECONDS", 3600)
		jwksFetcher := middleware.NewJWKSFetcher(jwksURL, cacheTTL)
		if err := jwksFetcher.Init(context.Background()); err != nil {
			slog.Error("init jwks fetcher", "error", err)
			os.Exit(1)
		}

		authConfig = &middleware.AuthConfig{
			JWKSFetcher:  jwksFetcher,
			ClientID:     os.Getenv("AUTHENTIK_CLIENT_ID"),
			ClientSecret: os.Getenv("AUTHENTIK_CLIENT_SECRET"),
			RedirectURI:  os.Getenv("AUTHENTIK_REDIRECT_URI"),
			IssuerURL:    os.Getenv("AUTHENTIK_ISSUER_URL"),
			AuthorizeURL: os.Getenv("AUTHENTIK_AUTHORIZE_URL"),
			TokenURL:     os.Getenv("AUTHENTIK_TOKEN_URL"),
			SessionName:  "it_tools_session",
		}
	}

	// Initialize token signer.
	tokenSigner, err := services.NewTokenSigner()
	if err != nil {
		slog.Error("init token signer", "error", err)
		os.Exit(1)
	}

	router := handlers.NewRouter(handlers.RouterOptions{
		ToolStore:   toolRepository,
		AuditStore:  auditRepository,
		TokenSigner: tokenSigner,
		AuthConfig:  authConfig,
	})
	if err := router.Run(":" + port); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func envDuration(key string, defaultSeconds int) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return time.Duration(defaultSeconds) * time.Second
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return time.Duration(defaultSeconds) * time.Second
	}
	return time.Duration(n) * time.Second
}
