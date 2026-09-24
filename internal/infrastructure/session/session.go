package session

import (
	"go-rag/internal/configs"
	"net/http"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
)

func NewMiddleware(cfg configs.SessionConfig, isProd bool) gin.HandlerFunc {
	store := cookie.NewStore([]byte(cfg.Secret))
	store.Options(sessions.Options{
		Path:     "/",
		MaxAge:   cfg.MaxAge,
		Secure:   cfg.Secure,
		HttpOnly: cfg.HTTPOnly,
		SameSite: http.SameSiteLaxMode,
	})

	return sessions.Sessions(cfg.Name, store)
}
