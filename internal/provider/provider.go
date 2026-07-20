// Package provider abstracts one model API call (DESIGN-002 §4 step 5). One
// interface, one real implementation (Anthropic, per D-2), one mock — a
// second implementation is deliberately not written until a second provider
// is configured.
package provider

import (
	"context"
	"encoding/json"
	"errors"
)

// Request is one model call. Messages carry the running agent loop; the
// dispatcher owns turn management.
type Request struct {
	Model     string
	System    string
	MaxTokens int
	Messages  []Message
	Tools     []ToolDef
}

type Message struct {
	Role   string // "user" | "assistant"
	Blocks []Block
}

type Block struct {
	Type      string          `json:"type"` // "text" | "tool_use" | "tool_result"
	Text      string          `json:"text,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	ToolName  string          `json:"tool_name,omitempty"`
	ToolInput json.RawMessage `json:"tool_input,omitempty"`
	Result    string          `json:"result,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
}

func TextBlock(s string) Block  { return Block{Type: "text", Text: s} }
func UserText(s string) Message { return Message{Role: "user", Blocks: []Block{TextBlock(s)}} }

type ToolDef struct {
	Name        string
	Description string
	InputSchema map[string]any // JSON Schema for the input object
}

type Usage struct {
	Input      int64
	Output     int64
	CacheRead  int64
	CacheWrite int64
}

func (u *Usage) Add(v Usage) {
	u.Input += v.Input
	u.Output += v.Output
	u.CacheRead += v.CacheRead
	u.CacheWrite += v.CacheWrite
}

type Response struct {
	Blocks     []Block
	StopReason string // "end_turn" | "tool_use" | "max_tokens"
	Usage      Usage
}

// ToolUse returns the first tool_use block, or nil.
func (r *Response) ToolUse() *Block {
	for i := range r.Blocks {
		if r.Blocks[i].Type == "tool_use" {
			return &r.Blocks[i]
		}
	}
	return nil
}

type Provider interface {
	Complete(ctx context.Context, req Request) (*Response, error)
}

// TransientError marks a provider failure worth retrying with backoff
// (429/5xx/network — FR-8.2). Non-transient errors fail the attempt.
type TransientError struct {
	Status int
	Err    error
}

func (e *TransientError) Error() string { return e.Err.Error() }
func (e *TransientError) Unwrap() error { return e.Err }

func IsTransient(err error) bool {
	var t *TransientError
	return errors.As(err, &t)
}
