package compaction

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// StateContext is the fixed explanation sent with every state, verbatim from the original.
const StateContext = "A coding assistant conversation is being compacted to free context. `history` is the whole conversation so far, oldest first; tool outputs are replaced by a short `result` note and long texts may be abridged. Each question asks whether one tool call, or the full output of that call, still needs to stay in the history verbatim. Whatever is not kept is deleted permanently, but the assistant can always re-run a tool or re-read a file."

// inputChars are the successive caps on the serialised tool input per call.
var inputChars = [3]int{1000, 200, 60}

const (
	textHead = 400
	textTail = 150
)

// HistoryToolCall is a tool call in structured form.
type HistoryToolCall struct {
	ID     string `json:"id"`
	Tool   string `json:"tool"`
	Input  string `json:"input"`
	Result string `json:"result"`
}

// HistoryEntry is one message in the state. Its tool calls are either
// structured or, once the state has to shrink, one line per call.
type HistoryEntry struct {
	I     int
	Role  Role
	Text  string
	Calls []HistoryToolCall
	Lines []string
}

func (e HistoryEntry) hasCalls() bool { return len(e.Calls) > 0 || len(e.Lines) > 0 }

func (e HistoryEntry) MarshalJSON() ([]byte, error) {
	out := struct {
		I         int    `json:"i"`
		Role      Role   `json:"role"`
		Text      string `json:"text"`
		ToolCalls any    `json:"tool_calls,omitempty"`
	}{I: e.I, Role: e.Role, Text: e.Text}
	switch {
	case len(e.Lines) > 0 && len(e.Calls) > 0: // windowed state: structured calls first, then one-liners
		mixed := make([]any, 0, len(e.Calls)+len(e.Lines))
		for _, c := range e.Calls {
			mixed = append(mixed, c)
		}
		for _, l := range e.Lines {
			mixed = append(mixed, l)
		}
		out.ToolCalls = mixed
	case len(e.Lines) > 0:
		out.ToolCalls = e.Lines
	case len(e.Calls) > 0:
		out.ToolCalls = e.Calls
	}
	return []byte(marshal(out)), nil
}

// State is what the model is sent: the history, tool results replaced by notes.
type State struct {
	Context string         `json:"context"`
	Goal    string         `json:"goal"`
	History []HistoryEntry `json:"history"`
}

// FittedState is a State with its estimated size and the stage that produced it.
type FittedState struct {
	State  State
	Tokens int
	Stage  string
}

func resultNote(c ToolCall) string {
	status := "ok"
	if c.IsError {
		status = "error"
	}
	return fmt.Sprintf("%s, %d chars (omitted)", status, c.ResultChars)
}

func inputText(input json.RawMessage, limit int) string {
	s, ok := compactJSON(input)
	if !ok {
		s = "[unserializable input]"
	}
	return truncate(s, limit)
}

// compactCall renders one call as a single line.
func compactCall(c ToolCall) string {
	var parts []string
	for _, e := range orderedEntries(c.Input) {
		var text string
		if len(e.Value) > 0 && e.Value[0] == '"' {
			_ = json.Unmarshal(e.Value, &text)
		} else {
			v, _ := compactJSON(e.Value)
			text = truncate(fmt.Sprintf("{%s:%s}", marshal(e.Key), v), 200)
		}
		parts = append(parts, e.Key+"="+collapseWhitespace(text))
	}
	status := "ok"
	if c.IsError {
		status = "error"
	}
	return fmt.Sprintf("%s %s %s → %s %dch", c.ID, c.Tool, truncate(strings.Join(parts, " "), inputChars[2]), status, c.ResultChars)
}

func callsByMessage(calls []ToolCall) map[int][]ToolCall {
	by := map[int][]ToolCall{}
	for _, c := range calls {
		by[c.CallIndex] = append(by[c.CallIndex], c)
	}
	return by
}

func historyEntries(messages []Message, calls []ToolCall, inputLimit int) []HistoryEntry {
	by := callsByMessage(calls)
	var entries []HistoryEntry
	for i, m := range messages {
		var toolCalls []HistoryToolCall
		for _, c := range by[i] {
			toolCalls = append(toolCalls, HistoryToolCall{ID: c.ID, Tool: c.Tool, Input: inputText(c.Input, inputLimit), Result: resultNote(c)})
		}
		if strings.TrimSpace(m.Text) == "" && len(toolCalls) == 0 {
			continue
		}
		entries = append(entries, HistoryEntry{I: i, Role: m.Role, Text: m.Text, Calls: toolCalls})
	}
	return entries
}

// mergeCallRuns folds runs of adjacent call-only entries into one entry each.
func mergeCallRuns(history []HistoryEntry, pinned func(HistoryEntry) bool) []HistoryEntry {
	foldable := func(e HistoryEntry) bool { return !pinned(e) && e.Text == "" && len(e.Lines) > 0 }
	var merged []HistoryEntry
	for _, e := range history {
		if n := len(merged); n > 0 && foldable(merged[n-1]) && foldable(e) && merged[n-1].Role == e.Role {
			merged[n-1].Lines = append(slices.Clone(merged[n-1].Lines), e.Lines...)
			continue
		}
		merged = append(merged, e)
	}
	return merged
}

// FitState builds the state from the whole conversation and shrinks it in
// stages until it fits MaxStateTokens: tool inputs are truncated, then long
// texts abridged oldest-first (pinned messages last), then old messages
// collapsed to a one-line note, then old tool calls shrunk to one line each,
// then old messages without a call left out, then runs of old call-only
// messages folded. It returns ErrStateTooLarge when even that is too big.
func FitState(messages []Message, calls []ToolCall, opts ResolvedOptions) (FittedState, error) {
	est := estimator{scale: opts.TokenScale}
	goal := opts.Goal
	if goal == "" {
		goal = GoalFromMessages(messages)
	}
	stateOf := func(h []HistoryEntry) State {
		if h == nil {
			h = []HistoryEntry{}
		}
		return State{Context: StateContext, Goal: goal, History: h}
	}
	entryTokens := func(e HistoryEntry) int { return est.tokens(marshal(e)) + 1 }
	baseTokens := est.tokens(marshal(stateOf(nil)))
	fitted := func(h []HistoryEntry, tokens int, stage string) FittedState {
		return FittedState{State: stateOf(h), Tokens: tokens, Stage: stage}
	}

	var history []HistoryEntry
	var perEntry []int
	tokens := 0
	sum := func() int {
		total := baseTokens
		for _, n := range perEntry {
			total += n
		}
		return total
	}
	rebuild := func(limit int) {
		history = historyEntries(messages, calls, limit)
		perEntry = make([]int, len(history))
		for i, e := range history {
			perEntry[i] = entryTokens(e)
		}
		tokens = sum()
	}
	fits := func() bool { return tokens <= opts.MaxStateTokens }
	shrink := func(i int, change func(*HistoryEntry)) {
		change(&history[i])
		now := entryTokens(history[i])
		tokens += now - perEntry[i]
		perEntry[i] = now
	}

	rebuild(inputChars[0])
	if fits() {
		return fitted(history, tokens, "full"), nil
	}
	for _, limit := range inputChars[1:] {
		rebuild(limit)
		if fits() {
			return fitted(history, tokens, fmt.Sprintf("inputs<=%d", limit)), nil
		}
	}

	pinned := func(e HistoryEntry) bool { return isPinned(e.I, len(messages), opts.PreserveRecentMessages) }
	var order []int
	for i := range history {
		if !pinned(history[i]) {
			order = append(order, i)
		}
	}
	for i := range history {
		if pinned(history[i]) {
			order = append(order, i)
		}
	}

	for _, i := range order {
		if runeLen(history[i].Text) <= textHead+textTail+40 {
			continue
		}
		shrink(i, func(e *HistoryEntry) { e.Text = abridge(e.Text, textHead, textTail) })
		if fits() {
			return fitted(history, tokens, "texts abridged"), nil
		}
	}

	for _, i := range order {
		if pinned(history[i]) || history[i].Text == "" {
			continue
		}
		original := runeLen(history[i].Text)
		if src := history[i].I; src < len(messages) {
			original = runeLen(messages[src].Text)
		}
		shrink(i, func(e *HistoryEntry) { e.Text = fmt.Sprintf("[… %d chars omitted …]", original) })
		if fits() {
			return fitted(history, tokens, "old messages collapsed"), nil
		}
	}

	by := callsByMessage(calls)
	for _, i := range order {
		own, ok := by[history[i].I]
		if pinned(history[i]) || !ok {
			continue
		}
		shrink(i, func(e *HistoryEntry) {
			e.Calls = nil
			e.Lines = e.Lines[:0:0]
			for _, c := range own {
				e.Lines = append(e.Lines, compactCall(c))
			}
		})
		if fits() {
			return fitted(history, tokens, "old calls compacted"), nil
		}
	}

	left := map[int]bool{}
	for _, i := range order {
		if pinned(history[i]) || history[i].hasCalls() {
			continue
		}
		left[i] = true
		tokens -= perEntry[i]
		if fits() {
			return fitted(withoutIndexes(history, left), tokens, "old messages left out"), nil
		}
	}

	history = mergeCallRuns(withoutIndexes(history, left), pinned)
	perEntry = make([]int, len(history))
	for i, e := range history {
		perEntry[i] = entryTokens(e)
	}
	tokens = sum()
	if fits() {
		return fitted(history, tokens, "old calls merged"), nil
	}
	return FittedState{}, fmt.Errorf("%w (~%d tokens after truncation, limit %d)", ErrStateTooLarge, tokens, opts.MaxStateTokens)
}

func withoutIndexes(history []HistoryEntry, drop map[int]bool) []HistoryEntry {
	out := make([]HistoryEntry, 0, len(history))
	for i, e := range history {
		if !drop[i] {
			out = append(out, e)
		}
	}
	return out
}

// MarshalState serialises a State exactly as the model should see it: compact
// JSON without HTML escaping (json.Marshal would turn < > & into < etc.).
func MarshalState(s State) []byte { return []byte(marshal(s)) }
