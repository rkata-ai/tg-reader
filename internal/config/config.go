package config

import (
	"fmt"

	"github.com/spf13/viper"
)

type Config struct {
	Telegram TelegramConfig `mapstructure:"telegram"`
}

type TelegramConfig struct {
	BotToken string   `mapstructure:"bot_token"`
	APIID    string   `mapstructure:"api_id"`
	APIHash  string   `mapstructure:"api_hash"`
	Channels []string `mapstructure:"channels"`
}

func Load(configPath string) (*Config, error) {
	// Устанавливаем значения по умолчанию
	viper.SetDefault("telegram.bot_token", "")
	viper.SetDefault("telegram.api_id", "")
	viper.SetDefault("telegram.api_hash", "")
	viper.SetDefault("telegram.channels", []string{})

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
}

func validateConfig(config *Config) error {
	// Проверяем Telegram конфигурацию
	if config.Telegram.APIID == "" {
		return fmt.Errorf("telegram API ID is required")
	}

	if config.Telegram.APIHash == "" {
		return fmt.Errorf("telegram API hash is required")
	}

	return nil
}
