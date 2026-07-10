package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fresp/it-tools-portal/internal/models"
	"github.com/fresp/it-tools-portal/internal/repositories"
	"github.com/gin-gonic/gin"
)

func TestToolsAdminRoutesRejectUnauthenticatedCallers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newFakeToolStore()
	router := NewRouter(RouterOptions{ToolStore: store, AdminToken: "secret"})

	requestBody := `{"name":"Docs","base_url":"https://docs.example.com","icon_url":"https://docs.example.com/icon.svg","allowed_groups":["dev"]}`
	tests := []struct {
		name       string
		token      string
		wantStatus int
	}{
		{name: "missing token", wantStatus: http.StatusUnauthorized},
		{name: "wrong token", token: "wrong", wantStatus: http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/api/admin/tools", strings.NewReader(requestBody))
			request.Header.Set("Content-Type", "application/json")
			if tt.token != "" {
				request.Header.Set("X-Admin-Token", tt.token)
			}

			router.ServeHTTP(recorder, request)

			if recorder.Code != tt.wantStatus {
				t.Fatalf("POST /api/admin/tools status = %d, want %d", recorder.Code, tt.wantStatus)
			}
			if store.createCalls != 0 {
				t.Fatalf("store create calls = %d, want 0", store.createCalls)
			}
		})
	}
}

func TestToolCRUDLifecycle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newFakeToolStore()
	router := NewRouter(RouterOptions{ToolStore: store, AdminToken: "secret"})

	createBody := `{"name":"Docs","base_url":"https://docs.example.com","icon_url":"https://docs.example.com/icon.svg","allowed_groups":["dev"]}`
	createRecorder := serveJSON(t, router, http.MethodPost, "/api/admin/tools", createBody, "secret")
	if createRecorder.Code != http.StatusCreated {
		t.Fatalf("POST /api/admin/tools status = %d, want %d", createRecorder.Code, http.StatusCreated)
	}
	created := decodeTool(t, createRecorder.Body.Bytes())

	listRecorder := serveJSON(t, router, http.MethodGet, "/api/admin/tools", "", "secret")
	if listRecorder.Code != http.StatusOK {
		t.Fatalf("GET /api/admin/tools status = %d, want %d", listRecorder.Code, http.StatusOK)
	}
	listed := decodeTools(t, listRecorder.Body.Bytes())
	if len(listed) != 1 || listed[0].ID != created.ID {
		t.Fatalf("GET /api/admin/tools body = %#v, want created tool", listed)
	}

	getRecorder := serveJSON(t, router, http.MethodGet, "/api/admin/tools/"+created.ID, "", "secret")
	if getRecorder.Code != http.StatusOK {
		t.Fatalf("GET /api/admin/tools/:id status = %d, want %d", getRecorder.Code, http.StatusOK)
	}

	updateBody := `{"name":"Knowledge Base"}`
	updateRecorder := serveJSON(t, router, http.MethodPut, "/api/admin/tools/"+created.ID, updateBody, "secret")
	if updateRecorder.Code != http.StatusOK {
		t.Fatalf("PUT /api/admin/tools/:id status = %d, want %d", updateRecorder.Code, http.StatusOK)
	}
	updated := decodeTool(t, updateRecorder.Body.Bytes())
	if updated.Name != "Knowledge Base" {
		t.Fatalf("updated name = %q, want Knowledge Base", updated.Name)
	}

	deleteRecorder := serveJSON(t, router, http.MethodDelete, "/api/admin/tools/"+created.ID, "", "secret")
	if deleteRecorder.Code != http.StatusNoContent {
		t.Fatalf("DELETE /api/admin/tools/:id status = %d, want %d", deleteRecorder.Code, http.StatusNoContent)
	}

	missingRecorder := serveJSON(t, router, http.MethodGet, "/api/admin/tools/"+created.ID, "", "secret")
	if missingRecorder.Code != http.StatusNotFound {
		t.Fatalf("GET deleted tool status = %d, want %d", missingRecorder.Code, http.StatusNotFound)
	}
}

func TestListAvailableToolsFiltersByCallerGroupsAndActiveFlag(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newFakeToolStore()
	_, _ = store.Create(context.Background(), models.CreateToolRequest{Name: "Dev", BaseURL: "https://dev.example.com", IconURL: "https://dev.example.com/icon.svg", AllowedGroups: []string{"dev"}})
	_, _ = store.Create(context.Background(), models.CreateToolRequest{Name: "Finance", BaseURL: "https://finance.example.com", IconURL: "https://finance.example.com/icon.svg", AllowedGroups: []string{"finance"}})
	inactive := false
	_, _ = store.Create(context.Background(), models.CreateToolRequest{Name: "Inactive", BaseURL: "https://inactive.example.com", IconURL: "https://inactive.example.com/icon.svg", AllowedGroups: []string{"dev"}, IsActive: &inactive})
	router := NewRouter(RouterOptions{ToolStore: store, AdminToken: "secret"})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/tools", nil)
	request.Header.Set("X-User-Groups", "dev, ops")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /api/tools status = %d, want %d", recorder.Code, http.StatusOK)
	}
	tools := decodeTools(t, recorder.Body.Bytes())
	if len(tools) != 1 || tools[0].Name != "Dev" {
		t.Fatalf("GET /api/tools body = %#v, want active dev tool only", tools)
	}
}

func TestToolValidationRejectsInvalidInput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newFakeToolStore()
	router := NewRouter(RouterOptions{ToolStore: store, AdminToken: "secret"})

	tests := []struct {
		name string
		body string
	}{
		{name: "empty name", body: `{"name":"","base_url":"https://docs.example.com","icon_url":"https://docs.example.com/icon.svg","allowed_groups":["dev"]}`},
		{name: "invalid base url", body: `{"name":"Docs","base_url":"not-a-url","icon_url":"https://docs.example.com/icon.svg","allowed_groups":["dev"]}`},
		{name: "empty groups", body: `{"name":"Docs","base_url":"https://docs.example.com","icon_url":"https://docs.example.com/icon.svg","allowed_groups":[]}`},
		{name: "invalid health check url", body: `{"name":"Docs","base_url":"https://docs.example.com","icon_url":"https://docs.example.com/icon.svg","allowed_groups":["dev"],"health_check_url":"not-a-url"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := serveJSON(t, router, http.MethodPost, "/api/admin/tools", tt.body, "secret")
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("POST /api/admin/tools status = %d, want %d", recorder.Code, http.StatusBadRequest)
			}
		})
	}
	if len(store.tools) != 0 {
		t.Fatalf("stored tools = %d, want 0", len(store.tools))
	}
}

type fakeToolStore struct {
	mu          sync.Mutex
	tools       map[string]models.Tool
	createCalls int
}

func newFakeToolStore() *fakeToolStore {
	return &fakeToolStore{tools: map[string]models.Tool{}}
}

func (s *fakeToolStore) Create(_ context.Context, request models.CreateToolRequest) (models.Tool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.createCalls++
	if err := request.Validate(); err != nil {
		return models.Tool{}, err
	}
	active := true
	if request.IsActive != nil {
		active = *request.IsActive
	}
	id := "tool-" + request.Name
	tool := models.Tool{ID: id, Name: request.Name, BaseURL: request.BaseURL, IconURL: request.IconURL, AllowedGroups: request.AllowedGroups, HealthCheckURL: request.HealthCheckURL, IsActive: active, CreatedAt: time.Unix(1, 0), UpdatedAt: time.Unix(1, 0)}
	s.tools[id] = tool
	return tool, nil
}

func (s *fakeToolStore) List(_ context.Context) ([]models.Tool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tools := make([]models.Tool, 0, len(s.tools))
	for _, tool := range s.tools {
		tools = append(tools, tool)
	}
	return tools, nil
}

func (s *fakeToolStore) ListAvailable(_ context.Context, groups []string) ([]models.Tool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	wanted := map[string]struct{}{}
	for _, group := range groups {
		wanted[group] = struct{}{}
	}
	tools := make([]models.Tool, 0)
	for _, tool := range s.tools {
		if !tool.IsActive {
			continue
		}
		for _, group := range tool.AllowedGroups {
			if _, ok := wanted[group]; ok {
				tools = append(tools, tool)
				break
			}
		}
	}
	return tools, nil
}

func (s *fakeToolStore) Get(_ context.Context, id string) (models.Tool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tool, ok := s.tools[id]
	if !ok {
		return models.Tool{}, repositories.ErrToolNotFound
	}
	return tool, nil
}

func (s *fakeToolStore) Update(_ context.Context, id string, request models.UpdateToolRequest) (models.Tool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tool, ok := s.tools[id]
	if !ok {
		return models.Tool{}, repositories.ErrToolNotFound
	}
	if err := request.Validate(); err != nil {
		return models.Tool{}, err
	}
	if request.Name != nil {
		tool.Name = *request.Name
	}
	if request.BaseURL != nil {
		tool.BaseURL = *request.BaseURL
	}
	if request.IconURL != nil {
		tool.IconURL = *request.IconURL
	}
	if request.AllowedGroups != nil {
		tool.AllowedGroups = request.AllowedGroups
	}
	if request.HealthCheckURL != nil {
		tool.HealthCheckURL = request.HealthCheckURL
	}
	if request.IsActive != nil {
		tool.IsActive = *request.IsActive
	}
	tool.UpdatedAt = time.Unix(2, 0)
	s.tools[id] = tool
	return tool, nil
}

func (s *fakeToolStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.tools[id]; !ok {
		return repositories.ErrToolNotFound
	}
	delete(s.tools, id)
	return nil
}

func serveJSON(t *testing.T, router http.Handler, method string, path string, body string, adminToken string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	if adminToken != "" {
		request.Header.Set("X-Admin-Token", adminToken)
	}
	router.ServeHTTP(recorder, request)
	return recorder
}

func decodeTool(t *testing.T, body []byte) models.Tool {
	t.Helper()
	var tool models.Tool
	if err := json.Unmarshal(body, &tool); err != nil {
		t.Fatalf("decode tool JSON: %v", err)
	}
	return tool
}

func decodeTools(t *testing.T, body []byte) []models.Tool {
	t.Helper()
	var tools []models.Tool
	if err := json.Unmarshal(body, &tools); err != nil {
		t.Fatalf("decode tools JSON: %v", err)
	}
	return tools
}
