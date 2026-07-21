package provider

import (
	"context"
	"fmt"
	"sync"
)

// Mock is a scripted Provider for tests: each call pops the next step. A
// step is either a Response or an error to return.
type Mock struct {
	mu       sync.Mutex
	steps    []mockStep
	Requests []Request // every request received, for prompt assertions
}

type mockStep struct {
	resp *Response
	err  error
}

func (m *Mock) Respond(resp Response) *Mock {
	m.steps = append(m.steps, mockStep{resp: &resp})
	return m
}

func (m *Mock) Fail(err error) *Mock {
	m.steps = append(m.steps, mockStep{err: err})
	return m
}

// RespondToolUse scripts one turn that calls the named tool with the given
// JSON input — used to drive multi-turn tool sequences (read/edit/command)
// as well as the final outcome tool.
func (m *Mock) RespondToolUse(tool, inputJSON string, usage Usage) *Mock {
	return m.RespondOutcome(tool, inputJSON, usage)
}

// RespondOutcome scripts a response that calls the named outcome tool with
// the given JSON input — the common case for reviewer tests.
func (m *Mock) RespondOutcome(tool, inputJSON string, usage Usage) *Mock {
	return m.Respond(Response{
		StopReason: "tool_use",
		Usage:      usage,
		Blocks: []Block{{
			Type:      "tool_use",
			ToolUseID: fmt.Sprintf("toolu_%d", len(m.steps)+1),
			ToolName:  tool,
			ToolInput: []byte(inputJSON),
		}},
	})
}

func (m *Mock) Complete(_ context.Context, req Request) (*Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Requests = append(m.Requests, req)
	if len(m.steps) == 0 {
		return nil, fmt.Errorf("mock provider: no scripted response for call %d", len(m.Requests))
	}
	step := m.steps[0]
	m.steps = m.steps[1:]
	if step.err != nil {
		return nil, step.err
	}
	return step.resp, nil
}

// Remaining reports how many scripted steps are unconsumed (test diagnostics).
func (m *Mock) Remaining() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.steps)
}
