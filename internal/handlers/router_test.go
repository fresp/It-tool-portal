package handlers

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRouter_health_whenRequested(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouter()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /healthz status = %d, want %d", recorder.Code, http.StatusOK)
	}

	var body map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("GET /healthz body JSON decode failed: %v", err)
	}

	if body["status"] != "ok" || body["service"] != "it-tools-portal" {
		t.Fatalf("GET /healthz body = %#v, want status ok and service it-tools-portal", body)
	}
}

func TestRouter_frontendFallback_whenUnknownPathRequested(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouter()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/tools/example", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /tools/example status = %d, want %d", recorder.Code, http.StatusOK)
	}

	contentType := recorder.Header().Get("Content-Type")
	if !strings.Contains(contentType, "text/html") {
		t.Fatalf("GET /tools/example Content-Type = %q, want text/html", contentType)
	}

	if !strings.Contains(recorder.Body.String(), "IT Tools Portal") {
		t.Fatalf("GET /tools/example body did not contain frontend placeholder marker")
	}
}

func TestRouter_frontendAsset_whenRequested(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouter()

	entries, err := fs.ReadDir(frontendAssets, "web/dist/assets")
	if err != nil {
		t.Fatalf("read embedded asset directory: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("embedded asset directory is empty")
	}

	assetPath := "/assets/" + entries[0].Name()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, assetPath, nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("GET %s status = %d, want %d", assetPath, recorder.Code, http.StatusOK)
	}
	if strings.Contains(recorder.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("GET %s returned HTML fallback, want static asset", assetPath)
	}
	if recorder.Body.Len() == 0 {
		t.Fatalf("GET %s returned empty asset body", assetPath)
	}
}
