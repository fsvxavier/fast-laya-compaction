package compaction

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// WindowedStateNote is appended to the state context in the windowed stage:
// the original text says `history` is the whole conversation, which is false
// here, and the model could read a missing later call as "never happened".
const WindowedStateNote = " In this request `history` is an excerpt: it always contains the calls being asked about and the later calls that touch the same files or commands; other messages may be missing, so do not treat an absent message as never having happened."

// Window is a group of candidate calls that share one state.
type Window struct {
	Calls  []ToolCall
	State  State
	Tokens int
}

// resourceKeys are the input fields that identify what a call touches.
var resourceKeys = []string{"file_path", "path", "notebook_path", "pattern", "url", "command"}

// resources lists "key=value" for the resource fields present in a call input.
func resources(input json.RawMessage) []string {
	var out []string
	for _, e := range orderedEntries(input) {
		if !slices.Contains(resourceKeys, e.Key) || len(e.Value) == 0 || e.Value[0] != '"' {
			continue
		}
		var v string
		if json.Unmarshal(e.Value, &v) == nil && v != "" {
			out = append(out, e.Key+"="+v)
		}
	}
	return out
}

func sharesResource(a, b []string) bool {
	for _, x := range a {
		if slices.Contains(b, x) {
			return true
		}
	}
	return false
}

// windowBuilder holds what BuildWindows needs to assemble one window's state.
type windowBuilder struct {
	messages []Message
	calls    []ToolCall
	opts     ResolvedOptions
	est      estimator
	goal     string
}

// windowEntries accumulates history entries by source message with a running token count.
type windowEntries struct {
	b       *windowBuilder
	budget  int
	base    int
	entries map[int]*HistoryEntry
	tokens  map[int]int
	total   int
}

func (b *windowBuilder) entryTokens(e HistoryEntry) int { return b.est.tokens(marshal(e)) + 1 }

func (w *windowEntries) set(e HistoryEntry) {
	if old, ok := w.tokens[e.I]; ok {
		w.total -= old
	}
	w.entries[e.I] = &e
	w.tokens[e.I] = w.b.entryTokens(e)
	w.total += w.tokens[e.I]
}

// tryAddLine adds a one-line call under its message, if it fits.
func (w *windowEntries) tryAddLine(c ToolCall) bool {
	cur, ok := w.entries[c.CallIndex]
	var next HistoryEntry
	if ok {
		next = *cur
		next.Lines = append(slices.Clone(cur.Lines), compactCall(c))
	} else {
		m := w.b.messages[c.CallIndex]
		next = HistoryEntry{I: c.CallIndex, Role: m.Role, Lines: []string{compactCall(c)}}
	}
	delta := w.b.entryTokens(next) - w.tokens[c.CallIndex]
	if w.base+w.total+delta > w.budget {
		return false
	}
	w.set(next)
	return true
}

// tryAddMessage adds a message's abridged text (and its calls as lines), if it fits.
func (w *windowEntries) tryAddMessage(i int, head, tail int) bool {
	if _, ok := w.entries[i]; ok {
		return false
	}
	m := w.messages()[i]
	if strings.TrimSpace(m.Text) == "" {
		return false
	}
	e := HistoryEntry{I: i, Role: m.Role, Text: abridge(m.Text, head, tail)}
	if w.base+w.total+w.b.entryTokens(e) > w.budget {
		return false
	}
	w.set(e)
	return true
}

func (w *windowEntries) messages() []Message { return w.b.messages }

func (w *windowEntries) history() []HistoryEntry {
	idx := make([]int, 0, len(w.entries))
	for i := range w.entries {
		idx = append(idx, i)
	}
	slices.Sort(idx)
	out := make([]HistoryEntry, 0, len(idx))
	for _, i := range idx {
		out = append(out, *w.entries[i])
	}
	return out
}

func (b *windowBuilder) context() string { return StateContext + WindowedStateNote }

func (b *windowBuilder) stateOf(goal string, h []HistoryEntry) State {
	if h == nil {
		h = []HistoryEntry{}
	}
	return State{Context: b.context(), Goal: goal, History: h}
}

// questionSeqTokens is the largest single-question cost in a window: the
// model reads one sequence per question, so questions do not add up.
func questionSeqTokens(window []ToolCall, opts ResolvedOptions) int {
	best := 0
	est := estimator{scale: opts.TokenScale}
	for _, c := range window {
		for _, q := range QuestionsFor(c) {
			best = max(best, est.tokens(q.Instructions)+opts.QuestionOverheadTokens)
		}
	}
	return best
}

// build assembles one window's state within budget (estimated tokens), or
// reports that even the required part does not fit.
func (b *windowBuilder) build(window []ToolCall, budget int, inputLimit int, collapseText bool) (Window, bool) {
	// goal: at most a quarter of the budget
	goal := b.goal
	for runeLen(goal) > 20 && b.est.tokens(goal) > budget/4 {
		goal = truncate(goal, runeLen(goal)*3/4)
	}
	base := b.est.tokens(marshal(b.stateOf(goal, nil)))
	w := &windowEntries{b: b, budget: budget, base: base, entries: map[int]*HistoryEntry{}, tokens: map[int]int{}}

	// required: the window's own calls, in their messages
	inWindow := map[string]bool{}
	for _, c := range window {
		inWindow[c.ID] = true
	}
	byMsg := callsByMessage(b.calls)
	var msgIdx []int
	for _, c := range window {
		if !slices.Contains(msgIdx, c.CallIndex) {
			msgIdx = append(msgIdx, c.CallIndex)
		}
	}
	for _, i := range msgIdx {
		m := b.messages[i]
		text := abridge(m.Text, 200, 100)
		if collapseText && runeLen(m.Text) > 0 {
			text = fmt.Sprintf("[… %d chars omitted …]", runeLen(m.Text))
		}
		e := HistoryEntry{I: i, Role: m.Role, Text: text}
		for _, c := range byMsg[i] {
			if inWindow[c.ID] {
				e.Calls = append(e.Calls, HistoryToolCall{ID: c.ID, Tool: c.Tool, Input: inputText(c.Input, inputLimit), Result: resultNote(c)})
			} else {
				e.Lines = append(e.Lines, compactCall(c))
			}
		}
		w.set(e)
	}
	if base+w.total > budget {
		return Window{}, false
	}

	// optional, by priority
	last := window[len(window)-1].CallIndex
	var mine []string
	for _, c := range window {
		mine = append(mine, resources(c.Input)...)
	}
	var later, earlier []ToolCall
	for _, c := range b.calls {
		switch {
		case inWindow[c.ID]:
		case c.CallIndex > last:
			later = append(later, c)
		default:
			earlier = append(earlier, c)
		}
	}
	// 1. later calls that touch the same resource
	for _, c := range later {
		if sharesResource(mine, resources(c.Input)) {
			w.tryAddLine(c)
		}
	}
	// 2. pinned messages: the first one and the newest ones
	for i := range b.messages {
		if isPinned(i, len(b.messages), b.opts.PreserveRecentMessages) {
			w.tryAddMessage(i, textHead, textTail)
		}
	}
	// 3. other later calls, nearest first; 4. earlier calls, nearest first
	for _, c := range later {
		w.tryAddLine(c)
	}
	for j := len(earlier) - 1; j >= 0; j-- {
		w.tryAddLine(earlier[j])
	}

	hist := w.history()
	return Window{Calls: window, State: b.stateOf(goal, hist), Tokens: base + w.total}, true
}

// BuildWindows groups the candidate calls into windows of WindowCalls and
// builds a state for each within the token budget. A window whose required
// part does not fit is retried with shorter inputs and collapsed text, then
// split into single-call windows; a call that still does not fit is returned
// as unfit and is kept without being asked about.
func BuildWindows(messages []Message, calls, candidates []ToolCall, opts ResolvedOptions, shrink float64) (windows []Window, unfit []ToolCall) {
	b := &windowBuilder{messages: messages, calls: calls, opts: opts, est: estimator{scale: opts.TokenScale}}
	b.goal = opts.Goal
	if b.goal == "" {
		b.goal = GoalFromMessages(messages)
	}
	if shrink <= 0 || shrink > 1 {
		shrink = 1
	}
	var place func(group []ToolCall)
	place = func(group []ToolCall) {
		budget := min(opts.MaxStateTokens, opts.MaxRequestTokens-requestOverheadTokens-questionSeqTokens(group, opts))
		budget = int(float64(budget) * shrink)
		for _, attempt := range []struct {
			input    int
			collapse bool
		}{{inputChars[1], false}, {inputChars[2], false}, {inputChars[2], true}} {
			if w, ok := b.build(group, budget, attempt.input, attempt.collapse); ok {
				windows = append(windows, w)
				return
			}
		}
		if len(group) > 1 {
			for _, c := range group {
				place([]ToolCall{c})
			}
			return
		}
		unfit = append(unfit, group[0])
	}
	for i := 0; i < len(candidates); i += opts.WindowCalls {
		place(candidates[i:min(i+opts.WindowCalls, len(candidates))])
	}
	return windows, unfit
}
