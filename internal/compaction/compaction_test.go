package compaction_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
	"testing"

	c "github.com/fsvxavier/fast-laya-compaction/internal/compaction"
	"github.com/fsvxavier/fast-laya-compaction/internal/compaction/fakelaya"
)

func TestResolveOptions(t *testing.T) {
	o := c.ResolveOptions(c.Options{})
	if o.KeepThreshold != 0.5 || o.PreserveRecentMessages != 6 || o.MaxStateTokens != 800 ||
		o.MaxRequestTokens != 960 || o.TruncateHeadChars != 300 || o.WindowCalls != 2 {
		t.Fatalf("defaults: %+v", o)
	}
	o = c.ResolveOptions(c.Options{KeepThreshold: f(math.NaN()), PreserveRecentMessages: f(2.7), TruncateHeadChars: f(-1.2)})
	if o.KeepThreshold != 0.5 || o.PreserveRecentMessages != 2 || o.TruncateHeadChars != 0 {
		t.Fatalf("non-finite/rounding: %+v", o)
	}
	// budgets follow the model limit unless explicit
	o = c.ResolveOptions(c.Options{InputTokenLimit: f(2048)})
	if o.MaxRequestTokens != 1984 || o.MaxStateTokens != 1653 {
		t.Fatalf("derived budgets: %+v", o)
	}
	o = c.ResolveOptions(c.Options{MaxRequestTokens: f(30000), MaxStateTokens: f(25000)})
	if o.MaxRequestTokens != 30000 || o.MaxStateTokens != 25000 {
		t.Fatalf("explicit budgets: %+v", o)
	}
}

func TestEstimateTokens(t *testing.T) {
	for in, want := range map[string]int{"": 0, "hello world": 2, "internationalization": 4, "12345678": 4} {
		if got := c.EstimateTokens(in); got != want {
			t.Errorf("EstimateTokens(%q) = %d, want %d", in, got, want)
		}
	}
	j := `{"file_path":"/Users/x/src/a.ts","old_string":"a = 1;","n":42}`
	if got := c.EstimateTokens(j); got < (len(j)+2)/3 {
		t.Errorf("JSON undercounted: %d for %d chars", got, len(j))
	}
}

func TestCollectToolCalls(t *testing.T) {
	calls := c.CollectToolCalls(transcript(), 3)
	type row struct {
		id, tool  string
		call, res int
		pinned    bool
	}
	var got []row
	for _, x := range calls {
		got = append(got, row{x.ID, x.Tool, x.CallIndex, x.ResultIndex, x.Pinned})
	}
	want := []row{{"t1", "Read", 1, 2, false}, {"t2", "Read", 4, 5, false}, {"t3", "Bash", 6, 7, true}}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	if !calls[2].IsError || calls[0].ResultChars != len(fileA) {
		t.Fatalf("flags: %+v", calls[0])
	}
}

func TestCollectIgnoresCallsWithoutResult(t *testing.T) {
	if n := len(c.CollectToolCalls([]c.Message{msg(c.RoleUser, "hi"), call("x", "Read", `{}`, "")}, 0)); n != 0 {
		t.Fatalf("got %d calls", n)
	}
}

func TestFitStateFullHistoryWithResultNotes(t *testing.T) {
	messages := transcript()
	fs, err := c.FitState(messages, c.CollectToolCalls(messages, 0), fit())
	if err != nil {
		t.Fatal(err)
	}
	if fs.Stage != "full" {
		t.Fatalf("stage %q", fs.Stage)
	}
	js, _ := json.Marshal(fs.State)
	s := string(js)
	if strings.Contains(s, "export const a = 1;") || !strings.Contains(s, "Never edit anything under src/generated") || !strings.Contains(s, "go ahead") {
		t.Fatalf("state content: %s", s)
	}
	var is []int
	for _, e := range fs.State.History {
		is = append(is, e.I)
	}
	if fmt.Sprint(is) != "[0 1 3 4 6 8 9]" {
		t.Fatalf("history indexes %v", is)
	}
	h1 := fs.State.History[1].Calls[0]
	if h1.ID != "t1" || h1.Tool != "Read" || h1.Result != fmt.Sprintf("ok, %d chars (omitted)", len(fileA)) {
		t.Fatalf("%+v", h1)
	}
	if !strings.HasPrefix(fs.State.History[4].Calls[0].Result, "error, ") {
		t.Fatalf("%+v", fs.State.History[4].Calls[0])
	}
}

func TestFitStateDefaultsGoalToLatestPrompts(t *testing.T) {
	o := fit()
	o.Goal = ""
	fs, _ := c.FitState(transcript(), nil, o)
	if !strings.Contains(fs.State.Goal, "Fix the failing test") || !strings.Contains(fs.State.Goal, "go ahead") {
		t.Fatalf("goal %q", fs.State.Goal)
	}
}

func TestFitStateTruncatesInputsBeforeText(t *testing.T) {
	messages := []c.Message{
		msg(c.RoleUser, "start"),
		call("w", "Write", fmt.Sprintf(`{"file_path":"x.ts","content":%q}`, strings.Repeat("x", 5000)), "ok"),
		result("w", "ok", false),
		msg(c.RoleAssistant, "written"),
	}
	o := fit()
	o.MaxStateTokens = 300
	fs, err := c.FitState(messages, c.CollectToolCalls(messages, 0), o)
	if err != nil {
		t.Fatal(err)
	}
	if fs.Stage != "inputs<=200" || fs.Tokens > 300 {
		t.Fatalf("stage %q tokens %d", fs.Stage, fs.Tokens)
	}
	if fs.State.History[0].Text != "start" || len([]rune(fs.State.History[1].Calls[0].Input)) > 200 {
		t.Fatalf("%+v", fs.State.History)
	}
}

func manyReads() []c.Message {
	messages := []c.Message{msg(c.RoleUser, "start")}
	for i := 0; i < 40; i++ {
		id := fmt.Sprintf("c%d", i)
		messages = append(messages, call(id, "Read", fmt.Sprintf(`{"file_path":"/repo/src/module-%d.ts"}`, i), "x"), result(id, "x", false))
	}
	return append(messages, msg(c.RoleAssistant, "done"))
}

func TestFitStateShrinksOldCallsToOneLineThenMerges(t *testing.T) {
	messages := manyReads()
	calls := c.CollectToolCalls(messages, 1)
	o := fit()
	o.PreserveRecentMessages = 1
	full, _ := c.FitState(messages, calls, o)

	o.MaxStateTokens = int(math.Floor(float64(full.Tokens) * 0.8))
	compacted, err := c.FitState(messages, calls, o)
	if err != nil {
		t.Fatal(err)
	}
	if compacted.Stage != "old calls compacted" || compacted.Tokens > o.MaxStateTokens {
		t.Fatalf("stage %q tokens %d", compacted.Stage, compacted.Tokens)
	}
	if got := compacted.State.History[1].Lines[0]; got != "t1 Read file_path=/repo/src/module-0.ts → ok 1ch" {
		t.Fatalf("line %q", got)
	}
	if last := compacted.State.History[len(compacted.State.History)-1]; last.Text != "done" {
		t.Fatalf("last %+v", last)
	}

	o.MaxStateTokens = int(math.Floor(float64(full.Tokens) * 0.45))
	merged, err := c.FitState(messages, calls, o)
	if err != nil {
		t.Fatal(err)
	}
	h := merged.State.History
	if merged.Stage != "old calls merged" || merged.Tokens > o.MaxStateTokens || len(h) != 3 || len(h[1].Lines) != 40 ||
		!strings.HasPrefix(h[1].Lines[39], "t40 Read ") || h[0].Text != "start" || h[2].Text != "done" {
		t.Fatalf("stage %q tokens %d len %d", merged.Stage, merged.Tokens, len(h))
	}
}

func TestFitStateAbridgesThenCollapses(t *testing.T) {
	long := func(n int) string { return fmt.Sprintf("%d ", n) + strings.Repeat("lorem ipsum ", 300) }
	messages := []c.Message{
		msg(c.RoleUser, long(0)), msg(c.RoleAssistant, long(1)), msg(c.RoleUser, long(2)),
		msg(c.RoleAssistant, long(3)), msg(c.RoleUser, "latest"),
	}
	o := fit()
	o.PreserveRecentMessages = 1
	o.MaxStateTokens = 1800
	ab, err := c.FitState(messages, nil, o)
	if err != nil {
		t.Fatal(err)
	}
	if ab.Stage != "texts abridged" || ab.Tokens > 1800 || !strings.Contains(ab.State.History[1].Text, "chars omitted") ||
		ab.State.History[0].Text != long(0) || ab.State.History[4].Text != "latest" {
		t.Fatalf("abridged: stage %q tokens %d", ab.Stage, ab.Tokens)
	}
	o.MaxStateTokens = 420
	co, err := c.FitState(messages, nil, o)
	if err != nil {
		t.Fatal(err)
	}
	if co.Stage != "old messages collapsed" || co.Tokens > 420 ||
		!regexp.MustCompile(`^\[… \d+ chars omitted …\]$`).MatchString(co.State.History[1].Text) ||
		!strings.Contains(co.State.History[0].Text, "lorem") || co.State.History[4].Text != "latest" {
		t.Fatalf("collapsed: stage %q tokens %d", co.Stage, co.Tokens)
	}
}

func TestFitStateTooLarge(t *testing.T) {
	messages := []c.Message{msg(c.RoleUser, strings.Repeat("a", 2000)), msg(c.RoleAssistant, "b")}
	o := fit()
	o.MaxStateTokens = 50
	if _, err := c.FitState(messages, nil, o); !errors.Is(err, c.ErrStateTooLarge) {
		t.Fatalf("err %v", err)
	}
}

func tenCalls() []c.ToolCall {
	var calls []c.ToolCall
	for i := 0; i < 10; i++ {
		calls = append(calls, c.ToolCall{ID: fmt.Sprintf("t%d", i+1), ToolUseID: fmt.Sprintf("tool-%d", i+1), Tool: "Read",
			CallIndex: i*2 + 1, ResultIndex: i*2 + 2, ResultChars: 100})
	}
	return calls
}

func TestBatchCalls(t *testing.T) {
	o := c.ResolveOptions(c.Options{MaxRequestTokens: f(30000)})
	one, err := c.BatchCalls(tenCalls(), 1000, o)
	if err != nil || len(one) != 1 {
		t.Fatalf("one batch: %v %v", len(one), err)
	}
	many, err := c.BatchCalls(tenCalls(), 29600, o)
	if err != nil || len(many) < 2 {
		t.Fatalf("split: %v %v", len(many), err)
	}
	var ids []string
	for _, b := range many {
		for _, x := range b {
			ids = append(ids, x.ID)
		}
	}
	if len(ids) != 10 || ids[0] != "t1" || ids[9] != "t10" {
		t.Fatalf("order %v", ids)
	}
	if _, err := c.BatchCalls(tenCalls(), 29990, o); !errors.Is(err, c.ErrNoRoomForQuestions) {
		t.Fatalf("err %v", err)
	}
}

func TestDecideCall(t *testing.T) {
	o := c.ResolveOptions(c.Options{})
	call := c.ToolCall{ID: "t1", Tool: "Read"}
	act := func(k, r float64, pinned bool) c.Decision {
		x := call
		x.Pinned = pinned
		return c.DecideCall(x, c.CallAnswer{KeepCall: k, KeepResult: r}, o)
	}
	if act(0.9, 0.7, false).Action != c.ActionKeep || act(0.9, 0.2, false).Action != c.ActionDropResult || act(0.1, 0.2, false).Action != c.ActionDropCall {
		t.Fatal("thresholds")
	}
	if d := act(0, 0, true); d.Action != c.ActionKeep || d.Reason != c.ReasonPinned {
		t.Fatalf("%+v", d)
	}
}

func TestApplyDecisionsRemovesAndTruncates(t *testing.T) {
	messages := transcript()
	messages[4].ToolUses[0].Text = strings.Repeat("x", 2000)
	messages[5].ToolResults[0].Text = strings.Repeat("x", 2000)
	calls := c.CollectToolCalls(messages, 0)
	o := c.ResolveOptions(c.Options{})
	decisions := []c.Decision{
		c.DecideCall(calls[0], c.CallAnswer{KeepCall: 0.1, KeepResult: 0.1}, o),
		c.DecideCall(calls[1], c.CallAnswer{KeepCall: 0.9, KeepResult: 0.1}, o),
		c.DecideCall(calls[2], c.CallAnswer{KeepCall: 0.9, KeepResult: 0.9}, o),
	}
	kept := c.ApplyDecisions(messages, decisions, calls, 300)

	label := func(m c.OutMessage) string {
		switch {
		case m.Text != "":
			return m.Text
		case len(m.ToolUses) > 0:
			return m.ToolUses[0].ToolUseID
		}
		return m.ToolResults[0].ToolUseID
	}
	var got []string
	for _, m := range kept {
		got = append(got, label(m))
	}
	want := []string{"Never edit anything under src/generated. Fix the failing test.", "a.ts looks fine; checking b.ts",
		"tool-2", "tool-2", "tool-3", "tool-3", "The failure is in b.test.ts; fixing now.", "go ahead"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("got %q", got)
	}
	head := regexp.MustCompile("^" + strings.Repeat("x", 300) + `\n\[fast-laya-compaction truncated 1700 chars`)
	if !kept[0].Unchanged || kept[0].Source != 0 || kept[2].Unchanged || kept[2].Source != 4 || !kept[2].ToolUses[0].Changed ||
		!head.MatchString(kept[2].ToolUses[0].Text) || !head.MatchString(kept[3].ToolResults[0].Text) {
		t.Fatalf("truncation: %+v", kept[2])
	}
	if !kept[4].Unchanged || kept[4].Source != 6 || !strings.Contains(kept[5].ToolResults[0].Text, "expected 2 to be 3") {
		t.Fatalf("untouched: %+v", kept[4])
	}

	// short dropped results stay as they were
	short := transcript()
	short[4].ToolUses[0].Text = strings.Repeat("y", 100)
	short[5].ToolResults[0].Text = strings.Repeat("y", 100)
	sk := c.ApplyDecisions(short, decisions, calls, 300)
	if !sk[2].Unchanged || !sk[3].Unchanged {
		t.Fatalf("short results must be unchanged: %+v %+v", sk[2], sk[3])
	}
}

func TestApplyDecisionsHonoursHeadChars(t *testing.T) {
	messages := transcript()
	calls := c.CollectToolCalls(messages, 0)
	o := c.ResolveOptions(c.Options{})
	decisions := []c.Decision{c.DecideCall(calls[0], c.CallAnswer{KeepCall: 0.9, KeepResult: 0.1}, o)}
	original := messages[2].ToolResults[0].Text
	total := len(original)

	kept := c.ApplyDecisions(messages, decisions, calls, 50)
	wantText := fmt.Sprintf("%s\n[fast-laya-compaction truncated %d chars of this tool result; re-run the tool if needed]", original[:50], total-50)
	if kept[2].ToolResults[0].Text != wantText || kept[1].ToolUses[0].Text != wantText {
		t.Fatalf("head 50: %q", kept[2].ToolResults[0].Text)
	}
	noHead := c.ApplyDecisions(messages, decisions, calls, 0)
	if want := fmt.Sprintf("[fast-laya-compaction truncated %d chars of this tool result; re-run the tool if needed]", total); noHead[2].ToolResults[0].Text != want {
		t.Fatalf("head 0: %q", noHead[2].ToolResults[0].Text)
	}
}

func TestCompactResendsFullStateWithEveryBatch(t *testing.T) {
	messages := transcript()
	o := fit()
	o.Goal, o.PreserveRecentMessages = "", 1
	stateTokens := mustFit(t, messages, c.CollectToolCalls(messages, 1), o).Tokens
	fake := &fakelaya.Asker{Answer: func(id string) float64 {
		if strings.HasPrefix(id, "call_") {
			return 0.9
		}
		return 0.1
	}}
	out, err := c.Compact(context.Background(), messages, fake, c.Options{PreserveRecentMessages: f(1), MaxRequestTokens: f(float64(stateTokens + 150))})
	if err != nil {
		t.Fatal(err)
	}
	if out.Stats.Requests != len(fake.Seen) || len(fake.Seen) < 2 {
		t.Fatalf("requests %d seen %d", out.Stats.Requests, len(fake.Seen))
	}
	states := map[string]bool{}
	var ids []string
	for _, r := range fake.Seen {
		b, _ := json.Marshal(r.State)
		states[string(b)] = true
		ids = append(ids, r.QuestionIDs...)
	}
	if len(states) != 1 {
		t.Fatalf("%d distinct states", len(states))
	}
	if fmt.Sprint(sorted(ids)) != "[call_t1 call_t2 call_t3 result_t1 result_t2 result_t3]" {
		t.Fatalf("ids %v", sorted(ids))
	}
	for _, d := range out.Decisions {
		if d.Action != c.ActionDropResult {
			t.Fatalf("%+v", d)
		}
	}
	s := out.Stats
	if len(out.Messages) != len(messages) || s.ResultsDropped != 3 || s.Kept != 0 || s.CallsDropped != 0 || s.Pinned != 0 || s.ReductionRatio <= 0 {
		t.Fatalf("stats %+v", s)
	}
}

func TestCompactWithoutCandidatesNeverAsks(t *testing.T) {
	fake := &fakelaya.Asker{Answer: func(string) float64 { return 0 }}
	out, err := c.Compact(context.Background(), []c.Message{msg(c.RoleUser, "hello"), msg(c.RoleAssistant, "hi")}, fake, c.Options{})
	if err != nil || len(fake.Seen) != 0 || out.Stats.Requests != 0 || out.Stats.StateStage != "" || out.Stats.Calls != 0 || len(out.Messages) != 2 {
		t.Fatalf("%v %+v", err, out.Stats)
	}
}

func TestCompactAllKeptReducesNothing(t *testing.T) {
	fake := &fakelaya.Asker{Answer: func(string) float64 { return 0.95 }}
	out, err := c.Compact(context.Background(), transcript(), fake, c.Options{PreserveRecentMessages: f(1)})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range out.Decisions {
		if d.Action != c.ActionKeep {
			t.Fatalf("%+v", d)
		}
	}
	if out.Stats.ReductionRatio != 0 {
		t.Fatalf("ratio %v", out.Stats.ReductionRatio)
	}
}

func TestCompactRejectsMalformedAnswers(t *testing.T) {
	for name, probs := range map[string]map[string]float64{
		"missing":      {"call_t1": 0.5},
		"out of range": {"call_t1": 0.5, "result_t1": 1.5, "call_t2": 0.5, "result_t2": 0.5},
		"nan":          {"call_t1": math.NaN(), "result_t1": 0.5, "call_t2": 0.5, "result_t2": 0.5},
	} {
		t.Run(name, func(t *testing.T) {
			fake := &fakelaya.Asker{Override: func([]c.Question) c.Answers { return c.Answers{Probabilities: probs} }}
			_, err := c.Compact(context.Background(), transcript(), fake, c.Options{PreserveRecentMessages: f(1)})
			if !errors.Is(err, c.ErrMalformedAnswer) {
				t.Fatalf("err %v", err)
			}
		})
	}
}

func TestCompactPropagatesAskerErrorAndCancellation(t *testing.T) {
	boom := errors.New("boom")
	fake := &fakelaya.Asker{Err: boom}
	if _, err := c.Compact(context.Background(), transcript(), fake, c.Options{PreserveRecentMessages: f(1)}); !errors.Is(err, boom) {
		t.Fatalf("err %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ok := &fakelaya.Asker{Answer: func(string) float64 { return 0.5 }}
	if _, err := c.Compact(ctx, transcript(), ok, c.Options{PreserveRecentMessages: f(1)}); !errors.Is(err, context.Canceled) {
		t.Fatalf("err %v", err)
	}
}

func mustFit(t *testing.T, m []c.Message, calls []c.ToolCall, o c.ResolvedOptions) c.FittedState {
	t.Helper()
	fs, err := c.FitState(m, calls, o)
	if err != nil {
		t.Fatal(err)
	}
	return fs
}

func sorted(in []string) []string {
	out := append([]string(nil), in...)
	for i := range out {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}
