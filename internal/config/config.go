package config

import (
	"fmt"

	"github.com/spf13/viper"
)

type Config struct {
	Telegram TelegramConfig `mapstructure:"telegram"`
	Database DatabaseConfig `mapstructure:"database"`
}

type TelegramConfig struct {
	BotToken  string   `mapstructure:"bot_token"`
	APIID     string   `mapstructure:"api_id"`
	APIHash   string   `mapstructure:"api_hash"`
	Channels  []string `mapstructure:"channels"`
	StartDate string   `mapstructure:"start_date"`
}

type DatabaseConfig struct {
	Host             string `mapstructure:"host"`
	Port             int    `mapstructure:"port"`
	User             string `mapstructure:"user"`
	Password         string `mapstructure:"password"`
	DBName           string `mapstructure:"dbname"`
	SSLMode          string `mapstructure:"sslmode"`
	ConnectionString string `mapstructure:"connection_string"` // Опционально, для прямой строки подключения
}

func Load(configPath string) (*Config, error) {
	// Устанавливаем значения по умолчанию
	viper.SetDefault("telegram.bot_token", "")
	viper.SetDefault("telegram.api_id", "")
	viper.SetDefault("telegram.api_hash", "")
	viper.SetDefault("telegram.channels", []string{})
	viper.SetDefault("telegram.start_date", "2024-01-01") // Значение по умолчанию для даты начала

	viper.SetDefault("database.host", "localhost")
	viper.SetDefault("database.port", 5432)
	viper.SetDefault("database.user", "postgres")
	viper.SetDefault("database.password", "")
	viper.SetDefault("database.dbname", "tg_reader")
	viper.SetDefault("database.sslmode", "disable")

	// Читаем конфигурацию из указанного файла
	viper.SetConfigFile(configPath)

	// Читаем переменные окружения
	viper.AutomaticEnv()
	viper.SetEnvPrefix("TRADING")

	// Привязываем переменные окружения к конфигурации
	bindEnvs()

	// Пытаемся прочитать файл конфигурации
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
	}

	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}

	// Валидация обязательных полей
	if err := validateConfig(&config); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &config, nil
}

func LoadDefaults(config *Config) error {
	// Устанавливаем значения по умолчанию
	viper.SetDefault("telegram.bot_token", "")
	viper.SetDefault("telegram.api_id", "")
	viper.SetDefault("telegram.api_hash", "")
	viper.SetDefault("telegram.channels", []string{})
	viper.SetDefault("telegram.start_date", "2024-01-01") // Значение по умолчанию для даты начала

	viper.SetDefault("database.host", "localhost")
	viper.SetDefault("database.port", 5432)
	viper.SetDefault("database.user", "postgres")
	viper.SetDefault("database.password", "")
	viper.SetDefault("database.dbname", "tg_reader")
	viper.SetDefault("database.sslmode", "disable")

	// Читаем переменные окружения
	viper.AutomaticEnv()
	viper.SetEnvPrefix("TRADING")

	// Привязываем переменные окружения к конфигурации
	bindEnvs()

	// Создаем конфиг с значениями по умолчанию
	if err := viper.Unmarshal(config); err != nil {
		return fmt.Errorf("error unmarshaling default config: %w", err)
	}

	return nil
}

func LoadFromEnv(config *Config) error {
	// Читаем переменные окружения
	viper.AutomaticEnv()
	viper.SetEnvPrefix("TRADING")

	// Привязываем переменные окружения к конфигурации
	bindEnvs()

	// Обновляем конфиг из переменных окружения
	if err := viper.Unmarshal(config); err != nil {
		return fmt.Errorf("error unmarshaling from env: %w", err)
	}

	return nil
}

func bindEnvs() {
	// Telegram
	viper.BindEnv("telegram.bot_token", "TRADING_TELEGRAM_BOT_TOKEN")
	viper.BindEnv("telegram.api_id", "TRADING_TELEGRAM_API_ID")
	viper.BindEnv("telegram.api_hash", "TRADING_TELEGRAM_API_HASH")
	viper.BindEnv("telegram.start_date", "TRADING_TELEGRAM_START_DATE")

	// Database
	viper.BindEnv("database.host", "TRADING_DATABASE_HOST")
	viper.BindEnv("database.port", "TRADING_DATABASE_PORT")
	viper.BindEnv("database.user", "TRADING_DATABASE_USER")
	viper.BindEnv("database.password", "TRADING_DATABASE_PASSWORD")
	viper.BindEnv("database.dbname", "TRADING_DATABASE_DBNAME")
	viper.BindEnv("database.sslmode", "TRADING_DATABASE_SSLMODE")
}

func validateConfig(config *Config) error {
	// Проверяем Telegram конфигурацию
	if config.Telegram.APIID == "" {
		return fmt.Errorf("telegram API ID is required")
	}

	if config.Telegram.APIHash == "" {
		return fmt.Errorf("telegram API hash is required")
	}

	// Проверяем конфигурацию базы данных
	if config.Database.Host == "" {
		return fmt.Errorf("database host is required")
	}

	if config.Database.Port == 0 {
		return fmt.Errorf("database port is required")
	}

	if config.Database.User == "" {
		return fmt.Errorf("database user is required")
	}

	if config.Database.Password == "" {
		return fmt.Errorf("database password is required")
	}

	if config.Database.DBName == "" {
		return fmt.Errorf("database name is required")
	}

	return nil
}
