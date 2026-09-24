package database

import (
	"fmt"
	"go-rag/internal/configs"
	"go-rag/internal/domain/models"
	"go-rag/internal/infrastructure/logger"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// Connect opens the database, retrying with exponential backoff if the first
// attempt fails. This covers the case where the database is still starting up
// alongside this app (e.g. both installed as Windows services) — without it, a
// slow-starting MySQL fails the whole app on the very first try, and only the
// SCM's restart-on-failure (a full process kill and relaunch, once a minute)
// brings it back up.
func Connect(cfg configs.DatabaseConfig, log logger.AppLogger, isProd bool) (*gorm.DB, error) {
	db, err := connectWithRetry(cfg, isProd, log)
	if err != nil {
		return nil, err
	}

	db.AutoMigrate(
		&models.AppVersions{},
		&models.Migrations{},
	)

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}

	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(0)

	return db, nil
}

// connectWithRetry calls open() up to cfg.ConnectRetries times, doubling the
// delay between attempts up to ConnectRetryMaxDelay. Any failure is retried —
// a bad password looks identical to "server not ready yet" from here, and
// retrying once more before giving up is cheap either way.
func connectWithRetry(cfg configs.DatabaseConfig, isProd bool, log logger.AppLogger) (*gorm.DB, error) {
	attempts := cfg.ConnectRetries
	if attempts < 1 {
		attempts = 1
	}
	delay := cfg.ConnectRetryDelay

	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		db, err := open(cfg, isProd)
		if err == nil {
			if attempt > 1 {
				log.Info().Int("attempt", attempt).Msg("database connected")
			}
			return db, nil
		}
		lastErr = err

		if attempt == attempts {
			break
		}

		log.Warn().
			Err(err).
			Int("attempt", attempt).
			Int("max_attempts", attempts).
			Dur("retry_in", delay).
			Msg("database connect failed, retrying")

		time.Sleep(delay)

		delay *= 2
		if delay > cfg.ConnectRetryMaxDelay {
			delay = cfg.ConnectRetryMaxDelay
		}
	}

	return nil, fmt.Errorf("connect database after %d attempt(s): %w", attempts, lastErr)
}

// open makes a single connection attempt. No retry here — that is
// connectWithRetry's job — so every failure path returns immediately.
func open(cfg configs.DatabaseConfig, isProd bool) (*gorm.DB, error) {
	gormCfg := &gorm.Config{
		PrepareStmt:                              true,
		DisableForeignKeyConstraintWhenMigrating: true,
	}

	if isProd {
		gormCfg.Logger = gormlogger.Default.LogMode(gormlogger.Warn)
	} else {
		gormCfg.Logger = gormlogger.Default.LogMode(gormlogger.Info)
	}

	dsn := cfg.DSN()
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN: dsn,
	}), gormCfg)

	if err != nil {
		// The DSN carries the password, so report the target only.
		return nil, fmt.Errorf("connect postgres %s:%s/%s: %w", cfg.Host, cfg.Port, cfg.Name, err)
	}
	return db, nil
}

func Close(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
