package compaction

import "fmt"

// Question is one yes/no question for the model.
type Question struct {
	ID           string
	Instructions string
}

// Answers are the model's P(true) per question ID, with the input size it reported.
type Answers struct {
	Probabilities map[string]float64
	InputTokens   int
}

func callQuestionID(c ToolCall) string   { return "call_" + c.ID }
func resultQuestionID(c ToolCall) string { return "result_" + c.ID }

// QuestionsFor returns the two questions asked about one call: keep the call,
// keep its result. The texts are the original's, verbatim.
func QuestionsFor(c ToolCall) []Question {
	return []Question{
		{
			ID:           callQuestionID(c),
			Instructions: fmt.Sprintf("Tool call %s (%s) should stay in the history: knowing this call was made, with its input, still matters for what the assistant does next", c.ID, c.Tool),
		},
		{
			ID:           resultQuestionID(c),
			Instructions: fmt.Sprintf("The full output of tool call %s (%s, %d chars) should stay in the history verbatim: the assistant still needs its contents and re-running the tool would not do", c.ID, c.Tool, c.ResultChars),
		},
	}
}

func questionTokens(c ToolCall, opts ResolvedOptions) int {
	est := estimator{scale: opts.TokenScale}
	total := 0
	for _, q := range QuestionsFor(c) {
		total += est.tokens(q.Instructions) + opts.QuestionOverheadTokens
	}
	return total
}

// BatchCalls splits the candidate calls into batches whose questions, with the
// (always complete) state, fit one request.
func BatchCalls(calls []ToolCall, stateTokens int, opts ResolvedOptions) ([][]ToolCall, error) {
	budget := opts.MaxRequestTokens - stateTokens - requestOverheadTokens
	var batches [][]ToolCall
	var current []ToolCall
	currentTokens := 0
	for _, c := range calls {
		tokens := questionTokens(c, opts)
		if len(current) > 0 && currentTokens+tokens > budget {
			batches = append(batches, current)
			current, currentTokens = nil, 0
		}
		if len(current) == 0 && tokens > budget {
			return nil, fmt.Errorf("%w (~%d of %d tokens)", ErrNoRoomForQuestions, stateTokens, opts.MaxRequestTokens)
		}
		current = append(current, c)
		currentTokens += tokens
	}
	if len(current) > 0 {
		batches = append(batches, current)
	}
	return batches, nil
}
