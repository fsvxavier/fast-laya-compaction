package compaction

import (
	"fmt"
	"strings"
)

func isPinned(index, total, preserveRecent int) bool {
	return index == 0 || index >= total-preserveRecent
}

// CollectToolCalls pairs every tool_use with its tool_result by tool_use_id.
// Calls without a result are not candidates (there is nothing to drop yet).
func CollectToolCalls(messages []Message, preserveRecent int) []ToolCall {
	type found struct {
		index int
		text  string
		isErr bool
	}
	results := map[string]found{}
	for i, m := range messages {
		for _, r := range m.ToolResults {
			results[r.ToolUseID] = found{index: i, text: r.Text, isErr: r.IsError}
		}
	}
	var calls []ToolCall
	for callIndex, m := range messages {
		for _, tool := range m.ToolUses {
			res, ok := results[tool.ToolUseID]
			if !ok {
				continue
			}
			calls = append(calls, ToolCall{
				ID:          fmt.Sprintf("t%d", len(calls)+1),
				ToolUseID:   tool.ToolUseID,
				Tool:        tool.Tool,
				Input:       tool.Input,
				CallIndex:   callIndex,
				ResultIndex: res.index,
				ResultChars: runeLen(res.text),
				IsError:     res.isErr,
				Pinned: isPinned(callIndex, len(messages), preserveRecent) ||
					isPinned(res.index, len(messages), preserveRecent),
			})
		}
	}
	return calls
}

// GoalFromMessages is the last three user prompts, the default goal.
func GoalFromMessages(messages []Message) string {
	var prompts []string
	for _, m := range messages {
		if m.Role == RoleUser && strings.TrimSpace(m.Text) != "" && len(m.ToolResults) == 0 {
			prompts = append(prompts, truncate(m.Text, 500))
		}
	}
	if len(prompts) > 3 {
		prompts = prompts[len(prompts)-3:]
	}
	return strings.Join(prompts, "\n")
}

// MessageChars is the characters of text, tool input and tool output a message holds.
func MessageChars(m Message) int {
	total := runeLen(m.Text)
	for _, t := range m.ToolUses {
		if s, ok := compactJSON(t.Input); ok {
			total += runeLen(s)
		} else {
			total += 20
		}
	}
	for _, r := range m.ToolResults {
		total += runeLen(r.Text)
	}
	return total
}
