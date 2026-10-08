package compaction

import "math"

// Options are the caller-facing settings. A nil field takes its default; a
// non-finite value is ignored, as in the original.
type Options struct {
	// Goal is the ongoing task; empty means the last few user prompts.
	Goal                   string   `json:"goal,omitempty"`
	KeepThreshold          *float64 `json:"keepThreshold,omitempty"`
	PreserveRecentMessages *float64 `json:"preserveRecentMessages,omitempty"`
	MaxStateTokens         *float64 `json:"maxStateTokens,omitempty"`
	MaxRequestTokens       *float64 `json:"maxRequestTokens,omitempty"`
	TruncateHeadChars      *float64 `json:"truncateHeadChars,omitempty"`
	WindowCalls            *float64 `json:"windowCalls,omitempty"`
	// InputTokenLimit is the model's limit; the two budgets derive from it.
	InputTokenLimit *float64 `json:"inputTokenLimit,omitempty"`
	// TokenScale multiplies the token estimate (calibration).
	TokenScale *float64 `json:"tokenScale,omitempty"`
	// QuestionOverheadTokens is added to each question's estimated cost.
	QuestionOverheadTokens *float64 `json:"questionOverheadTokens,omitempty"`
}

// ResolvedOptions are Options with every default applied.
type ResolvedOptions struct {
	Goal                   string
	KeepThreshold          float64
	PreserveRecentMessages int
	MaxStateTokens         int
	MaxRequestTokens       int
	TruncateHeadChars      int
	WindowCalls            int
	InputTokenLimit        int
	TokenScale             float64
	QuestionOverheadTokens int
}

const (
	defaultInputTokenLimit = 1024
	// requestMargin leaves room for special tokens and estimation error.
	requestMargin = 64
	// requestOverheadTokens is what the request envelope adds around state and questions.
	requestOverheadTokens = 20
)

func finite(v *float64, fallback float64) float64 {
	if v != nil && !math.IsNaN(*v) && !math.IsInf(*v, 0) {
		return *v
	}
	return fallback
}

// ResolveOptions applies defaults. The token budgets derive from the model
// limit (maxRequest = limit - 64, maxState = 5/6 of that, the original's
// 25k/30k proportion) unless given explicitly.
func ResolveOptions(o Options) ResolvedOptions {
	limit := int(math.Max(1, math.Floor(finite(o.InputTokenLimit, defaultInputTokenLimit))))
	maxRequest := float64(limit - requestMargin)
	if maxRequest < 1 {
		maxRequest = 1
	}
	maxState := math.Round(maxRequest * 25 / 30)
	return ResolvedOptions{
		Goal:                   o.Goal,
		KeepThreshold:          finite(o.KeepThreshold, 0.5),
		PreserveRecentMessages: int(math.Max(0, math.Floor(finite(o.PreserveRecentMessages, 6)))),
		MaxStateTokens:         int(math.Max(1, finite(o.MaxStateTokens, maxState))),
		MaxRequestTokens:       int(math.Max(1, finite(o.MaxRequestTokens, maxRequest))),
		TruncateHeadChars:      int(math.Max(0, math.Floor(finite(o.TruncateHeadChars, 300)))),
		WindowCalls:            int(math.Max(1, math.Floor(finite(o.WindowCalls, 2)))),
		InputTokenLimit:        limit,
		TokenScale:             math.Max(0, finite(o.TokenScale, 1)),
		QuestionOverheadTokens: int(math.Max(0, math.Floor(finite(o.QuestionOverheadTokens, 8)))),
	}
}
