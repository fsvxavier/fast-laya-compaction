package compaction

import (
	"math"
	"unicode"
)

func isLetter(r rune) bool { return (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') }
func isDigit(r rune) bool  { return r >= '0' && r <= '9' }

// estimateRaw estimates tokens without a tokenizer: a word costs one token per
// six letters, a digit half a token, any other symbol nine tenths. This is the
// original's heuristic, calibrated against the Jev tokenizer.
func estimateRaw(text string) float64 {
	rs := []rune(text)
	tokens := 0.0
	for i := 0; i < len(rs); {
		r := rs[i]
		switch {
		case isLetter(r):
			j := i
			for j < len(rs) && isLetter(rs[j]) {
				j++
			}
			tokens += float64(1 + (j-i-1)/6)
			i = j
		case isDigit(r):
			j := i
			for j < len(rs) && isDigit(rs[j]) {
				j++
			}
			tokens += float64(j-i) / 2
			i = j
		case unicode.IsSpace(r) || r == 0xFEFF:
			i++
		default:
			tokens += 0.9
			if r > 0xFFFF { // two UTF-16 units in the original
				tokens += 0.9
			}
			i++
		}
	}
	return tokens
}

// EstimateTokens is the original estimate with no calibration.
func EstimateTokens(text string) int { return int(math.Ceil(estimateRaw(text))) }

// estimator applies the calibration scale.
type estimator struct{ scale float64 }

func (e estimator) tokens(text string) int {
	return int(math.Ceil(estimateRaw(text) * e.scale))
}
