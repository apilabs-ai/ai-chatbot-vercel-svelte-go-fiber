package config

import (
	"testing"
)

func TestLoad_requiresAPIKey(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error when OPENAI_API_KEY is empty")
	}
}

func TestLoad_defaults(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-test")
	t.Setenv("PORT", "")
	t.Setenv("APP_ENV", "")
	t.Setenv("CORS_ALLOW_ORIGINS", "")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.Port != "8080" {
		t.Fatalf("Port = %q", c.Port)
	}
	if c.AppEnv != "development" {
		t.Fatalf("AppEnv = %q", c.AppEnv)
	}
	if c.CORSAllowOrigins != "http://localhost:5173" {
		t.Fatalf("CORS = %q", c.CORSAllowOrigins)
	}
	if c.OpenAIAPIKey != "sk-test" {
		t.Fatal("OpenAIAPIKey not set")
	}
}

func TestLoad_customEnv(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "k")
	t.Setenv("PORT", "9999")
	t.Setenv("APP_ENV", "production")
	t.Setenv("CORS_ALLOW_ORIGINS", "https://a.com,https://b.com")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.Port != "9999" || c.AppEnv != "production" {
		t.Fatalf("%+v", c)
	}
	if c.CORSAllowOrigins != "https://a.com,https://b.com" {
		t.Fatalf("CORS = %q", c.CORSAllowOrigins)
	}
}

func TestLoad_modelsAllowList(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "k")
	t.Setenv("OPENAI_MODEL", "gpt-4o")
	t.Setenv("OPENAI_MODELS", "")
	c, _ := Load()
	if c.Models[0] != "gpt-4o" || len(c.Models) != 3 {
		t.Fatalf("default model first, no duplicates: %v", c.Models)
	}
	t.Setenv("OPENAI_MODELS", " a , b,a")
	c, _ = Load()
	if len(c.Models) != 2 || c.Models[0] != "a" {
		t.Fatalf("custom list: %v", c.Models)
	}
}
