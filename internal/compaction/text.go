package compaction

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// runeLen counts characters, not bytes (the original counts UTF-16 units).
func runeLen(s string) int { return utf8.RuneCountInString(s) }

// sliceRunes returns runes [from, to) of s.
func sliceRunes(s string, from, to int) string {
	rs := []rune(s)
	if from < 0 {
		from = 0
	}
	if to > len(rs) {
		to = len(rs)
	}
	if from >= to {
		return ""
	}
	return string(rs[from:to])
}

func truncate(text string, limit int) string {
	if runeLen(text) <= limit {
		return text
	}
	return sliceRunes(text, 0, max(0, limit-1)) + "…"
}

func abridge(text string, head, tail int) string {
	n := runeLen(text)
	if n <= head+tail+40 {
		return text
	}
	omitted := n - head - tail
	return fmt.Sprintf("%s\n[… %d chars omitted …]\n%s", sliceRunes(text, 0, head), omitted, sliceRunes(text, n-tail, n))
}

// marshal is JSON.stringify: compact and without HTML escaping, so sizes match
// what the model is sent.
func marshal(v any) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return "null"
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// compactJSON normalises raw JSON to its compact form, keeping key order.
func compactJSON(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 {
		return "", false
	}
	var b bytes.Buffer
	if err := json.Compact(&b, raw); err != nil {
		return "", false
	}
	return b.String(), true
}

var whitespaceRun = regexp.MustCompile(`(?:\s|\p{Z}|\x{FEFF})+`)

func collapseWhitespace(s string) string { return whitespaceRun.ReplaceAllString(s, " ") }

// objectEntry is one key/value of a JSON object, in document order.
type objectEntry struct {
	Key   string
	Value json.RawMessage
}

// orderedEntries lists the entries of a JSON object in document order
// (Object.entries); anything that is not an object has none.
func orderedEntries(raw json.RawMessage) []objectEntry {
	dec := json.NewDecoder(bytes.NewReader(raw))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return nil
	}
	var entries []objectEntry
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return entries
		}
		key, _ := keyTok.(string)
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return entries
		}
		entries = append(entries, objectEntry{Key: key, Value: value})
	}
	return entries
}
