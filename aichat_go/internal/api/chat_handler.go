package api

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sync"

	"github.com/gofiber/fiber/v3"

	"aichat_go/internal/chat"
	"aichat_go/internal/config"
	"aichat_go/internal/store"
)

type streamRequest struct {
	Input          string   `json:"input"`
	ConversationID string   `json:"conversation_id"`
	TenantID       string   `json:"tenant_id"`
	WorkspaceID    string   `json:"workspace_id"`
	Model          string   `json:"model"`
	Tools          []string `json:"tools"`
}

func StreamChat(cfg *config.Config, _ *store.InMemoryStore, chatSvc *chat.Service) fiber.Handler {
	return func(c fiber.Ctx) error {
		var req streamRequest
		if err := c.Bind().Body(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid body"})
		}
		if req.Input == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "input is required"})
		}
		// Only allow-listed models: the client must not pick arbitrary (costly) models.
		if req.Model != "" && !slices.Contains(cfg.Models, req.Model) {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "model not allowed"})
		}

		tenantID := getString(c, "tenant_id", req.TenantID)
		workspaceID := getString(c, "workspace_id", req.WorkspaceID)
		userID := getString(c, "user_id", "anonymous")

		input := &chat.StreamInput{
			Input:          req.Input,
			ConversationID: req.ConversationID,
			TenantID:       tenantID,
			WorkspaceID:    workspaceID,
			UserID:         userID,
			Model:          req.Model,
			Tools:          req.Tools,
		}

		c.Set("Content-Type", "text/event-stream")
		c.Set("Cache-Control", "no-cache")
		c.Set("Connection", "keep-alive")
		c.Set("X-Accel-Buffering", "no")

		ctx, cancel := context.WithCancel(context.Background())
		return c.SendStreamWriter(func(w *bufio.Writer) {
			defer cancel()
			var mu sync.Mutex
			send := func(ev chat.StreamEvent) {
				data, _ := json.Marshal(ev.Data)
				mu.Lock()
				fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Event, data)
				w.Flush()
				mu.Unlock()
			}
			_ = chatSvc.Stream(ctx, input, send)
		})
	}
}

func getString(c fiber.Ctx, key, fallback string) string {
	if v := c.Locals(key); v != nil {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return fallback
}
