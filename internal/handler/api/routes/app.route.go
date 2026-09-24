package api_routes

import "github.com/gin-gonic/gin"

func (r *Router) AppRoutes(rG *gin.RouterGroup) {
	app := rG.Group("app")
	{
		app.GET("/version", r.controllers.App.Version)
	}
}
