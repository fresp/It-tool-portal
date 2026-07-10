package handlers

import (
	"io/fs"
	"net/http"
	"os"
	"strings"

	"github.com/fresp/it-tools-portal/internal/middleware"
	"github.com/gin-gonic/gin"
)

type RouterOptions struct {
	ToolStore  ToolStore
	AuthConfig *middleware.AuthConfig
}

func NewRouter(options ...RouterOptions) *gin.Engine {
	routerOptions := RouterOptions{}
	if len(options) > 0 {
		routerOptions = options[0]
	}

	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())

	// CSP frame-ancestors for embeddability.
	router.Use(frameAncestorsMiddleware())

	router.GET("/healthz", health)
	router.GET("/api/health", health)

	// Auth routes (OIDC login/callback/logout).
	if routerOptions.AuthConfig != nil {
		registerAuthRoutes(router, *routerOptions.AuthConfig)
	}

	// Protected API routes.
	if routerOptions.ToolStore != nil {
		registerToolRoutes(router, routerOptions.ToolStore, routerOptions.AuthConfig)
	}
	router.GET("/assets/*filepath", frontendAsset)
	router.NoRoute(frontendFallback)

	return router
}

func health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"service": "it-tools-portal",
	})
}

func frontendFallback(c *gin.Context) {
	if strings.HasPrefix(c.Request.URL.Path, "/api/") {
		c.Status(http.StatusNotFound)
		return
	}

	indexHTML, err := fs.ReadFile(frontendAssets, "web/dist/index.html")
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}

	c.Data(http.StatusOK, "text/html; charset=utf-8", indexHTML)
}

func frontendAsset(c *gin.Context) {
	assetPath := strings.TrimPrefix(c.Param("filepath"), "/")
	assetFile, err := frontendAssets.Open("web/dist/assets/" + assetPath)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	defer assetFile.Close()

	c.DataFromReader(http.StatusOK, -1, contentTypeFor(assetPath), assetFile, nil)
}

func contentTypeFor(path string) string {
	if strings.HasSuffix(path, ".css") {
		return "text/css; charset=utf-8"
	}
	if strings.HasSuffix(path, ".js") {
		return "text/javascript; charset=utf-8"
	}
	return "application/octet-stream"
}

func frameAncestorsMiddleware() gin.HandlerFunc {
	ancestors := os.Getenv("FRAME_ANCESTORS")
	if ancestors == "" {
		return func(c *gin.Context) { c.Next() }
	}
	return func(c *gin.Context) {
		c.Header("Content-Security-Policy", "frame-ancestors "+ancestors)
		c.Next()
	}
}
