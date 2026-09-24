package logger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/rs/zerolog"
	"gopkg.in/natefinch/lumberjack.v2"
)

type LogLevel string

type Logger interface {
	Debug() *Event
	Info() *Event
	Warn() *Event
	Error() *Event
	Fatal() *Event

	With() Context
}

type Context interface {
	Component(name string) Context
	Module(name string) Context
	RequestID(id string) Context
	Str(key, val string) Context
	Int(key string, val int) Context
	Interface(key string, val interface{}) Context
	Logger() Logger
}

type ctx struct {
	loggerContext zerolog.Context
}

const (
	Info  LogLevel = "info"
	Debug LogLevel = "debug"
	Warn  LogLevel = "warn"
	Error LogLevel = "error"
)

type AppLogger struct {
	logger zerolog.Logger
}

type LogConfig struct {
	LogPath     string
	FileName    string
	MaxSizeMB   int
	MaxBackups  int
	MaxAgeDays  int
	Compress    bool
	Level       string
	PrettyPrint bool
}

func DefaultLogConfig() LogConfig {
	logPath := filepath.Join("storages", "logs")

	return LogConfig{
		LogPath:     logPath,
		FileName:    "app",
		MaxSizeMB:   50,
		MaxBackups:  3,
		MaxAgeDays:  3,
		Compress:    true,
		Level:       string(Info),
		PrettyPrint: true,
	}
}

func NewLogger(cfg LogConfig) (*AppLogger, error) {

	if err := os.MkdirAll(cfg.LogPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	setupLog := &lumberjack.Logger{
		Filename:   filepath.Join(cfg.LogPath, cfg.FileName+".log"),
		MaxSize:    cfg.MaxSizeMB,
		MaxBackups: cfg.MaxBackups,
		MaxAge:     cfg.MaxAgeDays,
		Compress:   cfg.Compress,
	}

	var writer []io.Writer

	writer = append(writer, setupLog)
	if cfg.PrettyPrint {
		consoleWriter := zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: "15:04:05",
			FormatLevel: func(i interface{}) string {
				return fmt.Sprintf("| %-6s|", i)
			},
		}
		writer = append(writer, consoleWriter)
	} else {
		writer = append(writer, os.Stdout)
	}

	multi := io.MultiWriter(writer...)
	level := parseLevel(cfg.Level)

	zl := zerolog.New(multi).Level(level).With().Timestamp().Logger()

	return &AppLogger{
		logger: zl,
	}, nil
}

func (l *AppLogger) Debug() *Event { return &Event{ev: l.logger.Debug()} }
func (l *AppLogger) Info() *Event  { return &Event{ev: l.logger.Info()} }
func (l *AppLogger) Warn() *Event  { return &Event{ev: l.logger.Warn()} }
func (l *AppLogger) Error() *Event { return &Event{ev: l.logger.Error()} }
func (l *AppLogger) Fatal() *Event { return &Event{ev: l.logger.Fatal()} }

func (l *AppLogger) With() Context {
	return &ctx{loggerContext: l.logger.With()}
}

func (c *ctx) Component(name string) Context {
	c.loggerContext = c.loggerContext.Str("component", name)
	return c
}
func (c *ctx) Module(name string) Context {
	c.loggerContext = c.loggerContext.Str("module", name)
	return c
}
func (c *ctx) RequestID(id string) Context {
	c.loggerContext = c.loggerContext.Str("request_id", id)
	return c
}
func (c *ctx) Str(key, val string) Context {
	c.loggerContext = c.loggerContext.Str(key, val)
	return c
}
func (c *ctx) Int(key string, val int) Context {
	c.loggerContext = c.loggerContext.Int(key, val)
	return c
}
func (c *ctx) Interface(key string, val interface{}) Context {
	c.loggerContext = c.loggerContext.Interface(key, val)
	return c
}
func (c *ctx) Logger() Logger {
	return &AppLogger{logger: c.loggerContext.Logger()}
}

func parseLevel(s string) zerolog.Level {
	switch s {
	case "debug":
		return zerolog.DebugLevel
	case "info":
		return zerolog.InfoLevel
	case "warn":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	default:
		return zerolog.InfoLevel
	}
}
