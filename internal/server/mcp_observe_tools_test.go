package server

import (
	"strings"
	"testing"

	"subutai/internal/store"
)

// TestCompactTranscript is SPEC-017 SD-10: long text is cut and says so; tool
// results are left out unless asked for, and cut when they are; and a run of
// more than sixty turns keeps its first and last thirty.
func TestCompactTranscript(t *testing.T) {
	long := strings.Repeat("x", 5000)
	p := &runPage{Prompt: &store.TranscriptEntry{Kind: store.EntryPrompt, Content: long}}
	for n := 1; n <= 70; n++ {
		result := &store.TranscriptEntry{Kind: store.EntryToolResult, Content: long, ToolUseID: "u"}
		p.Turns = append(p.Turns, turnView{N: n, Items: []turnItem{
			{Kind: "text", Entry: store.TranscriptEntry{Kind: store.EntryText, Content: "turn text"}},
			{Kind: "tool", Entry: store.TranscriptEntry{Kind: store.EntryToolCall, ToolName: "read_file"}, Arg: "a.go", Result: result},
		}})
	}

	out := mcpTranscript(p, false)
	prompt := out["prompt"].(map[string]any)
	if prompt["truncated"] != true || len([]rune(prompt["text"].(string))) != runTextLimit+1 {
		t.Errorf("the prompt should be cut to %d characters and say so", runTextLimit)
	}
	turns := out["turns"].([]any)
	if len(turns) != runTurnLimit || out["turns_omitted"] != 10 {
		t.Fatalf("turns = %d, omitted = %v", len(turns), out["turns_omitted"])
	}
	if first, last := turns[0].(map[string]any)["turn"], turns[len(turns)-1].(map[string]any)["turn"]; first != 1 || last != 70 {
		t.Errorf("first and last kept: %v … %v", first, last)
	}
	tool := turns[0].(map[string]any)["items"].([]any)[1].(map[string]any)
	if _, has := tool["result"]; has || out["tool_results"] == nil {
		t.Errorf("tool results are left out unless asked: %v", tool)
	}

	out = mcpTranscript(p, true)
	tool = out["turns"].([]any)[0].(map[string]any)["items"].([]any)[1].(map[string]any)
	res, ok := tool["result"].(map[string]any)
	if !ok || res["truncated"] != true || len([]rune(res["text"].(string))) != runResultLimit+1 {
		t.Errorf("asked for, a tool result is cut to %d characters: %v", runResultLimit, tool["result"])
	}
}

// TestWriterAndVerdictSentences is SPEC-017 FR-2.4, over records built by
// hand.
func TestWriterAndVerdictSentences(t *testing.T) {
	agent := store.Writer{Act: store.ActWrote, Kind: store.WriterAgent, Actor: "spec-author", Model: "m1"}
	chat := store.Writer{Act: store.ActAdded, Kind: store.WriterChat, Actor: "chat-agent"}
	person := store.Writer{Act: store.ActAdded, Kind: store.WriterPerson, Actor: "sam"}
	opened := store.Writer{Act: store.ActOpenedRevision, Kind: store.WriterPerson, Actor: "sam", Via: "ui"}
	revised := store.Writer{Act: store.ActRevised, Kind: store.WriterAgent, Actor: "spec-author", Model: "m1"}
	for _, tc := range []struct {
		ws   []store.Writer
		want string
	}{
		{nil, "Who wrote this wasn't recorded."},
		{[]store.Writer{agent}, "Written by the spec author (m1)."},
		{[]store.Writer{chat}, "Written by the chat agent."},
		{[]store.Writer{person}, "Added by sam."},
		{[]store.Writer{chat, revised}, "Written by the chat agent. Revised by the spec author (m1)."},
		{[]store.Writer{chat, opened}, "Written by the chat agent. Revision opened by sam."},
		{[]store.Writer{chat, revised, opened}, "Written by the chat agent. Revision opened by sam."},
		{[]store.Writer{{Act: store.ActAdded, Kind: store.WriterChat, Actor: "chat-agent", Inferred: true}},
			"Written by the chat agent (from the audit trail)."},
	} {
		if got := writerSentence(tc.ws); got != tc.want {
			t.Errorf("writerSentence(%v) = %q, want %q", tc.ws, got, tc.want)
		}
	}
	for _, tc := range []struct {
		v    store.Verdict
		want string
	}{
		{store.Verdict{Verdict: "approve", Kind: "agent", Actor: "spec-reviewer", Model: "m2"}, "Approved by the spec reviewer (m2)."},
		{store.Verdict{Verdict: "approve", Kind: "agent", Actor: "spec-reviewer", Model: "m2", Held: true}, "Approved by the spec reviewer (m2), and held for a person."},
		{store.Verdict{Verdict: "approve", Kind: "agent", Actor: "spec-reviewer", Model: "m2", ReleasedBy: "sam", ReleasedVia: "ui"},
			"Approved by the spec reviewer (m2); sam let the reviewer decide."},
		{store.Verdict{Verdict: "send_back", Kind: "person", Actor: "chat-agent", Via: "mcp", Quote: "No."}, "Sent back by a person, relayed by the chat agent: “No.”"},
		{store.Verdict{Verdict: "approve", Kind: "person", Actor: "sam", Via: "ui"}, "Approved by sam."},
		{store.Verdict{Verdict: "approve", Kind: "person", Actor: "sam", Via: "escalation"}, "Approved by sam, answering the reviewer's escalation."},
	} {
		if got := verdictSentence(tc.v); got != tc.want {
			t.Errorf("verdictSentence = %q, want %q", got, tc.want)
		}
	}
}
