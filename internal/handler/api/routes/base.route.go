package api_routes

import (
	"go-rag/internal/domain/services"
	controllers "go-rag/internal/handler/api/controllers"
	"go-rag/internal/infrastructure/logger"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type Router struct {
	services    *services.ServiceContainer
	controllers *controllers.BaseController
	log         logger.Logger
}

func NewRouter(
	services *services.ServiceContainer,
	controllers *controllers.BaseController,
	log logger.Logger,
) *Router {
	return &Router{
		services:    services,
		controllers: controllers,
		log:         log,
	}
}

func notFoundHandler(c *gin.Context) {
	if strings.HasPrefix(c.Request.URL.Path, "/api") {
		c.JSON(http.StatusNotFound, gin.H{"message": "Route not found API"})
		c.Abort()
	}
}

func (r *Router) RegisterRoutes(router *gin.Engine) {
	api := router.Group("/api")
	{
		r.AppRoutes(api)
	}

	router.NoRoute(notFoundHandler)
}
