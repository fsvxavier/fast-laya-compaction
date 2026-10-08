package compaction_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	c "github.com/fsvxavier/fast-laya-compaction/internal/compaction"
)

// The golden files are produced by the ORIGINAL fast-jev-compaction (commit
// e3f262a) running fitState; see tools/golden/gen.mts. They prove the Go port
// builds the same state, picks the same stage and estimates the same tokens.
type goldenFile struct {
	Messages []c.Message `json:"messages"`
	Cases    []struct {
		Options struct {
			MaxStateTokens         float64 `json:"maxStateTokens"`
			PreserveRecentMessages float64 `json:"preserveRecentMessages"`
			Goal                   string  `json:"goal"`
		} `json:"options"`
		Expected struct {
			Stage  string          `json:"stage"`
			Tokens int             `json:"tokens"`
			State  json.RawMessage `json:"state"`
			Error  bool            `json:"error"`
		} `json:"expected"`
	} `json:"cases"`
}

func compactRaw(t *testing.T, raw []byte) string {
	t.Helper()
	var b bytes.Buffer
	if err := json.Compact(&b, raw); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func TestFitStateMatchesOriginal(t *testing.T) {
	files, _ := filepath.Glob("testdata/stages/*.json")
	if len(files) == 0 {
		t.Fatal("no golden files")
	}
	stages := map[string]int{}
	for _, path := range files {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var g goldenFile
		if err := json.Unmarshal(raw, &g); err != nil {
			t.Fatal(err)
		}
		for i, tc := range g.Cases {
			o := c.ResolveOptions(c.Options{
				MaxStateTokens:         f(tc.Options.MaxStateTokens),
				PreserveRecentMessages: f(tc.Options.PreserveRecentMessages),
				Goal:                   tc.Options.Goal,
			})
			calls := c.CollectToolCalls(g.Messages, o.PreserveRecentMessages)
			got, err := c.FitState(g.Messages, calls, o)
			name := filepath.Base(path)
			if tc.Expected.Error {
				if !errors.Is(err, c.ErrStateTooLarge) {
					t.Errorf("%s case %d: want ErrStateTooLarge, got %v (stage %q)", name, i, err, got.Stage)
				}
				stages["<error>"]++
				continue
			}
			if err != nil {
				t.Errorf("%s case %d: %v", name, i, err)
				continue
			}
			stages[got.Stage]++
			if got.Stage != tc.Expected.Stage || got.Tokens != tc.Expected.Tokens {
				t.Errorf("%s case %d (max %v, preserve %v): stage/tokens = %q/%d, want %q/%d",
					name, i, tc.Options.MaxStateTokens, tc.Options.PreserveRecentMessages,
					got.Stage, got.Tokens, tc.Expected.Stage, tc.Expected.Tokens)
				continue
			}
			gotJSON, _ := json.Marshal(got.State) // HistoryEntry marshals itself; compare compactly below
			_ = gotJSON
			if want, have := compactRaw(t, tc.Expected.State), marshalState(got.State); want != have {
				t.Errorf("%s case %d: state differs\nwant %.300s\nhave %.300s", name, i, want, have)
			}
		}
	}
	// every stage of the original must be exercised by the corpus
	for _, s := range []string{"full", "inputs<=200", "inputs<=60", "texts abridged", "old messages collapsed",
		"old calls compacted", "old messages left out", "old calls merged", "<error>"} {
		if stages[s] == 0 {
			t.Errorf("golden corpus never reaches stage %q (got %v)", s, stages)
		}
	}
}

// marshalState serialises like JSON.stringify (no HTML escaping).
func marshalState(s c.State) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s)
	return string(bytes.TrimSuffix(b.Bytes(), []byte("\n")))
}
