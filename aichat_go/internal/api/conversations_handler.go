package api

import (
	"aichat_go/internal/config"
	"aichat_go/internal/providers/openai"
	"aichat_go/internal/store"
	"github.com/gofiber/fiber/v3"
)

func ListConversations(store *store.InMemoryStore) fiber.Handler {
	return func(c fiber.Ctx) error {
		tenantID := getString(c, "tenant_id", c.Query("tenant_id", "default"))
		list, err := store.ListConversations(tenantID)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		out := make([]fiber.Map, 0, len(list))
		for _, conv := range list {
			out = append(out, fiber.Map{
				"id":           conv.ID,
				"tenant_id":    conv.TenantID,
				"workspace_id": conv.WorkspaceID,
				"user_id":      conv.UserID,
				"title":        conv.Title,
				"created_at":   conv.CreatedAt,
				"updated_at":   conv.UpdatedAt,
			})
		}
		return c.JSON(out)
	}
}

func GetConversationMessages(store *store.InMemoryStore) fiber.Handler {
	return func(c fiber.Ctx) error {
		convID := c.Params("id")
		if convID == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "conversation id required"})
		}
		msgs, err := store.GetMessages(convID)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		out := make([]fiber.Map, 0, len(msgs))
		for _, m := range msgs {
			out = append(out, fiber.Map{
				"id":              m.ID,
				"conversation_id": m.ConversationID,
				"role":            m.Role,
				"content":         m.Content,
				"provider":        m.Provider,
				"model":           m.Model,
				"input_tokens":    m.InputTokens,
				"output_tokens":   m.OutputTokens,
				"created_at":      m.CreatedAt,
			})
		}
		return c.JSON(out)
	}
}

// RestoreConversation truncates a conversation to a checkpoint: every message
// after message_id is dropped, so the next turn continues from that point.
func RestoreConversation(store *store.InMemoryStore) fiber.Handler {
	return func(c fiber.Ctx) error {
		var req struct {
			MessageID string `json:"message_id"`
		}
		if err := c.Bind().Body(&req); err != nil || req.MessageID == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "message_id is required"})
		}
		convID := c.Params("id")
		conv, _ := store.GetConversation(convID)
		tenantID := getString(c, "tenant_id", "")
		if conv == nil || (tenantID != "" && conv.TenantID != tenantID) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "conversation not found"})
		}
		ok, err := store.TruncateAfter(convID, req.MessageID)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		if !ok {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "message not found"})
		}
		return c.JSON(fiber.Map{"ok": true})
	}
}

// ListModels returns the allow-listed models; the first is the default.
func ListModels(cfg *config.Config) fiber.Handler {
	return func(c fiber.Ctx) error {
		out := make([]fiber.Map, 0, len(cfg.Models))
		for i, m := range cfg.Models {
			out = append(out, fiber.Map{"id": m, "reasoning": openai.IsReasoningModel(m), "default": i == 0})
		}
		return c.JSON(out)
	}
}
