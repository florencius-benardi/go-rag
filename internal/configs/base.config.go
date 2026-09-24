package configs

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Config struct {
	App       AppConfig
	Server    ServerConfig
	Database  DatabaseConfig
	Logger    LoggerConfig
	Session   SessionConfig
	Embedding EmbeddingRAGConfig
	LLM       LLMConfig
	Dev       DevConfig
}

type AppConfig struct {
	Name    string
	Env     string // development | production
	Version string
	Debug   bool
}

type ServerConfig struct {
	Host            string
	Port            string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration
}

type LoggerConfig struct {
	Level      string
	Path       string
	File       string
	MaxSizeMB  int
	MaxBackups int
	MaxAgeDays int
	Compress   bool
	Pretty     bool
}

type SessionConfig struct {
	Name     string
	Secret   string
	MaxAge   int
	Secure   bool
	HTTPOnly bool
	Driver   string
	Path     string
}

type EmbeddingRAGConfig struct {
	Provider     string
	BaseURL      string
	APIKey       string
	Model        string
	ChunkSize    int
	ChunkOverlap int
	SyncDebug    bool
}

type LLMConfig struct {
	BaseURL string
	APIKey  string
	Model   string
}

type DatabaseConfig struct {
	Connection      string
	Host            string
	Port            string
	User            string
	Password        string
	Name            string
	SSLMode         string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration

	// Startup connect retry. The database may still be starting when this app
	// launches (e.g. both run as Windows services and boot together), so the
	// first connect attempt is allowed to fail and retry with backoff instead
	// of failing the whole app straight away.
	ConnectRetries       int
	ConnectRetryDelay    time.Duration
	ConnectRetryMaxDelay time.Duration
}

type DevConfig struct {
	LiveTemplates bool
}

// AppSettings holds runtime settings loaded from the database at startup.
// All fields are protected by a RWMutex so services can safely read/write
// concurrently without hitting the DB on every request.
type AppSettings struct {
	mu            sync.RWMutex
	endpointURL   string
	endpointToken string
}

func (s *AppSettings) GetEndpointURL() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.endpointURL
}

func (s *AppSettings) SetEndpointURL(url string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.endpointURL = url
}

func (s *AppSettings) GetEndpointToken() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.endpointToken
}

func (s *AppSettings) SetEndpointToken(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.endpointToken = token
}

func (a AppConfig) IsProduction() bool {
	return a.Env == "production"
}

func (s ServerConfig) Address() string {
	return fmt.Sprintf("%s:%s", s.Host, s.Port)
}

func (d DatabaseConfig) DSN() string {
	sslMode := d.SSLMode
	if sslMode == "" {
		sslMode = "disable"
	}

	return "host=" + d.Host +
		" port=" + d.Port +
		" user=" + d.User +
		" password=" + d.Password +
		" dbname=" + d.Name +
		" sslmode=" + sslMode
}

func Load() (*Config, error) {
	isProd := os.Getenv("GIN_MODE") == "release"

	cfg := &Config{
		App: AppConfig{
			Name:    os.Getenv("APP_NAME"),
			Version: os.Getenv("APP_VERSION"),
			Env:     os.Getenv("GIN_MODE"),
			Debug:   true,
			// /!isProd,
		},
		Server: ServerConfig{
			Host: os.Getenv("GIN_URL"),
			Port: os.Getenv("GIN_PORT"),
			// Zero values here mean "no limit" for the http.Server and, worse,
			// an already-expired context for the graceful shutdown — so every
			// timeout gets an explicit default.
			ReadTimeout:  GetEnvAsDuration("SERVER_READ_TIMEOUT", 30*time.Second),
			WriteTimeout: GetEnvAsDuration("SERVER_WRITE_TIMEOUT", 60*time.Second),
			IdleTimeout:  GetEnvAsDuration("SERVER_IDLE_TIMEOUT", 120*time.Second),
			// Must stay under the SCM's stop timeout so a Windows service stop
			// never gets killed mid-drain.
			ShutdownTimeout: GetEnvAsDuration("SERVER_SHUTDOWN_TIMEOUT", 30*time.Second),
		},
		Database: DatabaseConfig{
			Connection:   os.Getenv("DB_CONNECTION"),
			Host:         os.Getenv("DB_HOST"),
			Port:         os.Getenv("DB_PORT"),
			User:         os.Getenv("DB_USERNAME"),
			Password:     os.Getenv("DB_PASSWORD"),
			Name:         os.Getenv("DB_DATABASE"),
			SSLMode:      os.Getenv("DB_SSLMODE"),
			MaxOpenConns: GetEnvAsInt("DB_MAX_OPEN_CONNS", 30),
			MaxIdleConns: GetEnvAsInt("DB_MAX_IDLE_CONNS", 30),
			// ConnMaxLifetime: GetEnvAsInt("DB_CONN_MAX_LIFETIME", 0),
			// Defaults total ~1 minute worst case (3s, 6s, 12s, 15s, 15s between
			// 6 attempts) — enough for a dependent DB service to finish starting,
			// while staying well inside the 3-minute SCM startup budget.
			ConnectRetries:       GetEnvAsInt("DB_CONNECT_RETRIES", 6),
			ConnectRetryDelay:    GetEnvAsDuration("DB_CONNECT_RETRY_DELAY", 3*time.Second),
			ConnectRetryMaxDelay: GetEnvAsDuration("DB_CONNECT_RETRY_MAX_DELAY", 15*time.Second),
		},
		Logger: LoggerConfig{
			Level:      os.Getenv("LOG_LEVEL"),
			Path:       os.Getenv("LOG_PATH"),
			File:       os.Getenv("LOG_FILE"),
			MaxSizeMB:  GetEnvAsInt("LOG_MAX_SIZE", 20),
			MaxBackups: GetEnvAsInt("LOG_MAX_BACKUPS", 7),
			MaxAgeDays: GetEnvAsInt("LOG_MAX_AGE", 7),
			Compress:   true,
			Pretty:     !isProd,
		},
		Embedding: EmbeddingRAGConfig{
			Provider:     strings.ToLower(strings.TrimSpace(os.Getenv("EMBEDDING_PROVIDER"))),
			BaseURL:      strings.TrimRight(os.Getenv("EMBEDDING_BASE_URL"), "/"),
			APIKey:       os.Getenv("EMBEDDING_API_KEY"),
			Model:        os.Getenv("EMBEDDING_MODEL"),
			ChunkSize:    GetEnvAsInt("EMBEDDING_CHUNK_SIZE", 500),
			ChunkOverlap: GetEnvAsInt("EMBEDDING_CHUNK_OVERLAP", 75),
			SyncDebug:    GetEnvAsBool("EMBEDDING_SYNC_DEBUG", false),
		},
		LLM: LLMConfig{
			BaseURL: strings.TrimRight(os.Getenv("LLM_BASE_URL"), "/"),
			APIKey:  os.Getenv("LLM_API_KEY"),
			Model:   os.Getenv("LLM_MODEL"),
		},
		// Session: SessionConfig{
		// 	Name:     os.Getenv("SESSION_NAME"),
		// 	Secret:   os.Getenv("SESSION_SECRET"),
		// 	MaxAge:   GetEnvAsInt("SESSION_LIFETIME", 120),
		// 	Secure:   GetEnvAsBool("SESSION_ENCRYPT", false),
		// 	HTTPOnly: GetEnvAsBool("SESSION_HTTP_ONLY", true),
		// 	Path:     os.Getenv("SESSION_PATH"),
		// 	Driver:   os.Getenv("SESSION_DRIVER"),
		// },
		Dev: DevConfig{
			LiveTemplates: GetEnvAsBool("DEV_LIVE_TEMPLATES", false),
		},
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) validate() error {
	if c.Session.Name == "" {
		c.Session.Name = "rag_session"
	}
	if c.Database.Name == "" {
		return fmt.Errorf("DB_NAME is required")
	}
	return c.validateEmbedding()
}

func (c *Config) validateEmbedding() error {
	if c.Embedding.Provider == "" {
		c.Embedding.Provider = "nvidia"
	}
	if c.Embedding.Provider != "nvidia" && c.Embedding.Provider != "openai" {
		return fmt.Errorf("EMBEDDING_PROVIDER must be nvidia or openai")
	}
	if c.Embedding.BaseURL == "" {
		return fmt.Errorf("EMBEDDING_BASE_URL is required")
	}
	if c.Embedding.APIKey == "" {
		return fmt.Errorf("EMBEDDING_API_KEY is required")
	}
	if c.Embedding.Model == "" {
		return fmt.Errorf("EMBEDDING_MODEL is required")
	}
	if c.Embedding.ChunkSize < 1 {
		return fmt.Errorf("EMBEDDING_CHUNK_SIZE must be greater than 0")
	}
	if c.Embedding.ChunkOverlap < 0 || c.Embedding.ChunkOverlap >= c.Embedding.ChunkSize {
		return fmt.Errorf("EMBEDDING_CHUNK_OVERLAP must be at least 0 and less than EMBEDDING_CHUNK_SIZE")
	}
	return nil
}

func GetEnvAsInt(name string, defValue int) int {
	if valueStr := os.Getenv(name); valueStr != "" {
		if value, err := strconv.Atoi(valueStr); err == nil {
			return value
		}
	}

	return defValue
}

// func GetEnvAsTime(name string, defValue int) time.Time {
// 	if valueStr := os.Getenv(name); valueStr != "" {
// 		if value, err := strconv.Atoi(valueStr); err == nil {
// 			return value
// 		}
// 	}

// 	return defValue
// }

// GetEnvAsDuration reads a duration from the environment. A bare number is
// read as seconds (SERVER_READ_TIMEOUT=45); a suffixed value is parsed by
// time.ParseDuration (SERVER_READ_TIMEOUT=1m30s). Anything unparseable falls
// back to defValue instead of silently becoming zero.
func GetEnvAsDuration(name string, defValue time.Duration) time.Duration {
	valueStr := os.Getenv(name)
	if valueStr == "" {
		return defValue
	}

	if seconds, err := strconv.Atoi(valueStr); err == nil {
		return time.Duration(seconds) * time.Second
	}
	if value, err := time.ParseDuration(valueStr); err == nil {
		return value
	}

	return defValue
}

func GetEnvAsBool(name string, defaultV bool) bool {
	if valueStr := os.Getenv(name); valueStr != "" {
		return strings.ToLower(valueStr) == "true"
	}

	return defaultV
}
