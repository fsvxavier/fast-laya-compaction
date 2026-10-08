package compaction_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"testing"

	c "github.com/fsvxavier/fast-laya-compaction/internal/compaction"
	"github.com/fsvxavier/fast-laya-compaction/internal/compaction/fakelaya"
)

// longSession has n reads of distinct files, a late edit of module-0 and a closing message.
func longSession(n int) []c.Message {
	msgs := []c.Message{msg(c.RoleUser, "Refactor the module loader and keep tests green.")}
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("c%d", i)
		msgs = append(msgs, call(id, "Read", fmt.Sprintf(`{"file_path":"/repo/src/module-%d.ts"}`, i), "x"), result(id, "x", false))
		if i%7 == 3 {
			msgs = append(msgs, msg(c.RoleAssistant, fmt.Sprintf("Looking at module %d next; it wires the loader to the cache.", i)))
		}
	}
	msgs = append(msgs, call("edit0", "Edit", `{"file_path":"/repo/src/module-0.ts","old_string":"a","new_string":"b"}`, "ok"), result("edit0", "ok", false))
	return append(msgs, msg(c.RoleAssistant, "done"))
}

func candidatesOf(calls []c.ToolCall) []c.ToolCall {
	var out []c.ToolCall
	for _, x := range calls {
		if !x.Pinned {
			out = append(out, x)
		}
	}
	return out
}

func TestBuildWindowsRespectsBudgetAndOrder(t *testing.T) {
	messages := longSession(60)
	opts := c.ResolveOptions(c.Options{})
	calls := c.CollectToolCalls(messages, opts.PreserveRecentMessages)
	cands := candidatesOf(calls)
	if _, err := c.FitState(messages, calls, opts); !errors.Is(err, c.ErrStateTooLarge) {
		t.Fatalf("scenario must overflow the global state, got %v", err)
	}
	windows, unfit := c.BuildWindows(messages, calls, cands, opts, 1)
	if len(unfit) != 0 || len(windows) == 0 {
		t.Fatalf("windows=%d unfit=%d", len(windows), len(unfit))
	}
	asked := 0
	for _, w := range windows {
		asked += len(w.Calls)
		if w.Tokens > opts.MaxStateTokens {
			t.Errorf("window over budget: %d > %d", w.Tokens, opts.MaxStateTokens)
		}
		prev := -1
		structured := map[string]bool{}
		for _, e := range w.State.History {
			if e.I <= prev {
				t.Errorf("history not ordered by i: %d after %d", e.I, prev)
			}
			prev = e.I
			for _, tc := range e.Calls {
				structured[tc.ID] = true
			}
		}
		for _, call := range w.Calls {
			if !structured[call.ID] {
				t.Errorf("asked call %s missing from its window state", call.ID)
			}
		}
		if !strings.Contains(w.State.Context, "this request `history` is an excerpt") {
			t.Error("windowed context must say history is an excerpt")
		}
	}
	if asked != len(cands) {
		t.Errorf("asked %d of %d candidates", asked, len(cands))
	}
}

func TestBuildWindowsIncludesLaterCallOnSameResource(t *testing.T) {
	messages := longSession(60)
	opts := c.ResolveOptions(c.Options{})
	calls := c.CollectToolCalls(messages, opts.PreserveRecentMessages)
	windows, _ := c.BuildWindows(messages, calls, candidatesOf(calls)[:2], opts, 1) // c0, c1
	if len(windows) != 1 {
		t.Fatalf("windows %d", len(windows))
	}
	js := string(c.MarshalState(windows[0].State))
	if !strings.Contains(js, "Edit file_path=/repo/src/module-0.ts") {
		t.Fatalf("the later Edit of module-0 must be in the state of the call that read it:\n%.600s", js)
	}
}

func TestBuildWindowsShrinksWithRetryFactor(t *testing.T) {
	messages := longSession(60)
	opts := c.ResolveOptions(c.Options{})
	calls := c.CollectToolCalls(messages, opts.PreserveRecentMessages)
	cands := candidatesOf(calls)[:2]
	full, _ := c.BuildWindows(messages, calls, cands, opts, 1)
	small, _ := c.BuildWindows(messages, calls, cands, opts, 0.5)
	if small[0].Tokens >= full[0].Tokens || small[0].Tokens > opts.MaxStateTokens/2 {
		t.Fatalf("shrink: full %d small %d", full[0].Tokens, small[0].Tokens)
	}
}

func TestBuildWindowsCallThatCannotFitIsUnfit(t *testing.T) {
	messages := longSession(10)
	opts := c.ResolveOptions(c.Options{MaxStateTokens: f(40)})
	calls := c.CollectToolCalls(messages, opts.PreserveRecentMessages)
	windows, unfit := c.BuildWindows(messages, calls, candidatesOf(calls), opts, 1)
	if len(windows) != 0 || len(unfit) != len(candidatesOf(calls)) {
		t.Fatalf("windows=%d unfit=%d", len(windows), len(unfit))
	}
}

func TestCompactUsesWindowedStageWhenGlobalStateOverflows(t *testing.T) {
	messages := longSession(60)
	fake := &fakelaya.Asker{Answer: func(id string) float64 {
		if strings.HasSuffix(id, "_t1") { // c0 read, superseded by the late edit
			return 0.1
		}
		return 0.9
	}}
	out, err := c.Compact(context.Background(), messages, fake, c.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s := out.Stats
	if s.StateStage != "windowed" || s.Windows == 0 || s.Requests != s.Windows || s.Unfit != 0 || s.StateTokens > 800 {
		t.Fatalf("stats %+v", s)
	}
	if out.Decisions[0].Action != c.ActionDropCall {
		t.Fatalf("t1 should be dropped: %+v", out.Decisions[0])
	}
}

func TestCompactRetriesWhenModelRejectsInput(t *testing.T) {
	messages := longSession(60)
	rejected := 0
	fake := &fakelaya.Asker{Answer: func(string) float64 { return 0.9 }, Fail: func(n int) error {
		if n == 0 {
			rejected++
			return &c.InputTooLongError{Observed: 1100, Limit: 1024}
		}
		return nil
	}}
	out, err := c.Compact(context.Background(), messages, fake, c.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if rejected != 1 || out.Stats.Retries != 1 || out.Stats.Unfit != 0 {
		t.Fatalf("rejected %d stats %+v", rejected, out.Stats)
	}
	// the retry must use a smaller state than the rejected one
	if fake.Seen[1].State.Goal == "" || len(fake.Seen) < 2 {
		t.Fatal("expected a retry request")
	}
}

func TestCompactKeepsCallsTheModelAlwaysRejects(t *testing.T) {
	messages := longSession(20)
	fake := &fakelaya.Asker{Answer: func(string) float64 { return 0 }, Fail: func(int) error {
		return &c.InputTooLongError{Observed: 2000, Limit: 1024}
	}}
	out, err := c.Compact(context.Background(), messages, fake, c.Options{MaxStateTokens: f(60)})
	if err != nil {
		t.Fatal(err)
	}
	if out.Stats.Unfit == 0 || out.Stats.ResultsDropped+out.Stats.CallsDropped != 0 {
		t.Fatalf("stats %+v", out.Stats)
	}
	for _, d := range out.Decisions {
		if d.Reason != c.ReasonUnfit && d.Reason != c.ReasonPinned {
			t.Fatalf("nothing may be dropped without an answer: %+v", d)
		}
	}
}

// randomTranscript builds a transcript with random shapes: orphans, errors,
// repeated resources, long texts, parallel calls.
func randomTranscript(r *rand.Rand) []c.Message {
	n := 3 + r.Intn(60)
	msgs := []c.Message{msg(c.RoleUser, "task: "+strings.Repeat("do the thing ", 1+r.Intn(30)))}
	for i := 0; i < n; i++ {
		switch r.Intn(5) {
		case 0:
			msgs = append(msgs, msg(c.RoleAssistant, strings.Repeat("thinking out loud ", r.Intn(80))))
		case 1:
			msgs = append(msgs, msg(c.RoleUser, "follow up "+fmt.Sprint(i)))
		default:
			id := fmt.Sprintf("u%d", i)
			path := fmt.Sprintf(`{"file_path":"/repo/f%d.go"}`, r.Intn(6))
			m := call(id, []string{"Read", "Edit", "Bash"}[r.Intn(3)], path, "")
			if r.Intn(6) == 0 { // parallel call in the same message
				m.ToolUses = append(m.ToolUses, c.ToolUse{ToolUseID: id + "b", Tool: "Grep", Input: json.RawMessage(`{"pattern":"x"}`)})
			}
			msgs = append(msgs, m)
			if r.Intn(8) != 0 { // sometimes no result (orphan)
				res := result(id, strings.Repeat("r", r.Intn(4000)), r.Intn(7) == 0)
				if len(m.ToolUses) > 1 {
					res.ToolResults = append(res.ToolResults, c.ToolResult{ToolUseID: id + "b", Text: "found"})
				}
				msgs = append(msgs, res)
			}
		}
	}
	return msgs
}

func checkInvariants(t *testing.T, in []c.Message, out c.Result) {
	t.Helper()
	uses, results := map[string]bool{}, map[string]bool{}
	prev := -1
	for _, m := range out.Messages {
		if m.Source <= prev {
			t.Fatalf("order not preserved: source %d after %d", m.Source, prev)
		}
		prev = m.Source
		src := in[m.Source]
		if m.Text != src.Text || m.Role != src.Role {
			t.Fatalf("user/assistant text must never change (message %d)", m.Source)
		}
		if m.Unchanged && (len(m.ToolUses) != len(src.ToolUses) || len(m.ToolResults) != len(src.ToolResults)) {
			t.Fatalf("unchanged message %d differs from its source", m.Source)
		}
		for _, u := range m.ToolUses {
			uses[u.ToolUseID] = true
			if u.SourceIndex >= len(src.ToolUses) || src.ToolUses[u.SourceIndex].ToolUseID != u.ToolUseID {
				t.Fatalf("bad sourceIndex on tool_use %s", u.ToolUseID)
			}
		}
		for _, r := range m.ToolResults {
			results[r.ToolUseID] = true
		}
	}
	// a kept tool_use whose result existed in the input must keep its result, and vice versa
	inResults := map[string]bool{}
	for _, m := range in {
		for _, r := range m.ToolResults {
			inResults[r.ToolUseID] = true
		}
	}
	for id := range uses {
		if inResults[id] && !results[id] {
			t.Fatalf("tool_use %s kept without its tool_result", id)
		}
	}
	for id := range results {
		if !uses[id] {
			t.Fatalf("tool_result %s kept without its tool_use", id)
		}
	}
	for _, d := range out.Decisions {
		if (d.Reason == c.ReasonPinned || d.Reason == c.ReasonUnfit) && d.Action != c.ActionKeep {
			t.Fatalf("pinned/unfit call changed: %+v", d)
		}
	}
}

func FuzzCompact(f *testing.F) {
	for _, seed := range []int64{1, 2, 3, 42, 1234} {
		f.Add(seed, uint8(seed%4), uint8(seed%7), uint16(60+seed%800))
	}
	f.Fuzz(func(t *testing.T, seed int64, windowCalls, preserve uint8, maxState uint16) {
		r := rand.New(rand.NewSource(seed))
		in := randomTranscript(r)
		fake := &fakelaya.Asker{Answer: func(string) float64 { return r.Float64() }, Fail: func(n int) error {
			if r.Intn(9) == 0 {
				return &c.InputTooLongError{Observed: 1100, Limit: 1024}
			}
			return nil
		}}
		out, err := c.Compact(context.Background(), in, fake, c.Options{
			WindowCalls: f64(float64(1 + int(windowCalls)%3)), PreserveRecentMessages: f64(float64(preserve % 7)),
			MaxStateTokens: f64(float64(40 + int(maxState)%900)),
		})
		if err != nil {
			if errors.Is(err, c.ErrStateTooLarge) || errors.Is(err, c.ErrNoRoomForQuestions) {
				t.Fatalf("overflow must fall into the windowed stage, got %v", err)
			}
			t.Fatal(err)
		}
		checkInvariants(t, in, out)
	})
}

func f64(v float64) *float64 { return &v }
