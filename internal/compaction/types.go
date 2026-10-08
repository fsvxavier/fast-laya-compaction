// Package compaction ports the fast-jev-compaction algorithm: it drops tool
// calls and tool results a decision model judges no longer needed, and never
// rewrites user or assistant text.
package compaction

import "encoding/json"

type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// ToolUse is a tool_use block of an assistant message. Text and IsError mirror
// the outcome once the transcript holds it (Claude Code attaches them).
type ToolUse struct {
	ToolUseID string          `json:"tool_use_id"`
	Tool      string          `json:"tool"`
	Input     json.RawMessage `json:"input"`
	Text      string          `json:"text,omitempty"`
	IsError   bool            `json:"isError,omitempty"`
}

// ToolResult is a tool_result block of a user message.
type ToolResult struct {
	ToolUseID string `json:"tool_use_id"`
	Text      string `json:"text"`
	IsError   bool   `json:"isError,omitempty"`
}

// Message is one transcript message; a subset of Claude Code's SessionMessage.
type Message struct {
	Role        Role         `json:"role"`
	Text        string       `json:"text"`
	ToolUses    []ToolUse    `json:"toolUses"`
	ToolResults []ToolResult `json:"toolResults,omitempty"`
}

// ToolCall is a tool call paired with its result by tool_use_id.
type ToolCall struct {
	// ID is the short id used in the state and question names (t1, t2, ...).
	ID          string
	ToolUseID   string
	Tool        string
	Input       json.RawMessage
	CallIndex   int
	ResultIndex int
	ResultChars int
	IsError     bool
	// Pinned calls sit in the first or newest preserved messages; never candidates.
	Pinned bool
}

type Action string

const (
	ActionKeep       Action = "keep"
	ActionDropResult Action = "drop_result"
	ActionDropCall   Action = "drop_call"
)

type Reason string

const (
	ReasonPinned        Reason = "pinned"
	ReasonKept          Reason = "kept"
	ReasonResultDropped Reason = "result_dropped"
	ReasonCallDropped   Reason = "call_dropped"
	ReasonUnfit         Reason = "unfit"
)

// CallAnswer holds the model's probabilities for one call.
type CallAnswer struct {
	// KeepCall is the probability that the call itself still matters.
	KeepCall float64 `json:"keepCall"`
	// KeepResult is the probability that the full result must stay verbatim.
	KeepResult float64 `json:"keepResult"`
}

type Decision struct {
	ID   string `json:"id"`
	Tool string `json:"tool"`
	CallAnswer
	Action Action `json:"action"`
	Reason Reason `json:"reason"`
}

// OutToolUse is a kept tool_use block with its position in the source message.
type OutToolUse struct {
	SourceIndex int             `json:"sourceIndex"`
	Changed     bool            `json:"changed,omitempty"`
	ToolUseID   string          `json:"tool_use_id"`
	Tool        string          `json:"tool"`
	Input       json.RawMessage `json:"input"`
	Text        string          `json:"text,omitempty"`
	IsError     bool            `json:"isError,omitempty"`
}

// OutToolResult is a kept tool_result block with its position in the source message.
type OutToolResult struct {
	SourceIndex int    `json:"sourceIndex"`
	Changed     bool   `json:"changed,omitempty"`
	ToolUseID   string `json:"tool_use_id"`
	Text        string `json:"text"`
	IsError     bool   `json:"isError,omitempty"`
}

// OutMessage is a message of the compacted transcript. Source is the index of
// the input message it came from; Unchanged means it is identical to it.
type OutMessage struct {
	Source      int             `json:"source"`
	Unchanged   bool            `json:"unchanged"`
	Role        Role            `json:"role"`
	Text        string          `json:"text"`
	ToolUses    []OutToolUse    `json:"toolUses"`
	ToolResults []OutToolResult `json:"toolResults"`
}

type Stats struct {
	MessagesBefore int `json:"messagesBefore"`
	MessagesAfter  int `json:"messagesAfter"`
	CharsBefore    int `json:"charsBefore"`
	CharsAfter     int `json:"charsAfter"`
	Calls          int `json:"calls"`
	Kept           int `json:"kept"`
	ResultsDropped int `json:"resultsDropped"`
	CallsDropped   int `json:"callsDropped"`
	Pinned         int `json:"pinned"`
	Unfit          int `json:"unfit"`
	StateTokens    int `json:"stateTokens"`
	// StateStage is the fitting stage the state needed; empty when no request was made.
	StateStage           string  `json:"stateStage"`
	Requests             int     `json:"requests"`
	Windows              int     `json:"windows"`
	Retries              int     `json:"retries"`
	InputTokensReported  int     `json:"inputTokensReported"`
	InputTokensEstimated int     `json:"inputTokensEstimated"`
	ReductionRatio       float64 `json:"reductionRatio"`
	Ms                   int64   `json:"ms"`
}

type Result struct {
	Messages  []OutMessage `json:"messages"`
	Decisions []Decision   `json:"decisions"`
	Stats     Stats        `json:"stats"`
}
