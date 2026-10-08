// Command corpus writes a labeled synthetic corpus for measuring whether the
// model separates tool results that are still needed from those that are not.
// Labels hold by construction (see the segment kinds), and every state is built
// by the production FitState, so the model sees the real format.
//
//	go run ./tools/corpus > testdata/probe/corpus.json
package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"

	c "github.com/fsvxavier/fast-laya-compaction/internal/compaction"
)

type question struct {
	ID     string `json:"id"`
	CallID string `json:"callId"`
	Tool   string `json:"tool"`
	Chars  int    `json:"chars"`
	// Label is 1 when the result is still needed, 0 when it can be dropped.
	Label int    `json:"label"`
	Kind  string `json:"kind"`
}

type scenario struct {
	State     json.RawMessage `json:"state"`
	Tokens    int             `json:"tokens"`
	Questions []question      `json:"questions"`
}

type builder struct {
	rng    *rand.Rand
	msgs   []c.Message
	next   int
	labels map[string]question // tool_use_id -> question template
	order  []string
}

func (b *builder) say(role c.Role, text string) {
	b.msgs = append(b.msgs, c.Message{Role: role, Text: text})
}

func (b *builder) call(tool, input string, chars int, isErr bool, label int, kind string) string {
	b.next++
	id := fmt.Sprintf("tu%d", b.next)
	b.msgs = append(b.msgs,
		c.Message{Role: c.RoleAssistant, ToolUses: []c.ToolUse{{ToolUseID: id, Tool: tool, Input: json.RawMessage(input)}}},
		c.Message{Role: c.RoleUser, ToolResults: []c.ToolResult{{ToolUseID: id, Text: string(make([]byte, 0)) + pad(chars), IsError: isErr}}})
	if kind != "" {
		b.labels[id] = question{Tool: tool, Chars: chars, Label: label, Kind: kind}
		b.order = append(b.order, id)
	}
	return id
}

func pad(n int) string { return string(make([]rune, 0)) + fmt.Sprintf("%*s", n, "") }

var files = []string{"parse.go", "lexer.go", "config.go", "server.go", "cache.go", "router.go", "store.go", "client.go", "auth.go", "retry.go"}
var dirs = []string{"pkg/parser", "pkg/net", "internal/core", "cmd/app", "internal/store"}
var funcs = []string{"handleRequest", "parseHeader", "loadConfig", "flushBuffer", "nextToken", "resolveRoute", "refreshToken", "evictEntry"}
var docs = []string{"docs/CHANGELOG.md", "README.md", "docs/architecture.md", "LICENSE", "docs/faq.md"}

func (b *builder) path() string {
	return fmt.Sprintf("/repo/%s/%s", dirs[b.rng.Intn(len(dirs))], files[b.rng.Intn(len(files))])
}

func (b *builder) segment(kind string) {
	r := b.rng
	p := b.path()
	size := 800 + r.Intn(9000)
	read := fmt.Sprintf(`{"file_path":%q}`, p)
	switch kind {
	case "superseded": // read, then edit or re-read the same file
		b.call("Read", read, size, false, 0, "superseded")
		b.say(c.RoleAssistant, "I will change "+funcs[r.Intn(len(funcs))]+" now.")
		if r.Intn(2) == 0 {
			b.call("Edit", fmt.Sprintf(`{"file_path":%q,"old_string":"i < n","new_string":"i <= n"}`, p), 120, false, 0, "")
		} else {
			b.call("Read", read, size, false, 0, "")
		}
	case "irrelevant": // unrelated document, never mentioned again
		d := docs[r.Intn(len(docs))]
		b.call("Read", fmt.Sprintf(`{"file_path":"/repo/%s"}`, d), 3000+r.Intn(9000), false, 0, "irrelevant")
		b.say(c.RoleAssistant, "Nothing relevant there, moving on.")
	case "cited": // content is used by later reasoning, file not touched again
		b.call("Read", read, size, false, 1, "cited")
		b.say(c.RoleAssistant, fmt.Sprintf("In %s, %s returns early when the slice is empty, so the caller must guard against that before dereferencing.", p, funcs[r.Intn(len(funcs))]))
	case "error_cited": // failing output the next steps rely on, never rerun
		cmd := fmt.Sprintf("go test ./%s", dirs[r.Intn(len(dirs))])
		b.call("Bash", fmt.Sprintf(`{"command":%q}`, cmd), 1500+r.Intn(3000), true, 1, "error_cited")
		b.say(c.RoleAssistant, fmt.Sprintf("The failure is a nil pointer dereference at %s:%d; I need to inspect how it is initialised.", p, 10+r.Intn(200)))
	case "error_fixed": // failure, then edit and a passing rerun of the same command
		cmd := fmt.Sprintf("go test ./%s", dirs[r.Intn(len(dirs))])
		b.call("Bash", fmt.Sprintf(`{"command":%q}`, cmd), 1500+r.Intn(3000), true, 0, "error_fixed")
		b.call("Edit", fmt.Sprintf(`{"file_path":%q,"old_string":"x := nil","new_string":"x := &T{}"}`, p), 120, false, 0, "")
		b.call("Bash", fmt.Sprintf(`{"command":%q}`, cmd), 40, false, 0, "")
	}
}

func main() {
	rng := rand.New(rand.NewSource(7))
	kinds := []string{"superseded", "irrelevant", "cited", "error_cited", "error_fixed"}
	var out struct {
		Scenarios []scenario `json:"scenarios"`
	}
	opts := c.ResolveOptions(c.Options{PreserveRecentMessages: ptr(0), Goal: "Fix the failing test and keep the change minimal."})
	for n := 0; n < 60 && len(out.Scenarios) < 40; n++ {
		b := &builder{rng: rng, labels: map[string]question{}}
		b.say(c.RoleUser, "Fix the failing test and keep the change minimal. Do not touch generated files.")
		for _, i := range rng.Perm(len(kinds))[:3+rng.Intn(2)] {
			b.segment(kinds[i])
		}
		b.say(c.RoleAssistant, "Continuing with the next step.")
		calls := c.CollectToolCalls(b.msgs, 0)
		fit, err := c.FitState(b.msgs, calls, opts)
		if err != nil || fit.Tokens > 800 {
			if os.Getenv("DEBUG") != "" {
				fmt.Fprintln(os.Stderr, "skip", fit.Tokens, err)
			} // leave room for the question sequence (~130 tokens)
			continue
		}
		sc := scenario{State: json.RawMessage(c.MarshalState(fit.State)), Tokens: fit.Tokens}
		for _, call := range calls {
			if tpl, ok := b.labels[call.ToolUseID]; ok {
				tpl.CallID, tpl.ID = call.ID, "result_"+call.ID
				sc.Questions = append(sc.Questions, tpl)
			}
		}
		if len(sc.Questions) > 0 {
			out.Scenarios = append(out.Scenarios, sc)
		}
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(out)
}

func ptr(v float64) *float64 { return &v }
