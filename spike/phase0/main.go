// Command phase0 is the throwaway Phase 0 spike: load the exported bundle,
// run real noul predictions, and measure latency and the token limit.
//
//	CGO_ENABLED=1 go build -tags laya_native -o phase0 .
//	LAYA_ONNXRUNTIME_LIBRARY=... LAYA_TOKENIZERS_LIBRARY=... LAYA_BUNDLE_DIR=... ./phase0
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	laya "github.com/metalagman/laya-go"
)

func must[T any](v T, err error) T {
	if err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
	return v
}

func question(i int) laya.NoulQuestion {
	id := fmt.Sprintf("t%d", i)
	if i%2 == 0 {
		return must(laya.NewNoulQuestion(laya.QuestionID("call_"+id),
			"Tool call "+id+" (Read) should stay in the history: knowing this call was made, with its input, still matters for what the assistant does next", "", ""))
	}
	return must(laya.NewNoulQuestion(laya.QuestionID("result_"+id),
		"The full output of tool call "+id+" (Read, 4213 chars) should stay in the history verbatim: the assistant still needs its contents and re-running the tool would not do", "", ""))
}

func state(entries int) laya.State {
	type call struct{ ID, Tool, Input, Result string }
	history := []map[string]any{{"i": 0, "role": "user", "text": "Fix the failing test in pkg/x and do not touch generated files."}}
	for i := 1; i <= entries; i++ {
		history = append(history, map[string]any{"i": i, "role": "assistant", "text": "",
			"tool_calls": []map[string]string{{"id": fmt.Sprintf("t%d", i), "tool": "Read",
				"input": fmt.Sprintf(`{"file_path":"/repo/pkg/x/module_%d.go"}`, i), "result": "ok, 4213 chars (omitted)"}}})
	}
	b, _ := json.Marshal(map[string]any{
		"context": "A coding assistant conversation is being compacted to free context. `history` is the whole conversation so far.",
		"goal":    "Fix the failing test",
		"history": history,
	})
	return must(laya.JSONState(b))
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }

// probe asks questions whose right answer a human can tell: is the model's
// P(true) discriminating, or does it say ~0.85 to everything?
func probe(ctx context.Context, model *laya.Model) {
	type tc struct{ ID, Tool, Input, Result string }
	entry := func(i int, role, text string, c ...tc) map[string]any {
		e := map[string]any{"i": i, "role": role, "text": text}
		if len(c) > 0 {
			e["tool_calls"] = c
		}
		return e
	}
	history := []map[string]any{
		entry(0, "user", "Fix the failing test TestParse in pkg/parser. Do not touch generated files."),
		entry(1, "assistant", "", tc{"t1", "Read", `{"file_path":"/repo/pkg/parser/parse.go"}`, "ok, 5210 chars (omitted)"}),
		entry(2, "assistant", "", tc{"t2", "Read", `{"file_path":"/repo/docs/CHANGELOG.md"}`, "ok, 9800 chars (omitted)"}),
		entry(3, "assistant", "", tc{"t3", "Bash", `{"command":"go test ./pkg/parser"}`, "error, 1830 chars (omitted)"}),
		entry(4, "assistant", "", tc{"t4", "Read", `{"file_path":"/repo/pkg/parser/parse.go"}`, "ok, 5210 chars (omitted)"}),
		entry(5, "assistant", "", tc{"t5", "Edit", `{"file_path":"/repo/pkg/parser/parse.go","old_string":"i < n","new_string":"i <= n"}`, "ok, 120 chars (omitted)"}),
		entry(6, "assistant", "", tc{"t6", "Bash", `{"command":"go test ./pkg/parser"}`, "ok, 40 chars (omitted)"}),
		entry(7, "assistant", "All tests pass now."),
	}
	b, _ := json.Marshal(map[string]any{
		"context": "A coding assistant conversation is being compacted to free context. `history` is the whole conversation so far, oldest first; tool outputs are replaced by a short `result` note and long texts may be abridged. Each question asks whether one tool call, or the full output of that call, still needs to stay in the history verbatim. Whatever is not kept is deleted permanently, but the assistant can always re-run a tool or re-read a file.",
		"goal":    "Fix the failing test TestParse in pkg/parser. Do not touch generated files.",
		"history": history,
	})
	st := must(laya.JSONState(b))
	phrase := os.Getenv("QPHRASE")
	resultQ := func(id, tool string, chars int) laya.Question {
		switch phrase {
		case "A": // inverted polarity: P(true) = safe to delete
			return must(laya.NewNoulQuestion(laya.QuestionID("result_"+id),
				fmt.Sprintf("The output of tool call %s (%s, %d chars) is now obsolete or redundant and can be safely deleted from the history", id, tool, chars), "", ""))
		case "B": // original polarity with steering descriptions
			return must(laya.NewNoulQuestion(laya.QuestionID("result_"+id),
				fmt.Sprintf("Is the full output of tool call %s (%s, %d chars) still needed?", id, tool, chars),
				"no: it was superseded by a later call, is unrelated to the goal, or can simply be re-obtained",
				"yes: later steps depend on details that only this output contains"))
		}
		return must(laya.NewNoulQuestion(laya.QuestionID("result_"+id),
			fmt.Sprintf("The full output of tool call %s (%s, %d chars) should stay in the history verbatim: the assistant still needs its contents and re-running the tool would not do", id, tool, chars), "", ""))
	}
	callQ := func(id, tool string) laya.Question {
		if phrase == "A" {
			return must(laya.NewNoulQuestion(laya.QuestionID("call_"+id),
				fmt.Sprintf("Tool call %s (%s) is irrelevant to the goal and can be safely deleted from the history", id, tool), "", ""))
		}
		if phrase == "B" {
			return must(laya.NewNoulQuestion(laya.QuestionID("call_"+id),
				fmt.Sprintf("Does it still matter that tool call %s (%s) was made?", id, tool),
				"no: it is unrelated to the goal or was superseded", "yes: it is part of the work done toward the goal"))
		}
		return must(laya.NewNoulQuestion(laya.QuestionID("call_"+id),
			fmt.Sprintf("Tool call %s (%s) should stay in the history: knowing this call was made, with its input, still matters for what the assistant does next", id, tool), "", ""))
	}
	qs := []laya.Question{
		resultQ("t1", "Read", 5210), // superseded by t4 re-read and t5 edit: expect low
		resultQ("t2", "Read", 9800), // unrelated CHANGELOG: expect low
		resultQ("t3", "Bash", 1830), // failing output, since fixed: expect lowish
		resultQ("t6", "Bash", 40),   // final passing run: expect higher
		callQ("t2", "Read"),         // reading an unrelated file: expect low
		callQ("t5", "Edit"),         // the actual fix: expect high
	}
	start := time.Now()
	p := must(model.Predict(ctx, st, qs))
	fmt.Printf("probe: %d questions, %d input tokens, %.0f ms\n", len(qs), p.Usage().InputTokens, ms(time.Since(start)))
	for _, r := range p.Results() {
		n := r.(laya.NoulResult)
		fmt.Printf("  %-10s P(true)=%.4f\n", n.QuestionID(), n.TrueProbability())
	}
}

func main() {
	os.Setenv("ORT_DISABLE_TELEMETRY", "1")
	ctx := context.Background()
	rt := must(laya.NewRuntime(ctx, laya.RuntimeOptions{}))
	defer rt.Close(ctx)
	t0 := time.Now()
	model := must(rt.OpenModelDir(ctx, os.Getenv("LAYA_BUNDLE_DIR"), laya.ModelOptions{}))
	defer model.Close(ctx)
	fmt.Printf("model load: %.0f ms\n", ms(time.Since(t0)))
	if path := os.Getenv("EVAL"); path != "" {
		evalCorpusRun(ctx, model, path, os.Getenv("QPHRASE"))
		return
	}
	if os.Getenv("PROBE") == "1" {
		probe(ctx, model)
		return
	}

	fmt.Println("\n== latency by state size and question count (median of 5, after 1 warmup) ==")
	fmt.Printf("%-8s %-10s %-12s %-12s %-10s\n", "entries", "questions", "inputTokens", "median ms", "ms/question")
	for _, entries := range []int{1, 2, 4, 6, 8, 10, 12} {
		for _, nq := range []int{1, 2, 4} {
			var qs []laya.Question
			for i := 0; i < nq; i++ {
				qs = append(qs, question(i+1))
			}
			st := state(entries)
			if _, err := model.Predict(ctx, st, qs); err != nil {
				fmt.Printf("%-8d %-10d error: %v\n", entries, nq, err)
				continue
			}
			var times []float64
			var tokens int
			for r := 0; r < 5; r++ {
				s := time.Now()
				p := must(model.Predict(ctx, st, qs))
				times = append(times, ms(time.Since(s)))
				tokens = p.Usage().InputTokens
			}
			sort.Float64s(times)
			fmt.Printf("%-8d %-10d %-12d %-12.0f %-10.0f\n", entries, nq, tokens, times[2], times[2]/float64(nq))
		}
	}

	fmt.Println("\n== sample probabilities (8 entries) ==")
	p := must(model.Predict(ctx, state(8), []laya.Question{question(1), question(2), question(3), question(4)}))
	for _, r := range p.Results() {
		n := r.(laya.NoulResult)
		fmt.Printf("%s P(true)=%.4f action=%.4f\n", n.QuestionID(), n.TrueProbability(), n.ActionProbability())
	}

	fmt.Println("\n== token limit ==")
	for entries := 8; entries <= 40; entries++ {
		_, err := model.Predict(ctx, state(entries), []laya.Question{question(1)})
		if err != nil {
			var tl *laya.InputTooLongError
			fmt.Printf("entries=%d err=%v is(ErrInputTooLong)=%v", entries, strings.SplitN(err.Error(), ":", 2)[0], errors.Is(err, laya.ErrInputTooLong))
			if errors.As(err, &tl) {
				fmt.Printf(" observed=%d limit=%d", tl.InputTokens, tl.Limit)
			}
			fmt.Println()
			break
		}
	}
}
