package compaction

import (
	"fmt"
	"strings"
)

func truncatedResultText(text string, isError bool, headChars int) string {
	n := runeLen(text)
	if n <= headChars+120 {
		return text
	}
	head := ""
	if headChars > 0 {
		head = sliceRunes(text, 0, headChars) + "\n"
	}
	suffix := ""
	if isError {
		suffix = " (error)"
	}
	return fmt.Sprintf("%s[fast-laya-compaction truncated %d chars of this tool result%s; re-run the tool if needed]", head, n-headChars, suffix)
}

// ApplyDecisions rebuilds the conversation from the decisions. A dropped call
// disappears together with its result; a dropped result keeps a bounded head
// and a note. Messages that lose all their content are removed. Each output
// message records its source so untouched ones can be mapped back.
func ApplyDecisions(messages []Message, decisions []Decision, calls []ToolCall, headChars int) []OutMessage {
	byID := map[string]ToolCall{}
	for _, c := range calls {
		byID[c.ID] = c
	}
	actions := map[string]Action{}
	for _, d := range decisions {
		if c, ok := byID[d.ID]; ok && d.Action != ActionKeep {
			actions[c.ToolUseID] = d.Action
		}
	}

	out := make([]OutMessage, 0, len(messages))
	for i, m := range messages {
		touched := false
		for _, t := range m.ToolUses {
			if _, ok := actions[t.ToolUseID]; ok {
				touched = true
			}
		}
		for _, r := range m.ToolResults {
			if _, ok := actions[r.ToolUseID]; ok {
				touched = true
			}
		}

		uses := make([]OutToolUse, 0, len(m.ToolUses))
		results := make([]OutToolResult, 0, len(m.ToolResults))
		changed := false
		for j, t := range m.ToolUses {
			action := actions[t.ToolUseID]
			if action == ActionDropCall {
				changed = true
				continue
			}
			ou := OutToolUse{SourceIndex: j, ToolUseID: t.ToolUseID, Tool: t.Tool, Input: t.Input, Text: t.Text, IsError: t.IsError}
			if action == ActionDropResult {
				if text := truncatedResultText(t.Text, t.IsError, headChars); text != t.Text {
					ou.Text, ou.Changed, changed = text, true, true
				}
			}
			uses = append(uses, ou)
		}
		for j, r := range m.ToolResults {
			action := actions[r.ToolUseID]
			if action == ActionDropCall {
				changed = true
				continue
			}
			or := OutToolResult{SourceIndex: j, ToolUseID: r.ToolUseID, Text: r.Text, IsError: r.IsError}
			if action == ActionDropResult {
				if text := truncatedResultText(r.Text, r.IsError, headChars); text != r.Text {
					or.Text, or.Changed, changed = text, true, true
				}
			}
			results = append(results, or)
		}

		if !touched || !changed {
			out = append(out, OutMessage{Source: i, Unchanged: true, Role: m.Role, Text: m.Text, ToolUses: uses, ToolResults: results})
			continue
		}
		if strings.TrimSpace(m.Text) == "" && len(uses) == 0 && len(results) == 0 {
			continue
		}
		out = append(out, OutMessage{Source: i, Role: m.Role, Text: m.Text, ToolUses: uses, ToolResults: results})
	}
	return out
}

func outMessageChars(m OutMessage) int {
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

// ReductionRatio is the share of characters the compaction removed.
func ReductionRatio(s Stats) float64 {
	if s.CharsBefore == 0 {
		return 0
	}
	return float64(s.CharsBefore-s.CharsAfter) / float64(s.CharsBefore)
}
