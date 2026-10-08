package compaction

import (
	"context"
	"fmt"
	"math"
	"time"
)

// Asker answers yes/no questions about a state. The Laya adapter implements
// it; fakelaya implements it for tests.
type Asker interface {
	Ask(ctx context.Context, state State, questions []Question) (Answers, error)
}

func askBatch(ctx context.Context, asker Asker, state State, batch []ToolCall) (map[string]CallAnswer, Answers, error) {
	var questions []Question
	for _, c := range batch {
		questions = append(questions, QuestionsFor(c)...)
	}
	answers, err := asker.Ask(ctx, state, questions)
	if err != nil {
		return nil, answers, err
	}
	probability := func(id string) (float64, error) {
		p, ok := answers.Probabilities[id]
		if !ok || math.IsNaN(p) || math.IsInf(p, 0) || p < 0 || p > 1 {
			return 0, fmt.Errorf("%w for %s", ErrMalformedAnswer, id)
		}
		return p, nil
	}
	out := make(map[string]CallAnswer, len(batch))
	for _, c := range batch {
		keepCall, err := probability(callQuestionID(c))
		if err != nil {
			return nil, answers, err
		}
		keepResult, err := probability(resultQuestionID(c))
		if err != nil {
			return nil, answers, err
		}
		out[c.ID] = CallAnswer{KeepCall: keepCall, KeepResult: keepResult}
	}
	return out, answers, nil
}

// Compact asks the model, for every tool call outside the pinned first and
// newest messages, whether the call and whether its result must stay. The
// whole history (results omitted, fitted into MaxStateTokens) is sent as state
// with every batch of questions; batches run in sequence. It returns an error
// when the model fails or the history cannot be fitted; the caller decides
// whether to fall back.
func Compact(ctx context.Context, messages []Message, asker Asker, options Options) (Result, error) {
	started := time.Now()
	opts := ResolveOptions(options)
	calls := CollectToolCalls(messages, opts.PreserveRecentMessages)
	var candidates []ToolCall
	for _, c := range calls {
		if !c.Pinned {
			candidates = append(candidates, c)
		}
	}
	charsBefore := 0
	for _, m := range messages {
		charsBefore += MessageChars(m)
	}

	stats := Stats{}
	answers := map[string]CallAnswer{}
	if len(candidates) > 0 {
		fitted, err := FitState(messages, calls, opts)
		if err != nil {
			return Result{}, err
		}
		stats.StateTokens, stats.StateStage = fitted.Tokens, fitted.Stage
		batches, err := BatchCalls(candidates, fitted.Tokens, opts)
		if err != nil {
			return Result{}, err
		}
		for _, batch := range batches {
			if err := ctx.Err(); err != nil {
				return Result{}, err
			}
			got, raw, err := askBatch(ctx, asker, fitted.State, batch)
			if err != nil {
				return Result{}, err
			}
			stats.Requests++
			stats.InputTokensReported += raw.InputTokens
			for id, a := range got {
				answers[id] = a
			}
		}
	}

	decisions := make([]Decision, 0, len(calls))
	for _, c := range calls {
		a, ok := answers[c.ID]
		if !ok {
			a = CallAnswer{KeepCall: 1, KeepResult: 1}
		}
		decisions = append(decisions, DecideCall(c, a, opts))
	}
	kept := ApplyDecisions(messages, decisions, calls, opts.TruncateHeadChars)

	stats.MessagesBefore, stats.MessagesAfter = len(messages), len(kept)
	stats.CharsBefore = charsBefore
	for _, m := range kept {
		stats.CharsAfter += outMessageChars(m)
	}
	stats.Calls = len(calls)
	for _, d := range decisions {
		switch d.Reason {
		case ReasonKept:
			stats.Kept++
		case ReasonResultDropped:
			stats.ResultsDropped++
		case ReasonCallDropped:
			stats.CallsDropped++
		case ReasonPinned:
			stats.Pinned++
		case ReasonUnfit:
			stats.Unfit++
		}
	}
	stats.ReductionRatio = ReductionRatio(stats)
	stats.Ms = time.Since(started).Milliseconds()
	return Result{Messages: kept, Decisions: decisions, Stats: stats}, nil
}
