package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/fresp/it-tools-portal/internal/services"
	"github.com/gin-gonic/gin"
)

type jwksHandlers struct {
	signer *services.TokenSigner
}

func registerJWKSRoute(router *gin.Engine, signer *services.TokenSigner) {
	h := jwksHandlers{signer: signer}
	router.GET("/.well-known/jwks.json", h.jwks)
}

// jwks returns the public JWK set for token verification by target tools.
func (h jwksHandlers) jwks(c *gin.Context) {
	set, err := h.signer.JWKS()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load keys"})
		return
	}

	jsonBytes, err := json.Marshal(set)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to serialize keys"})
		return
	}

	c.Data(http.StatusOK, "application/json", jsonBytes)
}
