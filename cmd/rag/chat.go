package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"go-rag/internal/configs"
	"go-rag/internal/core/embedding"
	"go-rag/internal/core/rag"
	"go-rag/internal/domain/models"
	"go-rag/internal/infrastructure/database"
	"go-rag/internal/infrastructure/logger"

	"github.com/google/uuid"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

type chatRunner interface {
	Run(context.Context, string, string) (rag.State, error)
}

func runChatCLI(cfg *configs.Config, args []string, input io.Reader, output io.Writer) error {
	flags := flag.NewFlagSet("chat", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	resumeID := flags.String("session-id", "", "resume an existing UUIDv7 chat session")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("chat options: %w (usage: chat [--session-id UUIDv7])", err)
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("usage: chat [--session-id UUIDv7]")
	}

	sessionID, err := chatSessionID(*resumeID)
	if err != nil {
		return err
	}

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
	db = db.Session(&gorm.Session{Logger: gormlogger.Default.LogMode(gormlogger.Warn)})
	if err := db.AutoMigrate(&models.RagTurn{}); err != nil {
		return fmt.Errorf("prepare chat history: %w", err)
	}
	if *resumeID != "" {
		var count int64
		if err := db.Model(&models.RagTurn{}).Where("conversation_id = ?", sessionID).Count(&count).Error; err != nil {
			return fmt.Errorf("check chat session: %w", err)
		}
		if count == 0 {
			return fmt.Errorf("chat session %s was not found", sessionID)
		}
	}

	embedder, err := embedding.NewClient(cfg.Embedding, nil)
	if err != nil {
		return err
	}
	model, err := rag.NewChatClient(cfg.LLM, nil)
	if err != nil {
		return err
	}
	model.WithRetryLogger(func(attempt int, wait time.Duration, err error) {
		appLog.Warn().Int("attempt", attempt).Dur("wait", wait).Err(err).Msg("LLM call retried")
	})
	rules, err := rag.LoadGuardrails(filepath.Join("docs", "guardrails"))
	if err != nil {
		return err
	}
	graph := rag.NewGraph(rag.NewStoreWithAllHistory(db, embedder, cfg.Embedding.Model, cfg.Merchant.ID), model).WithGuardrails(rules)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return chatLoop(ctx, input, output, sessionID, graph)
}

func chatSessionID(resumeID string) (string, error) {
	if resumeID != "" {
		parsed, err := uuid.Parse(resumeID)
		if err != nil || parsed.Version() != 7 || parsed.String() != resumeID {
			return "", fmt.Errorf("session-id must be a canonical UUIDv7")
		}
		return resumeID, nil
	}
	id, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("create session ID: %w", err)
	}
	return id.String(), nil
}

func chatLoop(ctx context.Context, input io.Reader, output io.Writer, sessionID string, runner chatRunner) error {
	if _, err := fmt.Fprintf(output, "session_id: %s\nKetik /exit untuk keluar.\n", sessionID); err != nil {
		return err
	}
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	for {
		if err := ctx.Err(); err != nil {
			return nil
		}
		if _, err := fmt.Fprint(output, "Anda> "); err != nil {
			return err
		}
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil && !errors.Is(err, context.Canceled) {
				return fmt.Errorf("read chat input: %w", err)
			}
			return nil
		}
		message := strings.TrimSpace(scanner.Text())
		if message == "" {
			continue
		}
		if message == "/exit" || message == "/quit" || message == "exit" || message == "quit" || message == "keluar" {
			return nil
		}
		state, err := runner.Run(ctx, sessionID, message)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			if _, writeErr := fmt.Fprintf(output, "Gagal: %v\n", err); writeErr != nil {
				return writeErr
			}
			continue
		}
		if _, err := fmt.Fprintf(output, "AI> %s\n", state.Answer); err != nil {
			return err
		}
	}
}
