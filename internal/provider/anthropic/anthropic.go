// Package anthropic implements provider.Provider over the official Go SDK
// (DEC-001, D-2). Cached-token counts are captured for the ledger; prompt
// assembly keeps stable prefixes so caching pays (DESIGN-002 §4).
package anthropic

import (
	"context"
	"errors"
	"fmt"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"subutai/internal/provider"
)

type Client struct {
	c sdk.Client
}

// New builds a client for the Anthropic wire protocol. baseURL, when
// non-empty, points at a compatible endpoint (e.g. DeepSeek's /anthropic
// gateway) — one protocol implementation, many endpoints (D-2).
func New(apiKey, baseURL string) *Client {
	opts := []option.RequestOption{option.WithAPIKey(apiKey)}
	if baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}
	return &Client{c: sdk.NewClient(opts...)}
}

func (a *Client) Complete(ctx context.Context, req provider.Request) (*provider.Response, error) {
	params := sdk.MessageNewParams{
		Model:     sdk.Model(req.Model),
		MaxTokens: int64(req.MaxTokens),
	}
	if req.System != "" {
		params.System = []sdk.TextBlockParam{{Text: req.System}}
	}
	for _, m := range req.Messages {
		var blocks []sdk.ContentBlockParamUnion
		for _, b := range m.Blocks {
			switch b.Type {
			case "text":
				blocks = append(blocks, sdk.NewTextBlock(b.Text))
			case "tool_use":
				blocks = append(blocks, sdk.NewToolUseBlock(b.ToolUseID, b.ToolInput, b.ToolName))
			case "tool_result":
				blocks = append(blocks, sdk.NewToolResultBlock(b.ToolUseID, b.Result, b.IsError))
			default:
				return nil, fmt.Errorf("unsupported block type %q", b.Type)
			}
		}
		switch m.Role {
		case "user":
			params.Messages = append(params.Messages, sdk.NewUserMessage(blocks...))
		case "assistant":
			params.Messages = append(params.Messages, sdk.NewAssistantMessage(blocks...))
		default:
			return nil, fmt.Errorf("unsupported role %q", m.Role)
		}
	}
	for _, t := range req.Tools {
		params.Tools = append(params.Tools, sdk.ToolUnionParam{OfTool: &sdk.ToolParam{
			Name:        t.Name,
			Description: sdk.String(t.Description),
			InputSchema: sdk.ToolInputSchemaParam{
				Properties: t.InputSchema["properties"],
				Required:   toStringSlice(t.InputSchema["required"]),
			},
		}})
	}

	msg, err := a.c.Messages.New(ctx, params)
	if err != nil {
		return nil, classify(err)
	}

	resp := &provider.Response{
		StopReason: string(msg.StopReason),
		Usage: provider.Usage{
			Input:      msg.Usage.InputTokens,
			Output:     msg.Usage.OutputTokens,
			CacheRead:  msg.Usage.CacheReadInputTokens,
			CacheWrite: msg.Usage.CacheCreationInputTokens,
		},
	}
	for _, block := range msg.Content {
		switch block.Type {
		case "text":
			resp.Blocks = append(resp.Blocks, provider.Block{Type: "text", Text: block.Text})
		case "tool_use":
			resp.Blocks = append(resp.Blocks, provider.Block{
				Type:      "tool_use",
				ToolUseID: block.ID,
				ToolName:  block.Name,
				ToolInput: block.Input,
			})
		}
	}
	return resp, nil
}

// classify wraps 429/5xx/network failures as transient (FR-8.2).
func classify(err error) error {
	var apiErr *sdk.Error
	if errors.As(err, &apiErr) {
		if apiErr.StatusCode == 429 || apiErr.StatusCode >= 500 {
			return &provider.TransientError{Status: apiErr.StatusCode, Err: err}
		}
		return err // 4xx other than 429: caller's request is wrong; retrying is noise
	}
	// No HTTP status at all: network-level failure, worth retrying.
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return &provider.TransientError{Err: err}
}

func toStringSlice(v any) []string {
	items, ok := v.([]any)
	if !ok {
		if s, ok := v.([]string); ok {
			return s
		}
		return nil
	}
	var out []string
	for _, it := range items {
		if s, ok := it.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
