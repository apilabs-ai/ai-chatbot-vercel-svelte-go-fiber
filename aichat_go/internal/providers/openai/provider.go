package openai

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	"aichat_go/internal/chat"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
)

type Provider struct {
	client       openai.Client
	defaultModel string
}

type structuredIntent string

const (
	intentNone         structuredIntent = ""
	intentPlan         structuredIntent = "plan"
	intentArtifact     structuredIntent = "artifact"
	intentConfirmation structuredIntent = "confirmation"
	intentQueue        structuredIntent = "queue"
	intentCitation     structuredIntent = "citation"
)

func NewProvider(apiKey, defaultModel string) *Provider {
	if defaultModel == "" {
		defaultModel = "gpt-4o-mini"
	}
	return &Provider{
		client:       openai.NewClient(option.WithAPIKey(apiKey)),
		defaultModel: defaultModel,
	}
}

// IsReasoningModel reports whether a model emits reasoning summaries.
// ponytail: prefix heuristic; move to per-model config if a model breaks the pattern.
func IsReasoningModel(model string) bool {
	return strings.HasPrefix(model, "o") || strings.HasPrefix(model, "gpt-5")
}

func (p *Provider) Stream(ctx context.Context, input *chat.StreamInput, history []chat.MessageRecord, out chan<- chat.StreamEvent) {
	defer close(out)

	model := p.defaultModel
	if input.Model != "" {
		model = input.Model
	}

	// history already ends with the current user message (service appends it first).
	items := make(responses.ResponseInputParam, 0, len(history))
	for _, h := range history {
		role := responses.EasyInputMessageRoleUser
		if h.Role == "assistant" {
			role = responses.EasyInputMessageRoleAssistant
		}
		items = append(items, responses.ResponseInputItemParamOfMessage(h.Content, role))
	}

	params := responses.ResponseNewParams{
		Model: shared.ResponsesModel(model),
		Input: responses.ResponseNewParamsInputUnion{OfInputItemList: items},
		Store: openai.Bool(false),
	}
	if IsReasoningModel(model) {
		params.Reasoning = shared.ReasoningParam{Summary: shared.ReasoningSummaryAuto}
	}
	for _, t := range input.Tools {
		switch t {
		case "web_search":
			params.Tools = append(params.Tools, responses.ToolParamOfWebSearch(responses.WebSearchToolTypeWebSearch))
		case "image_generation":
			params.Tools = append(params.Tools, responses.ToolUnionParam{OfImageGeneration: &responses.ToolImageGenerationParam{}})
		}
	}

	st := &streamState{intent: inferIntentFromPrompt(input.Input)}
	stream := p.client.Responses.NewStreaming(ctx, params)
	for stream.Next() {
		st.handle(stream.Current(), out)
	}
	if err := stream.Err(); err != nil {
		out <- chat.StreamEvent{Event: "error", Data: map[string]string{"message": err.Error()}}
		return
	}
	if st.failed {
		return
	}
	st.finish(input.Input, out)
}

// streamState accumulates one Responses API stream. Kept free of network types
// beyond the event union so it can be driven from tests.
type streamState struct {
	intent      structuredIntent
	content     string
	sources     []map[string]string
	seenSources map[string]bool
	inputTok    int
	outputTok   int
	failed      bool
}

func (st *streamState) handle(ev responses.ResponseStreamEventUnion, out chan<- chat.StreamEvent) {
	switch ev.Type {
	case "response.output_text.delta":
		if ev.Delta == "" {
			return
		}
		st.content += ev.Delta
		// For structured intents, buffer and emit a typed part at the end.
		if st.intent == intentNone {
			out <- chat.StreamEvent{Event: "message.delta", Data: map[string]string{"delta": ev.Delta}}
		}
	case "response.reasoning_summary_text.delta":
		if ev.Delta != "" {
			out <- chat.StreamEvent{Event: "reasoning.delta", Data: map[string]string{"delta": ev.Delta}}
		}
	case "response.output_text.annotation.added":
		a := ev.Annotation
		if a.Type != "url_citation" || a.URL == "" {
			return
		}
		if st.seenSources == nil {
			st.seenSources = map[string]bool{}
		}
		if st.seenSources[a.URL] {
			return
		}
		st.seenSources[a.URL] = true
		title := a.Title
		if title == "" {
			title = a.URL
		}
		st.sources = append(st.sources, map[string]string{"title": title, "url": a.URL})
	case "response.output_item.done":
		if ev.Item.Type == "image_generation_call" && ev.Item.Result != "" {
			format := ev.Item.OutputFormat
			if format == "" {
				format = "png"
			}
			out <- chat.StreamEvent{Event: "part", Data: chat.PartEvent{
				Type:    "image",
				Content: ev.Item.Result,
				Meta:    map[string]interface{}{"media_type": "image/" + format},
			}}
		}
	case "response.completed":
		st.inputTok = int(ev.Response.Usage.InputTokens)
		st.outputTok = int(ev.Response.Usage.OutputTokens)
	case "response.failed", "error":
		msg := ev.Message
		if msg == "" {
			msg = ev.Response.Error.Message
		}
		if msg == "" {
			msg = "response failed"
		}
		st.failed = true
		out <- chat.StreamEvent{Event: "error", Data: map[string]string{"message": msg}}
	}
}

func (st *streamState) finish(prompt string, out chan<- chat.StreamEvent) {
	content := st.content
	if st.intent != intentNone {
		if part, ok := buildStructuredPart(st.intent, prompt, content); ok {
			out <- chat.StreamEvent{Event: "part", Data: part}
			content = ""
		} else if content != "" {
			// Fallback to plain text if we couldn't construct a valid structured part.
			out <- chat.StreamEvent{Event: "message.delta", Data: map[string]string{"delta": content}}
		}
	}
	if len(st.sources) > 0 {
		out <- chat.StreamEvent{Event: "part", Data: chat.PartEvent{
			Type: "sources",
			Meta: map[string]interface{}{"sources": st.sources},
		}}
	}
	out <- chat.StreamEvent{Event: "usage", Data: map[string]int{"input_tokens": st.inputTok, "output_tokens": st.outputTok}}
	out <- chat.StreamEvent{Event: "_stream_done", Data: map[string]string{"content": content}}
}

func inferIntentFromPrompt(prompt string) structuredIntent {
	p := strings.ToLower(strings.TrimSpace(prompt))
	if p == "" {
		return intentNone
	}

	if containsAny(p, "citation", "citations", "source", "sources", "reference", "references", "resource", "resources") {
		return intentCitation
	}
	if containsAny(p, "task list", "todo", "to-do", "queue", "track", "tracking", "pending", "completed") {
		return intentQueue
	}
	if containsAny(p, "approval", "approve", "confirm", "are you sure") {
		return intentConfirmation
	}
	if containsAny(p, "runbook", "checklist", "sop", "playbook", "spec", "proposal", "report", "template", "artifact") {
		return intentArtifact
	}
	if containsAny(p, "plan", "roadmap", "step-by-step", "step by step", "migration plan", "rollout plan") {
		return intentPlan
	}

	return intentNone
}

func containsAny(s string, hints ...string) bool {
	for _, h := range hints {
		if strings.Contains(s, h) {
			return true
		}
	}
	return false
}

func buildStructuredPart(intent structuredIntent, prompt, content string) (chat.PartEvent, bool) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return chat.PartEvent{}, false
	}

	switch intent {
	case intentPlan:
		steps := extractListItems(trimmed, 8)
		if len(steps) < 3 {
			return chat.PartEvent{}, false
		}
		return chat.PartEvent{
			Type: "plan",
			Meta: map[string]interface{}{
				"title":       "Execution Plan",
				"description": "Auto-selected by prompt intent",
				"steps":       steps,
			},
		}, true
	case intentQueue:
		items := extractListItems(trimmed, 12)
		if len(items) < 2 {
			return chat.PartEvent{}, false
		}
		todos := make([]map[string]interface{}, 0, len(items))
		for i, it := range items {
			todos = append(todos, map[string]interface{}{
				"id":          "t" + strconv.Itoa(i+1),
				"title":       it,
				"description": "",
				"status":      "pending",
			})
		}
		return chat.PartEvent{
			Type: "queue",
			Meta: map[string]interface{}{
				"messages": []map[string]interface{}{
					{"id": "m1", "text": "Task list generated from your request"},
				},
				"todos": todos,
			},
		}, true
	case intentConfirmation:
		desc := trimmed
		if len(desc) > 240 {
			desc = desc[:240]
		}
		return chat.PartEvent{
			Type: "confirmation",
			Meta: map[string]interface{}{
				"title":       "Action Required",
				"description": desc,
				"state":       "approval-requested",
				"approval":    map[string]interface{}{"id": "approval-1"},
			},
		}, true
	case intentArtifact:
		return chat.PartEvent{
			Type:    "artifact",
			Content: trimmed,
			Meta: map[string]interface{}{
				"title":       inferArtifactTitle(trimmed),
				"description": "Auto-selected by prompt intent",
			},
		}, true
	case intentCitation:
		sources := extractSources(trimmed, 8)
		if len(sources) == 0 {
			return chat.PartEvent{}, false
		}
		return chat.PartEvent{
			Type:    "citation",
			Content: trimmed,
			Meta: map[string]interface{}{
				"sources": sources,
			},
		}, true
	default:
		return chat.PartEvent{}, false
	}
}

func extractListItems(text string, max int) []string {
	re := regexp.MustCompile(`(?m)^\s*(?:\d+[\).\s-]+|[-*]\s+)(.+)$`)
	matches := re.FindAllStringSubmatch(text, -1)
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		item := strings.TrimSpace(m[1])
		if item == "" {
			continue
		}
		out = append(out, item)
		if len(out) >= max {
			break
		}
	}
	return out
}

func inferArtifactTitle(text string) string {
	re := regexp.MustCompile(`(?m)^#{1,3}\s+(.+)$`)
	if m := re.FindStringSubmatch(text); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	line := strings.TrimSpace(strings.SplitN(text, "\n", 2)[0])
	if line == "" {
		return "Artifact"
	}
	if len(line) > 80 {
		return line[:80]
	}
	return line
}

func extractSources(text string, max int) []map[string]string {
	seen := map[string]bool{}
	out := make([]map[string]string, 0, max)

	mdLinkRe := regexp.MustCompile(`\[([^\]]+)\]\((https?://[^\s)]+)\)`)
	for _, m := range mdLinkRe.FindAllStringSubmatch(text, -1) {
		url := strings.TrimSpace(m[2])
		if url == "" || seen[url] {
			continue
		}
		seen[url] = true
		title := strings.TrimSpace(m[1])
		if title == "" {
			title = "Source"
		}
		out = append(out, map[string]string{"title": title, "url": url})
		if len(out) >= max {
			return out
		}
	}

	urlRe := regexp.MustCompile(`https?://[^\s)]+`)
	for _, u := range urlRe.FindAllString(text, -1) {
		url := strings.TrimSpace(u)
		if url == "" || seen[url] {
			continue
		}
		seen[url] = true
		out = append(out, map[string]string{"title": "Source", "url": url})
		if len(out) >= max {
			return out
		}
	}
	return out
}
