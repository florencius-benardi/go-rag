package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"go-rag/internal/configs"
	"go-rag/internal/core/embedding"
	"go-rag/internal/core/rag"
	"go-rag/internal/database/migrations"
	"go-rag/internal/database/seeders"
	"go-rag/internal/domain/services"
	"go-rag/internal/infrastructure/database"
	"go-rag/internal/infrastructure/logger"
	"go-rag/internal/infrastructure/session"
	"net/http"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

type App struct {
	cfg    *configs.Config
	log    logger.Logger
	db     *gorm.DB
	server *http.Server

	shutdownTasks []shutdownTask
}

type shutdownTask struct {
	name string
	fn   func(context.Context) error
}

func NewApp(cfg *configs.Config, isProd bool) (*App, error) {
	application := &App{
		cfg: cfg,
	}

	log, err := initializeLogger(isProd)
	if err != nil {
		return nil, fmt.Errorf("init logger: %w", err)
	}

	application.log = &log

	log.Info().
		Str("ENV", cfg.App.Env).
		Str("version", cfg.App.Version).
		Bool("pretty_log", cfg.Logger.Pretty).
		Msg("logger ready")

	db, err := database.Connect(cfg.Database, log, isProd)
	if err != nil {
		return nil, err
	}
	application.db = db

	application.addShutdown("database", func(context.Context) error {
		return database.Close(db)
	})

	if err := runAutoMigration(db, log.With().Logger()); err != nil {
		return nil, fmt.Errorf("database migration failed: %w", err)
	}
	runAutoSeed(db, log.With().Logger())

	svcContainer := services.NewBaseServiceContainer(db)
	engine := application.setupRouter(svcContainer)
	application.server = &http.Server{
		Addr:         cfg.Server.Address(),
		Handler:      engine,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	application.addShutdown("http server", application.server.Shutdown)

	return application, nil
}

func (app *App) addShutdown(name string, fn func(context.Context) error) {
	app.shutdownTasks = append(app.shutdownTasks, shutdownTask{name: name, fn: fn})
}

// ==========================
// HANDLE LOG SYSTEM
// ==========================

func initializeLogger(isProd bool) (logger.AppLogger, error) {
	logPath := filepath.Join("storages", "log")
	cfg := logger.LogConfig{
		LogPath:     logPath,
		FileName:    "iorder",
		MaxSizeMB:   50,
		MaxBackups:  7,
		MaxAgeDays:  7,
		Compress:    true,
		Level:       "info",
		PrettyPrint: !isProd,
	}

	log, err := logger.NewLogger(cfg)

	return *log, err
}

// ==========================
// HANDLE RENDERING TEMPLATE HTML
// ==========================

// func renderTemplate(cfg configs.DevConfig, log logger.AppLogger) (*renderer.Renderer, error) {
// 	renderer, err := renderer.New(
// 		iorder.TemplatesFS,
// 		cfg.LiveTemplates,
// 		"web/templates",
// 	)
// 	if err != nil {
// 		return nil, fmt.Errorf("init renderer: %w", err)
// 	}

// 	if cfg.LiveTemplates {
// 		log.Warn().Msg("template live-reload enabled (development mode)")
// 	}

// 	return renderer, nil
// }

// func (app *App) setupRouter(render *renderer.Renderer) *gin.Engine {
func (app *App) setupRouter(svcContainer *services.ServiceContainer) *gin.Engine {
	if app.cfg.App.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	} else {
		gin.SetMode(gin.DebugMode)
	}

	// server := gin.Default() // ← include gin.Logger() + gin.Recovery() built-in
	server := gin.New()

	// Register custom validator tags (custom_date, custom_time) on gin's binding
	// engine. Must run before any route binds a struct that uses these tags,
	// otherwise validator/v10 panics with "Undefined validation function".
	// validations.RegisterCustomValidation()

	// server.Use(configs.ConfigCORS())
	// server.Use(middlewares.RequestID())
	// server.Use(middlewares.Recovery(app.log))
	server.Use(session.NewMiddleware(app.cfg.Session, app.cfg.App.IsProduction()))
	// server.Use(middlewares.HTMX())

	// staticSub, err := fs.Sub(iorder.StaticFS, "web/static")
	// if err != nil {
	// 	panic(fmt.Errorf("mount static: %w", err))
	// }
	// server.StaticFS("/static", http.FS(staticSub))

	server.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"version": app.cfg.App.Version,
		})
	})

	server.GET("/api/version", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  true,
			"message": "ok",
			"data": gin.H{
				"version": app.cfg.App.Version,
			},
		})
	})

	server.POST("/api/rag/chat", func(c *gin.Context) {
		var input struct {
			ConversationID string `json:"conversation_id"`
			Message        string `json:"message" binding:"required"`
		}
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "message is required"})
			return
		}
		embedder, err := embedding.NewClient(app.cfg.Embedding, nil)
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "embedding provider is not configured"})
			return
		}
		model, err := rag.NewChatClient(app.cfg.LLM, nil)
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "LLM provider is not configured"})
			return
		}
		graph := rag.NewGraph(rag.NewStore(app.db, embedder, app.cfg.Embedding.Model), model)
		state, err := graph.Run(c.Request.Context(), input.ConversationID, input.Message)
		if err != nil {
			app.log.Error().Err(err).Str("request_id", state.RequestID).Msg("RAG workflow failed")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "RAG workflow failed", "request_id": state.RequestID})
			return
		}
		c.JSON(http.StatusOK, state)
	})

	// apiController := api_controllers.NewController(
	// 	svcContainer,
	// 	// render,
	// 	app.log,
	// )

	// apiRouter := api_routes.NewRouter(svcContainer, apiController, app.log)
	// apiRouter.RegisterRoutes(server)

	// if app.cfg.App.IsProduction() {
	// 	embedReactRoutes(server)
	// } else {
	// 	serveReactProxy(server)
	// }

	return server
}

// func (app *App) setupScheduler(svcContainer *services.ServiceContainer) error {
// 	sc, err := scheduler.New(svcContainer, app.log)
// 	if err != nil {
// 		return fmt.Errorf("init scheduler: %w", err)
// 	}
// 	sc.Start()

// 	app.addShutdown("scheduler", sc.Shutdown)

// 	return nil
// }

func (app *App) Run() error {
	ctx, stop := signal.NotifyContext(context.Background(),
		syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT,
	)
	defer stop()

	serverErr := make(chan error, 1)
	go func() {
		log.Info().Str("addr", app.cfg.Server.Address()).Msg("http server listening")
		if err := app.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
		close(serverErr)
	}()

	select {
	case err := <-serverErr:
		if err != nil {
			app.log.Error().Err(err).Msg("server crashed")
			return err
		}
	case <-ctx.Done():
		app.log.Info().Msg("shutdown signal received")
	}

	return app.Shutdown()
}

// Serve starts the HTTP server and blocks until it stops (via Shutdown) or
// crashes. Unlike Run it installs no OS signal handlers, so the lifecycle can
// be driven externally — e.g. by the Windows Service Control Manager, which
// delivers stop through a control channel rather than a signal.
func (app *App) Serve() error {
	app.log.Info().Str("addr", app.cfg.Server.Address()).Msg("http server listening")
	if err := app.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		app.log.Error().Err(err).Msg("server crashed")
		return err
	}
	return nil
}

func (app *App) Shutdown() error {
	shutdownCtx, cancel := context.WithTimeout(
		context.Background(),
		app.cfg.Server.ShutdownTimeout,
	)
	defer cancel()

	app.log.Info().
		Int("tasks", len(app.shutdownTasks)).
		Dur("timeout", app.cfg.Server.ShutdownTimeout).
		Msg("graceful shutdown started")

	for i := len(app.shutdownTasks) - 1; i >= 0; i-- {
		task := app.shutdownTasks[i]
		if err := task.fn(shutdownCtx); err != nil {
			app.log.Error().Err(err).Str("task", task.name).Msg("shutdown task failed")
			continue
		}
		app.log.Info().Str("task", task.name).Msg("shutdown task ok")
	}

	app.log.Info().Msg("shutdown complete")
	return nil
}

func (app *App) DB() *gorm.DB { return app.db }

func isAPIPath(p string) bool {
	return p == "/api" || strings.HasPrefix(p, "/api/")
}

func notFoundJSON(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusNotFound, gin.H{
		"status":  false,
		"message": "route not found",
		"path":    c.Request.URL.Path,
	})
}

// ==========================
// HANDLE MIGRATE & SEED DATA
// ==========================

func runAutoMigration(db *gorm.DB, log logger.Logger) error {
	log.Info().Msg("Checking database migrations...")

	migrator := migrations.NewMigrator(db)
	status := migrator.GetStatus()

	log.Info().
		Int("total", status["total"].(int)).
		Int("applied", status["applied"].(int)).
		Int("pending", status["pending"].(int)).
		Msg("Migration status")

	if !status["needs_migrate"].(bool) {
		log.Info().Msg("Database is up to date")
		return nil
	}

	log.Info().Msg("Running pending migrations...")
	ran, err := migrator.RunPending()
	if err != nil {
		return err
	}

	log.Info().
		Str("migrations", strings.Join(ran, " ")).
		Msg("Migrations completed")

	return nil
}

func runAutoSeed(db *gorm.DB, log logger.Logger) {
	seeder := seeders.NewSeeder(db)
	seeder.RunAll()

	log.Info().
		Msg("Seeders completed")
}
