package api

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/dheerajb/routebite/internal/history"
	"github.com/dheerajb/routebite/internal/middleware"
	"github.com/gin-gonic/gin"
)

// AgentSearch handles POST /v1/agent/search.
func (h *Handler) AgentSearch(c *gin.Context) {
	var req AgentSearchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, err.Error())
		return
	}

	resp, plan, searchErr := h.runAgentSearch(c.Request.Context(), req)
	if searchErr != nil {
		writeError(c, searchErr.status, searchErr.message)
		return
	}

	h.saveAgentSearch(c.Request.Context(), c, req, plan, resp)
	c.JSON(http.StatusOK, resp)
}

func (h *Handler) saveAgentSearch(ctx context.Context, c *gin.Context, req AgentSearchRequest, plan agentPlan, resp AgentSearchResponse) {
	if h.history == nil {
		return
	}

	saveCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	record := history.AgentSearch{
		RequestID:        middleware.GetRequestID(c),
		Query:            req.Query,
		StartLocation:    plan.Start,
		Destination:      plan.Destination,
		Preference:       plan.Preference,
		MaxDetourMinutes: plan.MaxDetourMinutes,
		ResultCount:      len(resp.Restaurants),
		Summary:          resp.Summary,
	}
	if err := h.history.SaveAgentSearch(saveCtx, record); err != nil {
		log.Printf("agent search history save failed request_id=%s: %v", record.RequestID, err)
	}
}
