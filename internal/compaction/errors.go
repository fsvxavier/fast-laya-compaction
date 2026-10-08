package compaction

import (
	"errors"
	"fmt"
)

var (
	// ErrStateTooLarge means the history cannot be fitted into maxStateTokens
	// by the global-state stages.
	ErrStateTooLarge = errors.New("history too large for the model")
	// ErrNoRoomForQuestions means the state leaves no room for even one call's questions.
	ErrNoRoomForQuestions = errors.New("state leaves no room for questions")
	// ErrMalformedAnswer means the model's answers are missing, non-finite or out of [0, 1].
	ErrMalformedAnswer = errors.New("invalid model answer")
	// ErrInputTooLong is returned by an Asker when the model rejects the input size.
	ErrInputTooLong = errors.New("input exceeds the model token limit")
)

// InputTooLongError is what an Asker returns when the model rejects an input
// as too long; Observed and Limit are the model's own token counts when known.
type InputTooLongError struct {
	Observed int
	Limit    int
}

func (e *InputTooLongError) Error() string {
	return fmt.Sprintf("%s: %d tokens, limit %d", ErrInputTooLong, e.Observed, e.Limit)
}

func (e *InputTooLongError) Is(target error) bool { return target == ErrInputTooLong }
