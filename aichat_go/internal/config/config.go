package config

import (
	"fmt"
	"os"
	"slices"
	"strings"
)

type Config struct {
	Port             string
	AppEnv           string
	OpenAIAPIKey     string
	CORSAllowOrigins string
	// Models is the allow-list the UI may pick from; Models[0] is the default.
	Models []string
}

func Load() (*Config, error) {
	c := &Config{
		Port:             getEnv("PORT", "8080"),
		AppEnv:           getEnv("APP_ENV", "development"),
		OpenAIAPIKey:     os.Getenv("OPENAI_API_KEY"),
		CORSAllowOrigins: getEnv("CORS_ALLOW_ORIGINS", "http://localhost:5173"),
	}
	for _, m := range strings.Split(getEnv("OPENAI_MODELS", getEnv("OPENAI_MODEL", "gpt-4o-mini")+",gpt-4o,gpt-5-mini,o4-mini"), ",") {
		if m = strings.TrimSpace(m); m != "" && !slices.Contains(c.Models, m) {
			c.Models = append(c.Models, m)
		}
	}

	if c.OpenAIAPIKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY is required")
	}

	return c, nil
}

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return strings.TrimSpace(v)
	}
	return defaultVal
}
