package compaction_test

import (
	"encoding/json"
	"strings"

	c "github.com/fsvxavier/fast-laya-compaction/internal/compaction"
)

func f(v float64) *float64 { return &v }

func msg(role c.Role, text string) c.Message { return c.Message{Role: role, Text: text} }

func call(id, tool, input, text string) c.Message {
	return c.Message{Role: c.RoleAssistant, ToolUses: []c.ToolUse{{ToolUseID: id, Tool: tool, Input: json.RawMessage(input), Text: text}}}
}

func result(id, text string, isError bool) c.Message {
	return c.Message{Role: c.RoleUser, ToolResults: []c.ToolResult{{ToolUseID: id, Text: text, IsError: isError}}}
}

var (
	fileA = strings.Repeat("export const a = 1;\n", 50)
	fileB = strings.Repeat("export const b = 2;\n", 50)
)

func transcript() []c.Message {
	return []c.Message{
		msg(c.RoleUser, "Never edit anything under src/generated. Fix the failing test."),
		call("tool-1", "Read", `{"file_path":"src/a.ts"}`, fileA),
		result("tool-1", fileA, false),
		msg(c.RoleAssistant, "a.ts looks fine; checking b.ts"),
		call("tool-2", "Read", `{"file_path":"src/b.ts"}`, fileB),
		result("tool-2", fileB, false),
		call("tool-3", "Bash", `{"command":"npm test"}`, "FAIL b.test.ts"),
		result("tool-3", "FAIL b.test.ts: expected 2 to be 3", true),
		msg(c.RoleAssistant, "The failure is in b.test.ts; fixing now."),
		msg(c.RoleUser, "go ahead"),
	}
}

// fit mirrors the original tests' fit options (huge state budget, nothing preserved).
func fit() c.ResolvedOptions {
	o := c.ResolveOptions(c.Options{MaxStateTokens: f(25000), PreserveRecentMessages: f(0), Goal: "fix the test"})
	return o
}
