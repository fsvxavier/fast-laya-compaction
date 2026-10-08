// Package fakelaya is a deterministic compaction.Asker for tests: no model, no cgo.
package fakelaya

import (
	"context"
	"sync"

	"github.com/fsvxavier/fast-laya-compaction/internal/compaction"
)

// Request is one recorded Ask call.
type Request struct {
	State       compaction.State
	QuestionIDs []string
}

// Asker answers every question with Answer(id) and records what it was asked.
type Asker struct {
	// Answer returns P(true) for a question ID.
	Answer func(id string) float64
	// Err, if set, is returned by every Ask.
	Err error
	// Fail, if set, is consulted with the 0-based request number before answering.
	Fail func(n int) error
	// Override, if set, replaces the answers entirely (for malformed-answer tests).
	Override func(questions []compaction.Question) compaction.Answers

	mu   sync.Mutex
	Seen []Request
}

func (a *Asker) Ask(_ context.Context, state compaction.State, questions []compaction.Question) (compaction.Answers, error) {
	ids := make([]string, len(questions))
	for i, q := range questions {
		ids[i] = q.ID
	}
	a.mu.Lock()
	n := len(a.Seen)
	a.Seen = append(a.Seen, Request{State: state, QuestionIDs: ids})
	a.mu.Unlock()
	if a.Fail != nil {
		if err := a.Fail(n); err != nil {
			return compaction.Answers{}, err
		}
	}
	if a.Err != nil {
		return compaction.Answers{}, a.Err
	}
	if a.Override != nil {
		return a.Override(questions), nil
	}
	probs := make(map[string]float64, len(ids))
	for _, id := range ids {
		probs[id] = a.Answer(id)
	}
	return compaction.Answers{Probabilities: probs}, nil
}
