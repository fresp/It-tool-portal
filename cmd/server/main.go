package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/fresp/it-tools-portal/internal/handlers"
	"github.com/fresp/it-tools-portal/internal/repositories"
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
	if err := toolRepository.EnsureIndexes(ctx); err != nil {
		slog.Error("ensure tool indexes", "error", err)
		os.Exit(1)
	}

	router := handlers.NewRouter(handlers.RouterOptions{
		ToolStore:  toolRepository,
		AdminToken: os.Getenv("ADMIN_TOKEN"),
	})
	if err := router.Run(":" + port); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
