package main

import (
	"context"
	"fmt"
	"os/signal"
	"path/filepath"
	"syscall"

	"go-rag/internal/configs"
	"go-rag/internal/core/embedding"
	"go-rag/internal/infrastructure/database"
	"go-rag/internal/infrastructure/logger"
)

func runEmbedCatalogs(cfg *configs.Config) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	logPath := cfg.Logger.Path
	if logPath == "" {
		logPath = filepath.Join("storages", "log")
	}
	logCfg := logger.DefaultLogConfig()
	logCfg.LogPath = logPath
	logCfg.Level = cfg.Logger.Level
	logCfg.PrettyPrint = cfg.Logger.Pretty
	appLog, err := logger.NewLogger(logCfg)
	if err != nil {
		return fmt.Errorf("create logger: %w", err)
	}
	db, err := database.Connect(cfg.Database, *appLog, cfg.App.IsProduction())
	if err != nil {
		return err
	}
	defer database.Close(db)

	client, err := embedding.NewClient(cfg.Embedding, nil)
	if err != nil {
		return err
	}
	engine, err := embedding.NewEngine(db, client, cfg.Embedding)
	if err != nil {
		return err
	}
	result, err := engine.SyncCatalogs(ctx)
	if err != nil {
		return err
	}
	appLog.Info().Int("products", result.Products).Int("chunks", result.Chunks).
		Int("embedded", result.Embedded).Int("reused", result.Reused).
		Int("metadata_updated", result.MetadataUpdated).Int64("removed", result.Removed).
		Msg("catalog embedding sync complete")
	return nil
}
