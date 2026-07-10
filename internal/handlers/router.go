package handlers

import (
	"io/fs"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type RouterOptions struct {
	ToolStore  ToolStore
	AdminToken string
}

func NewRouter(options ...RouterOptions) *gin.Engine {
	routerOptions := RouterOptions{}
	if len(options) > 0 {
		routerOptions = options[0]
	}

	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())

	router.GET("/healthz", health)
	router.GET("/api/health", health)
	if routerOptions.ToolStore != nil {
		registerToolRoutes(router, routerOptions.ToolStore, routerOptions.AdminToken)
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
