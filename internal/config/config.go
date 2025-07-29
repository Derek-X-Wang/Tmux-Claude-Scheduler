package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/viper"
)

// Config holds the application configuration
type Config struct {
	// Database configuration
	Database DatabaseConfig `mapstructure:"database" json:"database"`

	// TUI configuration
	TUI TUIConfig `mapstructure:"tui" json:"tui"`

	// Scheduler configuration
	Scheduler SchedulerConfig `mapstructure:"scheduler" json:"scheduler"`

	// Usage monitoring configuration
	Usage UsageConfig `mapstructure:"usage" json:"usage"`

	// Logging configuration
	Logging LoggingConfig `mapstructure:"logging" json:"logging"`

	// Tmux configuration
	Tmux TmuxConfig `mapstructure:"tmux" json:"tmux"`
}

// DatabaseConfig holds database configuration
type DatabaseConfig struct {
	Path         string        `mapstructure:"path" json:"path"`
	LogLevel     string        `mapstructure:"log_level" json:"log_level"`
	MaxIdleConns int           `mapstructure:"max_idle_conns" json:"max_idle_conns"`
	MaxOpenConns int           `mapstructure:"max_open_conns" json:"max_open_conns"`
	ConnMaxLife  time.Duration `mapstructure:"conn_max_life" json:"conn_max_life"`
}

// TUIConfig holds TUI configuration
type TUIConfig struct {
	RefreshRate   time.Duration `mapstructure:"refresh_rate" json:"refresh_rate"`
	Theme         string        `mapstructure:"theme" json:"theme"`
	ShowDebugInfo bool          `mapstructure:"show_debug_info" json:"show_debug_info"`
}

// SchedulerConfig holds scheduler configuration
type SchedulerConfig struct {
	SmartEnabled          bool          `mapstructure:"smart_enabled" json:"smart_enabled"`
	CronEnabled           bool          `mapstructure:"cron_enabled" json:"cron_enabled"`
	ProcessingInterval    time.Duration `mapstructure:"processing_interval" json:"processing_interval"`
	MaxConcurrentMessages int           `mapstructure:"max_concurrent_messages" json:"max_concurrent_messages"`
	RetryAttempts         int           `mapstructure:"retry_attempts" json:"retry_attempts"`
	RetryDelay            time.Duration `mapstructure:"retry_delay" json:"retry_delay"`
}

// UsageConfig holds usage monitoring configuration
type UsageConfig struct {
	MaxMessages        int           `mapstructure:"max_messages" json:"max_messages"`
	MaxTokens          int           `mapstructure:"max_tokens" json:"max_tokens"`
	WindowDuration     time.Duration `mapstructure:"window_duration" json:"window_duration"`
	MonitoringInterval time.Duration `mapstructure:"monitoring_interval" json:"monitoring_interval"`
	ClaudeResetHour    int           `mapstructure:"claude_reset_hour" json:"claude_reset_hour"` // Hour of day when Claude usage resets (0-23)
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level  string `mapstructure:"level" json:"level"`
	Format string `mapstructure:"format" json:"format"`
	File   string `mapstructure:"file" json:"file"`
}

// TmuxConfig holds tmux configuration
type TmuxConfig struct {
	DiscoveryInterval     time.Duration `mapstructure:"discovery_interval" json:"discovery_interval"`
	HealthCheckInterval   time.Duration `mapstructure:"health_check_interval" json:"health_check_interval"`
	MessageDelay          time.Duration `mapstructure:"message_delay" json:"message_delay"`
	ClaudeDetectionMethod string        `mapstructure:"claude_detection_method" json:"claude_detection_method"` // "process", "text", or "both"
	ClaudeProcessNames    []string      `mapstructure:"claude_process_names" json:"claude_process_names"`       // Process names to look for
}

// global configuration instance
var appConfig *Config

// Load loads the configuration from file and environment variables
func Load(configPath string) (*Config, error) {
	// Set up viper
	v := viper.New()

	// Set config name and paths
	v.SetConfigName("config")
	v.SetConfigType("yaml")

	// Add config paths
	if configPath != "" {
		v.AddConfigPath(filepath.Dir(configPath))
	}

	// Default config paths
	homeDir, _ := os.UserHomeDir()
	v.AddConfigPath(filepath.Join(homeDir, ".tcs"))
	v.AddConfigPath(".")

	// Environment variable configuration
	v.SetEnvPrefix("TCS")
	v.AutomaticEnv()

	// Set defaults
	setDefaults(v)

	// Read config file
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
		// Config file not found, use defaults
	}

	// Unmarshal config
	config := &Config{}
	if err := v.Unmarshal(config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Validate config
	if err := validateConfig(config); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	// Set global config
	appConfig = config

	return config, nil
}

// setDefaults sets default configuration values
func setDefaults(v *viper.Viper) {
	// Database defaults
	v.SetDefault("database.path", getDefaultDatabasePath())
	v.SetDefault("database.log_level", "warn")
	v.SetDefault("database.max_idle_conns", 10)
	v.SetDefault("database.max_open_conns", 100)
	v.SetDefault("database.conn_max_life", time.Hour)

	// TUI defaults
	v.SetDefault("tui.refresh_rate", time.Second)
	v.SetDefault("tui.theme", "default")
	v.SetDefault("tui.show_debug_info", false)

	// Scheduler defaults
	v.SetDefault("scheduler.smart_enabled", true)
	v.SetDefault("scheduler.cron_enabled", true)
	v.SetDefault("scheduler.processing_interval", 10*time.Second)
	v.SetDefault("scheduler.max_concurrent_messages", 3)
	v.SetDefault("scheduler.retry_attempts", 3)
	v.SetDefault("scheduler.retry_delay", 30*time.Second)

	// Usage defaults
	v.SetDefault("usage.max_messages", 1000)
	v.SetDefault("usage.max_tokens", 100000)
	v.SetDefault("usage.window_duration", 5*time.Hour)
	v.SetDefault("usage.monitoring_interval", 30*time.Second)
	v.SetDefault("usage.claude_reset_hour", 11) // 11 AM reset time

	// Logging defaults
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "text")
	v.SetDefault("logging.file", "")

	// Tmux defaults
	v.SetDefault("tmux.discovery_interval", 30*time.Second)
	v.SetDefault("tmux.health_check_interval", 60*time.Second)
	v.SetDefault("tmux.message_delay", 500*time.Millisecond)
	v.SetDefault("tmux.claude_detection_method", "both") // "process", "text", or "both"
	v.SetDefault("tmux.claude_process_names", []string{"claude-code", "claude_code", "claude"})
}

// validateConfig validates the configuration
func validateConfig(config *Config) error {
	// Validate database path
	if config.Database.Path == "" {
		return fmt.Errorf("database path cannot be empty")
	}

	// Validate TUI refresh rate
	if config.TUI.RefreshRate < 100*time.Millisecond {
		return fmt.Errorf("TUI refresh rate must be at least 100ms")
	}

	// Validate scheduler settings
	if config.Scheduler.MaxConcurrentMessages < 1 {
		return fmt.Errorf("max concurrent messages must be at least 1")
	}

	if config.Scheduler.ProcessingInterval < time.Second {
		return fmt.Errorf("processing interval must be at least 1 second")
	}

	// Validate usage limits
	if config.Usage.MaxMessages < 1 {
		return fmt.Errorf("max messages must be at least 1")
	}

	if config.Usage.WindowDuration < time.Hour {
		return fmt.Errorf("usage window duration must be at least 1 hour")
	}

	// Validate Claude reset hour
	if config.Usage.ClaudeResetHour < 0 || config.Usage.ClaudeResetHour > 23 {
		return fmt.Errorf("claude reset hour must be between 0 and 23, got: %d", config.Usage.ClaudeResetHour)
	}

	// Validate logging level
	validLevels := []string{"debug", "info", "warn", "error", "fatal"}
	levelValid := false
	for _, level := range validLevels {
		if config.Logging.Level == level {
			levelValid = true
			break
		}
	}
	if !levelValid {
		return fmt.Errorf("invalid logging level: %s", config.Logging.Level)
	}

	return nil
}

// getDefaultDatabasePath returns the default database path
func getDefaultDatabasePath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = os.TempDir()
	}

	configDir := filepath.Join(homeDir, ".tcs")
	return filepath.Join(configDir, "tcs.db")
}

// Get returns the global configuration instance
func Get() *Config {
	return appConfig
}

// GetDatabaseConfig returns the database configuration
func GetDatabaseConfig() DatabaseConfig {
	if appConfig == nil {
		return DatabaseConfig{
			Path:         getDefaultDatabasePath(),
			LogLevel:     "warn",
			MaxIdleConns: 10,
			MaxOpenConns: 100,
			ConnMaxLife:  time.Hour,
		}
	}
	return appConfig.Database
}

// GetTUIConfig returns the TUI configuration
func GetTUIConfig() TUIConfig {
	if appConfig == nil {
		return TUIConfig{
			RefreshRate:   time.Second,
			Theme:         "default",
			ShowDebugInfo: false,
		}
	}
	return appConfig.TUI
}

// GetSchedulerConfig returns the scheduler configuration
func GetSchedulerConfig() SchedulerConfig {
	if appConfig == nil {
		return SchedulerConfig{
			SmartEnabled:          true,
			CronEnabled:           true,
			ProcessingInterval:    10 * time.Second,
			MaxConcurrentMessages: 3,
			RetryAttempts:         3,
			RetryDelay:            30 * time.Second,
		}
	}
	return appConfig.Scheduler
}

// GetUsageConfig returns the usage monitoring configuration
func GetUsageConfig() UsageConfig {
	if appConfig == nil {
		return UsageConfig{
			MaxMessages:        1000,
			MaxTokens:          100000,
			WindowDuration:     5 * time.Hour,
			MonitoringInterval: 30 * time.Second,
			ClaudeResetHour:    11,
		}
	}
	return appConfig.Usage
}

// GetLoggingConfig returns the logging configuration
func GetLoggingConfig() LoggingConfig {
	if appConfig == nil {
		return LoggingConfig{
			Level:  "info",
			Format: "text",
			File:   "",
		}
	}
	return appConfig.Logging
}

// GetTmuxConfig returns the tmux configuration
func GetTmuxConfig() TmuxConfig {
	if appConfig == nil {
		return TmuxConfig{
			DiscoveryInterval:   30 * time.Second,
			HealthCheckInterval: 60 * time.Second,
			MessageDelay:        500 * time.Millisecond,
		}
	}
	return appConfig.Tmux
}

// SaveConfig saves the current configuration to file
func SaveConfig(configPath string) error {
	if appConfig == nil {
		return fmt.Errorf("no configuration loaded")
	}

	v := viper.New()
	v.SetConfigType("yaml")

	// Set all config values
	v.Set("database", appConfig.Database)
	v.Set("tui", appConfig.TUI)
	v.Set("scheduler", appConfig.Scheduler)
	v.Set("usage", appConfig.Usage)
	v.Set("logging", appConfig.Logging)
	v.Set("tmux", appConfig.Tmux)

	if configPath == "" {
		configPath = getDefaultConfigPath()
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Write config file
	if err := v.WriteConfigAs(configPath); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// getDefaultConfigPath returns the default config file path
func getDefaultConfigPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = os.TempDir()
	}

	configDir := filepath.Join(homeDir, ".tcs")
	return filepath.Join(configDir, "config.yaml")
}

// GenerateDefaultConfig generates a default configuration file
func GenerateDefaultConfig(configPath string) error {
	if configPath == "" {
		configPath = getDefaultConfigPath()
	}

	// Create default config
	config := &Config{
		Database: DatabaseConfig{
			Path:         getDefaultDatabasePath(),
			LogLevel:     "warn",
			MaxIdleConns: 10,
			MaxOpenConns: 100,
			ConnMaxLife:  time.Hour,
		},
		TUI: TUIConfig{
			RefreshRate:   time.Second,
			Theme:         "default",
			ShowDebugInfo: false,
		},
		Scheduler: SchedulerConfig{
			SmartEnabled:          true,
			CronEnabled:           true,
			ProcessingInterval:    10 * time.Second,
			MaxConcurrentMessages: 3,
			RetryAttempts:         3,
			RetryDelay:            30 * time.Second,
		},
		Usage: UsageConfig{
			MaxMessages:        1000,
			MaxTokens:          100000,
			WindowDuration:     5 * time.Hour,
			MonitoringInterval: 30 * time.Second,
			ClaudeResetHour:    11,
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "text",
			File:   "",
		},
		Tmux: TmuxConfig{
			DiscoveryInterval:   30 * time.Second,
			HealthCheckInterval: 60 * time.Second,
			MessageDelay:        500 * time.Millisecond,
		},
	}

	// Set global config
	appConfig = config

	// Save to file
	return SaveConfig(configPath)
}
