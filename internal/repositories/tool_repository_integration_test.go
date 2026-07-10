//go:build integration

package repositories

import (
	"context"
	"fmt"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/fresp/it-tools-portal/internal/models"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func TestMongoToolRepositoryCRUDAndIndexes(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	repository, cleanup := newIntegrationToolRepository(t, ctx)
	defer cleanup()

	if err := repository.EnsureIndexes(ctx); err != nil {
		t.Fatalf("EnsureIndexes() error = %v, want nil", err)
	}

	indexNames, err := repository.IndexNames(ctx)
	if err != nil {
		t.Fatalf("IndexNames() error = %v, want nil", err)
	}
	if !slices.Contains(indexNames, "idx_tools_is_active") || !slices.Contains(indexNames, "idx_tools_allowed_groups") {
		t.Fatalf("IndexNames() = %#v, want active and allowed groups indexes", indexNames)
	}

	created, err := repository.Create(ctx, models.CreateToolRequest{
		Name:          "Docs",
		BaseURL:       "https://docs.example.com",
		IconURL:       "https://docs.example.com/icon.svg",
		AllowedGroups: []string{"dev", "ops"},
	})
	if err != nil {
		t.Fatalf("Create() error = %v, want nil", err)
	}
	if created.ID == "" || !created.IsActive || created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() {
		t.Fatalf("Create() = %#v, want id, active flag, timestamps", created)
	}

	listed, err := repository.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v, want nil", err)
	}
	if len(listed) != 1 || listed[0].ID != created.ID {
		t.Fatalf("List() = %#v, want created tool only", listed)
	}

	fetched, err := repository.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	if fetched.Name != "Docs" {
		t.Fatalf("Get().Name = %q, want Docs", fetched.Name)
	}

	updatedName := "Knowledge Base"
	updated, err := repository.Update(ctx, created.ID, models.UpdateToolRequest{Name: &updatedName})
	if err != nil {
		t.Fatalf("Update() error = %v, want nil", err)
	}
	if updated.Name != updatedName || !updated.UpdatedAt.After(updated.CreatedAt) {
		t.Fatalf("Update() = %#v, want updated name and later UpdatedAt", updated)
	}

	if err := repository.Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete() error = %v, want nil", err)
	}
	if _, err := repository.Get(ctx, created.ID); err == nil {
		t.Fatalf("Get() after Delete() error = nil, want not found error")
	}
}

func TestMongoToolRepositoryListAvailableFiltersByGroupsAndActive(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	repository, cleanup := newIntegrationToolRepository(t, ctx)
	defer cleanup()

	activeDev, err := repository.Create(ctx, models.CreateToolRequest{
		Name:          "Dev Docs",
		BaseURL:       "https://dev.example.com",
		IconURL:       "https://dev.example.com/icon.svg",
		AllowedGroups: []string{"dev"},
	})
	if err != nil {
		t.Fatalf("Create(activeDev) error = %v, want nil", err)
	}
	_, err = repository.Create(ctx, models.CreateToolRequest{
		Name:          "Finance",
		BaseURL:       "https://finance.example.com",
		IconURL:       "https://finance.example.com/icon.svg",
		AllowedGroups: []string{"finance"},
	})
	if err != nil {
		t.Fatalf("Create(finance) error = %v, want nil", err)
	}
	inactive := false
	_, err = repository.Create(ctx, models.CreateToolRequest{
		Name:          "Inactive Dev",
		BaseURL:       "https://inactive.example.com",
		IconURL:       "https://inactive.example.com/icon.svg",
		AllowedGroups: []string{"dev"},
		IsActive:      &inactive,
	})
	if err != nil {
		t.Fatalf("Create(inactive) error = %v, want nil", err)
	}

	available, err := repository.ListAvailable(ctx, []string{"dev", "ops"})
	if err != nil {
		t.Fatalf("ListAvailable() error = %v, want nil", err)
	}
	if len(available) != 1 || available[0].ID != activeDev.ID {
		t.Fatalf("ListAvailable() = %#v, want only active dev tool", available)
	}
}

func newIntegrationToolRepository(t *testing.T, ctx context.Context) (*MongoToolRepository, func()) {
	t.Helper()

	uri := os.Getenv("MONGODB_URI")
	if uri == "" {
		t.Skip("MONGODB_URI is required for integration tests")
	}

	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatalf("connect MongoDB: %v", err)
	}
	databaseName := fmt.Sprintf("it_tools_portal_test_%d", time.Now().UnixNano())
	database := client.Database(databaseName)
	repository := NewMongoToolRepository(database.Collection("tools"))

	cleanup := func() {
		if err := database.Drop(context.Background()); err != nil {
			t.Logf("drop test database %s: %v", databaseName, err)
		}
		if err := client.Disconnect(context.Background()); err != nil {
			t.Logf("disconnect MongoDB: %v", err)
		}
	}
	return repository, cleanup
}

func TestMongoToolRepositoryNotFoundErrorMatches(t *testing.T) {
	if !IsToolNotFound(ErrToolNotFound) {
		t.Fatalf("IsToolNotFound(ErrToolNotFound) = false, want true")
	}
	if IsToolNotFound(bson.ErrDecodeToNil) {
		t.Fatalf("IsToolNotFound(unrelated error) = true, want false")
	}
}
