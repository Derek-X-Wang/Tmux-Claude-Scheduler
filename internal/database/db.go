package database

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

// Config holds database configuration
type Config struct {
	DatabasePath string
	LogLevel     string
	MaxIdleConns int
	MaxOpenConns int
	ConnMaxLife  time.Duration
}

// DefaultConfig returns the default database configuration
func DefaultConfig() *Config {
	homeDir, _ := os.UserHomeDir()
	configDir := filepath.Join(homeDir, ".config", "tcs")

	return &Config{
		DatabasePath: filepath.Join(configDir, "tcs.db"),
		LogLevel:     "warn",
		MaxIdleConns: 10,
		MaxOpenConns: 100,
		ConnMaxLife:  time.Hour,
	}
}

// Initialize initializes the database connection
func Initialize(config *Config) error {
	if config == nil {
		config = DefaultConfig()
	}

	// Ensure directory exists
	dbDir := filepath.Dir(config.DatabasePath)
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return fmt.Errorf("failed to create database directory: %w", err)
	}

	// Configure GORM logger
	logLevel := logger.Warn
	switch config.LogLevel {
	case "silent":
		logLevel = logger.Silent
	case "error":
		logLevel = logger.Error
	case "warn":
		logLevel = logger.Warn
	case "info":
		logLevel = logger.Info
	}

	gormConfig := &gorm.Config{
		Logger: logger.Default.LogMode(logLevel),
		// Disable default transaction for better performance
		SkipDefaultTransaction: true,
	}

	// Open database connection using pure-Go SQLite driver
	var err error
	DB, err = gorm.Open(sqlite.Open(config.DatabasePath), gormConfig)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	// Get underlying SQL database for connection pool configuration
	sqlDB, err := DB.DB()
	if err != nil {
		return fmt.Errorf("failed to get underlying database: %w", err)
	}

	// Configure connection pool
	sqlDB.SetMaxIdleConns(config.MaxIdleConns)
	sqlDB.SetMaxOpenConns(config.MaxOpenConns)
	sqlDB.SetConnMaxLifetime(config.ConnMaxLife)

	// Configure SQLite for better performance
	if err := configureSQLite(DB); err != nil {
		return fmt.Errorf("failed to configure SQLite: %w", err)
	}

	// Run auto-migration
	if err := runMigrations(DB); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	// Initialize default data
	if err := initializeDefaultData(DB); err != nil {
		return fmt.Errorf("failed to initialize default data: %w", err)
	}

	log.Printf("Database initialized successfully at: %s", config.DatabasePath)
	return nil
}

// configureSQLite optimizes SQLite settings for performance
func configureSQLite(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}

	// SQLite optimization settings
	pragmas := []string{
		"PRAGMA synchronous = NORMAL", // Balance between safety and performance
		"PRAGMA cache_size = 10000",   // Increase cache size
		"PRAGMA temp_store = memory",  // Store temp data in memory
		"PRAGMA journal_mode = WAL",   // Write-Ahead Logging for better concurrency
		"PRAGMA foreign_keys = ON",    // Enable foreign key constraints
		"PRAGMA busy_timeout = 5000",  // Wait 5 seconds for locks
	}

	for _, pragma := range pragmas {
		if _, err := sqlDB.Exec(pragma); err != nil {
			return fmt.Errorf("failed to execute pragma %s: %w", pragma, err)
		}
	}

	return nil
}

// runMigrations runs all database migrations
func runMigrations(db *gorm.DB) error {
	// Auto-migrate all models (window-based architecture)
	err := db.AutoMigrate(
		&Message{},            // window-based messages
		&UsageWindow{},        // 5-hour usage tracking
		&AppConfig{},          // application configuration
		&TmuxSession{},        // tmux session discovery
		&SchedulerState{},     // scheduler state management
		&TmuxWindow{},         // window-based architecture core
		&WindowMessageQueue{}, // per-window message queues
	)
	if err != nil {
		return fmt.Errorf("auto-migration failed: %w", err)
	}

	// Create custom indexes for better performance
	if err := createCustomIndexes(db); err != nil {
		return fmt.Errorf("failed to create custom indexes: %w", err)
	}

	return nil
}

// createCustomIndexes creates additional indexes for performance
func createCustomIndexes(db *gorm.DB) error {
	indexes := []string{
		"CREATE INDEX IF NOT EXISTS idx_messages_status_scheduled ON messages(status, scheduled_time)",
		"CREATE INDEX IF NOT EXISTS idx_messages_priority_time ON messages(priority DESC, scheduled_time ASC)",
		"CREATE INDEX IF NOT EXISTS idx_sessions_active_name ON sessions(active, name)",
		"CREATE INDEX IF NOT EXISTS idx_usage_windows_active_time ON usage_windows(active, start_time, end_time)",
		"CREATE INDEX IF NOT EXISTS idx_session_stats_date ON session_stats(date DESC)",
		"CREATE INDEX IF NOT EXISTS idx_tmux_sessions_active ON tmux_sessions(active, has_claude)",
	}

	for _, indexSQL := range indexes {
		if err := db.Exec(indexSQL).Error; err != nil {
			return fmt.Errorf("failed to create index: %s, error: %w", indexSQL, err)
		}
	}

	return nil
}

// initializeDefaultData sets up initial data
func initializeDefaultData(db *gorm.DB) error {
	// Initialize scheduler states
	schedulers := []SchedulerState{
		{
			Name:    SchedulerSmart,
			Enabled: true,
			Status:  SchedulerStatusIdle,
		},
		{
			Name:    SchedulerCron,
			Enabled: true,
			Status:  SchedulerStatusIdle,
		},
	}

	for _, scheduler := range schedulers {
		var existing SchedulerState
		err := db.Where("name = ?", scheduler.Name).First(&existing).Error
		if err == gorm.ErrRecordNotFound {
			if err := db.Create(&scheduler).Error; err != nil {
				return fmt.Errorf("failed to create scheduler state %s: %w", scheduler.Name, err)
			}
		}
	}

	// Initialize default configuration
	defaultConfigs := map[string]string{
		"refresh_rate":     "1000",
		"max_sessions":     "10",
		"default_priority": "5",
		"log_level":        "info",
	}

	for key, value := range defaultConfigs {
		var existing AppConfig
		err := db.Where("key = ?", key).First(&existing).Error
		if err == gorm.ErrRecordNotFound {
			config := AppConfig{
				Key:   key,
				Value: value,
			}
			if err := db.Create(&config).Error; err != nil {
				return fmt.Errorf("failed to create config %s: %w", key, err)
			}
		}
	}

	return nil
}

// Close closes the database connection
func Close() error {
	if DB == nil {
		return nil
	}

	sqlDB, err := DB.DB()
	if err != nil {
		return err
	}

	return sqlDB.Close()
}

// GetDB returns the global database instance
func GetDB() *gorm.DB {
	return DB
}

// Health checks database connectivity
func Health() error {
	if DB == nil {
		return fmt.Errorf("database not initialized")
	}

	sqlDB, err := DB.DB()
	if err != nil {
		return fmt.Errorf("failed to get underlying database: %w", err)
	}

	if err := sqlDB.Ping(); err != nil {
		return fmt.Errorf("database ping failed: %w", err)
	}

	return nil
}

// Stats returns database statistics
type Stats struct {
	Windows         int64 `json:"windows"` // changed from sessions
	Messages        int64 `json:"messages"`
	UsageWindows    int64 `json:"usage_windows"`
	TmuxSessions    int64 `json:"tmux_sessions"`  // actual tmux sessions
	ActiveWindows   int64 `json:"active_windows"` // changed from active_sessions
	PendingMessages int64 `json:"pending_messages"`
}

// GetStats returns database statistics
func GetStats() (*Stats, error) {
	if DB == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	stats := &Stats{}

	// Count all records
	DB.Model(&TmuxWindow{}).Count(&stats.Windows) // changed from Session
	DB.Model(&Message{}).Count(&stats.Messages)
	DB.Model(&UsageWindow{}).Count(&stats.UsageWindows)
	DB.Model(&TmuxSession{}).Count(&stats.TmuxSessions)

	// Count active/pending records
	DB.Model(&TmuxWindow{}).Where("active = ?", true).Count(&stats.ActiveWindows) // changed from Session
	DB.Model(&Message{}).Where("status = ?", MessageStatusPending).Count(&stats.PendingMessages)

	return stats, nil
}

// Cleanup performs database maintenance
func Cleanup() error {
	if DB == nil {
		return fmt.Errorf("database not initialized")
	}

	// Clean up old data (older than 7 days)
	if err := CleanupOldData(DB, 7*24*time.Hour); err != nil {
		return fmt.Errorf("failed to cleanup old data: %w", err)
	}

	// VACUUM database to reclaim space (SQLite specific)
	if err := DB.Exec("VACUUM").Error; err != nil {
		log.Printf("Warning: VACUUM failed: %v", err)
		// Don't return error, VACUUM is optimization
	}

	// Analyze tables for query optimization
	if err := DB.Exec("ANALYZE").Error; err != nil {
		log.Printf("Warning: ANALYZE failed: %v", err)
		// Don't return error, ANALYZE is optimization
	}

	return nil
}

// Transaction helper function
func Transaction(fn func(*gorm.DB) error) error {
	if DB == nil {
		return fmt.Errorf("database not initialized")
	}

	return DB.Transaction(fn)
}

// IsInitialized checks if database is initialized
func IsInitialized() bool {
	return DB != nil
}
