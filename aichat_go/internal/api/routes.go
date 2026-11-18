package api

import (
	"log/slog"

	"github.com/gofiber/fiber/v3"

	"aichat_go/internal/chat"
	"aichat_go/internal/config"
	"aichat_go/internal/providers/openai"
	"aichat_go/internal/store"
)

func RegisterV1(v1 fiber.Router, cfg *config.Config, logger *slog.Logger) {
	v1.Use(TenantFromHeader())

	memStore := store.NewInMemoryStore()
	provider := openai.NewProvider(cfg.OpenAIAPIKey, cfg.Models[0])
	chatSvc := chat.NewChatService(memStore, provider, logger)

	v1.Post("/responses:stream", StreamChat(cfg, memStore, chatSvc))
	v1.Get("/conversations", ListConversations(memStore))
	v1.Get("/conversations/:id/messages", GetConversationMessages(memStore))
	v1.Post("/conversations/:id/restore", RestoreConversation(memStore))
	v1.Get("/models", ListModels(cfg))
}
