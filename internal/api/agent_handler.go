package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// AgentSearch handles POST /v1/agent/search.
func (h *Handler) AgentSearch(c *gin.Context) {
	var req AgentSearchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, err.Error())
		return
	}

	resp, searchErr := h.runAgentSearch(c.Request.Context(), req)
	if searchErr != nil {
		writeError(c, searchErr.status, searchErr.message)
		return
	}

	c.JSON(http.StatusOK, resp)
}
