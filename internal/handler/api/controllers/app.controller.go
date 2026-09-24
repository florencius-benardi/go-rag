package api_controllers

import (
	"go-rag/internal/infrastructure/logger"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
)

type AppController struct {
	log logger.Logger
}

func NewAppController(log logger.Logger) *AppController {
	return &AppController{log: log.With().Component("APP_UPDATE").Logger()}
}

func (c *AppController) Version(ctx *gin.Context) {
	SuccessResponse(ctx, gin.H{
		"version": os.Getenv("APP_VERSION"),
	}, "OK", http.StatusOK)
}

func (c *AppController) logRouteHit(ctx *gin.Context, action string) {
	c.log.Info().
		Str("action", action).
		Str("method", ctx.Request.Method).
		Str("path", ctx.FullPath()).
		Str("client_ip", ctx.ClientIP()).
		Msg("update route hit")
}

func (c *AppController) logRouteError(action string, err error) {
	c.log.Error().
		Str("action", action).
		Err(err).
		Msg("update route failed")
}
