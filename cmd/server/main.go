package main

import (
	"log/slog"
	"os"

	"github.com/fresp/it-tools-portal/internal/handlers"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	router := handlers.NewRouter()
	if err := router.Run(":" + port); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
