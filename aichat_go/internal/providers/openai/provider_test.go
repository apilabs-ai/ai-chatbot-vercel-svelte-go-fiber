package openai

import (
	"encoding/json"
	"testing"

	"aichat_go/internal/chat"

	"github.com/openai/openai-go/v3/responses"
)

func TestInferIntentFromPrompt(t *testing.T) {
	tests := []struct {
		prompt string
		want   structuredIntent
	}{
		{"create plan for database migration", intentPlan},
		{"create a task list to track deploy", intentQueue},
		{"ask for approval before delete", intentConfirmation},
		{"create deployment runbook template", intentArtifact},
		{"explain svelte with citations and resources", intentCitation},
		{"hello there", intentNone},
	}

	for _, tt := range tests {
		got := inferIntentFromPrompt(tt.prompt)
		if got != tt.want {
			t.Fatalf("inferIntentFromPrompt(%q) = %q, want %q", tt.prompt, got, tt.want)
		}
	}
}

func TestBuildStructuredPart(t *testing.T) {
	planContent := "1. Assess schema\n2. Create migration\n3. Validate data"
	plan, ok := buildStructuredPart(intentPlan, "plan migration", planContent)
	if !ok || plan.Type != "plan" {
		t.Fatalf("expected plan part, got ok=%v type=%q", ok, plan.Type)
	}

	queueContent := "- Setup\n- Test\n- Deploy"
	queue, ok := buildStructuredPart(intentQueue, "task list", queueContent)
	if !ok || queue.Type != "queue" {
		t.Fatalf("expected queue part, got ok=%v type=%q", ok, queue.Type)
	}

	citationContent := "Read [Svelte Docs](https://svelte.dev/docs) and https://developer.mozilla.org."
	citation, ok := buildStructuredPart(intentCitation, "with citations", citationContent)
	if !ok || citation.Type != "citation" {
		t.Fatalf("expected citation part, got ok=%v type=%q", ok, citation.Type)
	}
}

func feed(t *testing.T, st *streamState, events ...string) []chat.StreamEvent {
	t.Helper()
	out := make(chan chat.StreamEvent, 32)
	for _, raw := range events {
		var ev responses.ResponseStreamEventUnion
		if err := json.Unmarshal([]byte(raw), &ev); err != nil {
			t.Fatal(err)
		}
		st.handle(ev, out)
	}
	st.finish("hi", out)
	close(out)
	var got []chat.StreamEvent
	for ev := range out {
		got = append(got, ev)
	}
	return got
}

func TestStreamState_reasoningSourcesImageUsage(t *testing.T) {
	got := feed(t, &streamState{},
		`{"type":"response.reasoning_summary_text.delta","delta":"thinking"}`,
		`{"type":"response.output_text.delta","delta":"Hello"}`,
		`{"type":"response.output_text.annotation.added","annotation":{"type":"url_citation","url":"https://a.dev","title":"A"}}`,
		`{"type":"response.output_text.annotation.added","annotation":{"type":"url_citation","url":"https://a.dev","title":"A"}}`,
		`{"type":"response.output_item.done","item":{"type":"image_generation_call","result":"QUJD","output_format":"webp"}}`,
		`{"type":"response.completed","response":{"usage":{"input_tokens":7,"output_tokens":3}}}`,
	)
	want := []string{"reasoning.delta", "message.delta", "part", "part", "usage", "_stream_done"}
	if len(got) != len(want) {
		t.Fatalf("got %d events %+v", len(got), got)
	}
	for i, ev := range got {
		if ev.Event != want[i] {
			t.Fatalf("event %d = %q, want %q", i, ev.Event, want[i])
		}
	}
	img := got[2].Data.(chat.PartEvent)
	if img.Type != "image" || img.Content != "QUJD" || img.Meta["media_type"] != "image/webp" {
		t.Fatalf("image part = %+v", img)
	}
	src := got[3].Data.(chat.PartEvent)
	if src.Type != "sources" || len(src.Meta["sources"].([]map[string]string)) != 1 {
		t.Fatalf("sources part not deduped: %+v", src)
	}
	if u := got[4].Data.(map[string]int); u["input_tokens"] != 7 || u["output_tokens"] != 3 {
		t.Fatalf("usage = %v", u)
	}
}

func TestStreamState_failed(t *testing.T) {
	st := &streamState{}
	out := make(chan chat.StreamEvent, 4)
	var ev responses.ResponseStreamEventUnion
	_ = json.Unmarshal([]byte(`{"type":"response.failed","response":{"error":{"message":"boom"}}}`), &ev)
	st.handle(ev, out)
	if !st.failed || (<-out).Data.(map[string]string)["message"] != "boom" {
		t.Fatal("expected error event with message")
	}
}

func TestIsReasoningModel(t *testing.T) {
	for m, want := range map[string]bool{"o4-mini": true, "gpt-5-mini": true, "gpt-4o": false, "gpt-4o-mini": false} {
		if IsReasoningModel(m) != want {
			t.Fatalf("IsReasoningModel(%q) != %v", m, want)
		}
	}
}
