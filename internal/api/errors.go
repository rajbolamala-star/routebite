package api

import (
	"github.com/dheerajb/routebite/internal/middleware"
	"github.com/gin-gonic/gin"
)

type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

type ErrorBody struct {
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
}

func writeError(c *gin.Context, status int, message string) {
	c.JSON(status, ErrorResponse{
		Error: ErrorBody{
			Message:   message,
			RequestID: middleware.GetRequestID(c),
		},
	})
}
