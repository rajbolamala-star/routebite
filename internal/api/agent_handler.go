package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// AgentSearch handles POST /v1/agent/search.
func (h *Handler) AgentSearch(c *gin.Context) {
	var req AgentSearchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, searchErr := h.runAgentSearch(c.Request.Context(), req)
	if searchErr != nil {
		c.JSON(searchErr.status, gin.H{"error": searchErr.message})
		return
	}

	c.JSON(http.StatusOK, resp)
}
