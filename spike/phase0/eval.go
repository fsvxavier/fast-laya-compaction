package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"

	laya "github.com/metalagman/laya-go"
)

type evalQuestion struct {
	ID     string `json:"id"`
	CallID string `json:"callId"`
	Tool   string `json:"tool"`
	Chars  int    `json:"chars"`
	Label  int    `json:"label"` // 1 = result still needed
	Kind   string `json:"kind"`
}

type evalCorpus struct {
	Scenarios []struct {
		State     json.RawMessage `json:"state"`
		Questions []evalQuestion  `json:"questions"`
	} `json:"scenarios"`
}

// buildQuestion renders one result question in the chosen phrasing and says
// whether P(true) means "keep" (true) or "drop" (false).
func buildQuestion(phrase string, q evalQuestion) (laya.Question, bool) {
	id := laya.QuestionID(q.ID)
	switch phrase {
	case "A":
		return must(laya.NewNoulQuestion(id, fmt.Sprintf("The output of tool call %s (%s, %d chars) is now obsolete or redundant and can be safely deleted from the history", q.CallID, q.Tool, q.Chars), "", "")), false
	case "B":
		return must(laya.NewNoulQuestion(id, fmt.Sprintf("Is the full output of tool call %s (%s, %d chars) still needed?", q.CallID, q.Tool, q.Chars),
			"no: it was superseded by a later call, is unrelated to the goal, or can simply be re-obtained",
			"yes: later steps depend on details that only this output contains")), true
	}
	return must(laya.NewNoulQuestion(id, fmt.Sprintf("The full output of tool call %s (%s, %d chars) should stay in the history verbatim: the assistant still needs its contents and re-running the tool would not do", q.CallID, q.Tool, q.Chars), "", "")), true
}

func auc(scores []float64, labels []int) float64 {
	var pos, neg []float64
	for i, l := range labels {
		if l == 1 {
			pos = append(pos, scores[i])
		} else {
			neg = append(neg, scores[i])
		}
	}
	wins := 0.0
	for _, p := range pos {
		for _, n := range neg {
			switch {
			case p > n:
				wins++
			case p == n:
				wins += 0.5
			}
		}
	}
	return wins / float64(len(pos)*len(neg))
}

func evalCorpusRun(ctx context.Context, model *laya.Model, path, phrase string) {
	var corpus evalCorpus
	raw := must(os.ReadFile(path))
	if err := json.Unmarshal(raw, &corpus); err != nil {
		panic(err)
	}
	var scores []float64 // always "probability the result is needed"
	var labels []int
	byKind := map[string][]float64{}
	for si, sc := range corpus.Scenarios {
		st := must(laya.JSONState(sc.State))
		var qs []laya.Question
		keep := make([]bool, len(sc.Questions))
		for i, q := range sc.Questions {
			question, k := buildQuestion(phrase, q)
			qs = append(qs, question)
			keep[i] = k
		}
		p, err := model.Predict(ctx, st, qs)
		if err != nil {
			fmt.Printf("scenario %d: %v\n", si, err)
			continue
		}
		for i, r := range p.Results() {
			v := r.(laya.NoulResult).TrueProbability()
			if !keep[i] {
				v = 1 - v
			}
			scores = append(scores, v)
			labels = append(labels, sc.Questions[i].Label)
			byKind[sc.Questions[i].Kind] = append(byKind[sc.Questions[i].Kind], v)
		}
	}
	fmt.Printf("phrase=%q questions=%d AUC=%.3f\n", phrase, len(scores), auc(scores, labels))
	for _, k := range []string{"superseded", "irrelevant", "error_fixed", "cited", "error_cited"} {
		vs := byKind[k]
		if len(vs) == 0 {
			continue
		}
		sort.Float64s(vs)
		sum := 0.0
		for _, v := range vs {
			sum += v
		}
		fmt.Printf("  %-12s n=%2d mean=%.3f min=%.3f max=%.3f\n", k, len(vs), sum/float64(len(vs)), vs[0], vs[len(vs)-1])
	}
	// best accuracy over all thresholds, and accuracy at the 0.5 the algorithm uses
	best, bestT, at05 := 0.0, 0.0, 0.0
	for _, t := range append(append([]float64{}, scores...), 0.5) {
		ok := 0
		for i, s := range scores {
			if (s >= t) == (labels[i] == 1) {
				ok++
			}
		}
		acc := float64(ok) / float64(len(scores))
		if acc > best {
			best, bestT = acc, t
		}
		if t == 0.5 {
			at05 = acc
		}
	}
	fmt.Printf("  accuracy: best=%.3f at threshold %.3f; at 0.5=%.3f; majority baseline=%.3f\n", best, bestT, at05, 85.0/137)
}
