package config

import (
	"os"
	"strconv"
)

type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	JWT      JWTConfig
	AI       AIConfig
}

type ServerConfig struct {
	Port string
	Env  string
}

type DatabaseConfig struct {
	Host        string
	Port        string
	Name        string
	User        string
	Password    string
	SSLMode     string
	SSLRootCert string
}

type JWTConfig struct {
	Secret string
}

type AIConfig struct {
	Provider        string
	APIKey          string
	Model           string
	DashScopeAPIKey string // DashScope (Qwen) API Key，用于获取模型列表等
	XAIAPIKey       string // xAI Grok 官方 API Key
}

func Load() *Config {
	return &Config{
		Server: ServerConfig{
			Port: getEnv("SERVER_PORT", "8080"),
			Env:  getEnv("ENV", "development"),
		},
		Database: LoadDatabase(),
		JWT: JWTConfig{
			Secret: GetJWTSecret(),
		},
		AI: AIConfig{
			Provider:        getEnv("AI_PROVIDER", "openai"),
			APIKey:          getEnv("AI_API_KEY", ""),
			Model:           getEnv("AI_MODEL", "gpt-4"),
			DashScopeAPIKey: getEnv("DASHSCOPE_API_KEY", ""),
			XAIAPIKey:       getEnv("XAI_API_KEY", ""),
		},
	}
}

// LoadDatabase returns only the settings needed to open a database
// connection. Deployment migrations intentionally use this instead of Load:
// schema upgrades must not be coupled to runtime-only requirements such as a
// JWT signing key or third-party API configuration.
func LoadDatabase() DatabaseConfig {
	return DatabaseConfig{
		Host:        getEnv("DB_HOST", "localhost"),
		Port:        getEnv("DB_PORT", "15433"),
		Name:        getEnv("DB_NAME", "iac_platform"),
		User:        getEnv("DB_USER", "postgres"),
		Password:    getEnv("DB_PASSWORD", "postgres123"),
		SSLMode:     getEnv("DB_SSLMODE", "require"),
		SSLRootCert: getEnv("DB_SSLROOTCERT", ""),
	}
}

func requireEnv(key string) string {
	value := os.Getenv(key)
	if value == "" {
		panic(key + " environment variable is required but not set")
	}
	return value
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if v, err := strconv.Atoi(value); err == nil {
			return v
		}
	}
	return defaultValue
}

// GetSchemaSolverMaxRetries 获取 SchemaSolver AI 反馈循环最大重试次数
// 环境变量: SCHEMA_SOLVER_MAX_RETRIES，默认 2
func GetSchemaSolverMaxRetries() int {
	v := getEnvInt("SCHEMA_SOLVER_MAX_RETRIES", 2)
	if v < 1 {
		return 1
	}
	if v > 5 {
		return 5
	}
	return v
}
