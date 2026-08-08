package gen

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// Odin reserved words that could collide with generated field or parameter names.
var odinKeywords = map[string]bool{
	"asm": true, "auto_cast": true, "bit_field": true, "bit_set": true,
	"break": true, "case": true, "cast": true, "context": true, "continue": true,
	"defer": true, "distinct": true, "do": true, "dynamic": true, "else": true,
	"enum": true, "fallthrough": true, "for": true, "foreign": true, "if": true,
	"import": true, "in": true, "map": true, "matrix": true, "not_in": true,
	"or_break": true, "or_continue": true, "or_else": true, "or_return": true,
	"package": true, "proc": true, "return": true, "struct": true, "switch": true,
	"transmute": true, "typeid": true, "union": true, "using": true, "when": true,
	"where": true,
}

var nonIdent = regexp.MustCompile(`[^A-Za-z0-9]+`)

// snakeWords splits any identifier-ish string (camelCase, spaces, hyphens)
// into lowercase words.
func snakeWords(s string) []string {
	// Break camelCase boundaries first.
	var b strings.Builder
	runes := []rune(s)
	for i, r := range runes {
		if i > 0 && unicode.IsUpper(r) && (unicode.IsLower(runes[i-1]) ||
			(i+1 < len(runes) && unicode.IsLower(runes[i+1]) && unicode.IsUpper(runes[i-1]))) {
			b.WriteByte(' ')
		}
		b.WriteRune(r)
	}
	parts := nonIdent.Split(b.String(), -1)
	words := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			words = append(words, strings.ToLower(p))
		}
	}
	return words
}

// SnakeCase converts to lower snake_case, escaping Odin keywords with a
// trailing underscore.
func SnakeCase(s string) string {
	out := strings.Join(snakeWords(s), "_")
	if odinKeywords[out] {
		out += "_"
	}
	if out == "" {
		panic(fmt.Sprintf("cannot derive identifier from %q", s))
	}
	return out
}

// AdaCase converts to Odin type-style Ada_Case (e.g. Transaction_Account).
func AdaCase(s string) string {
	words := snakeWords(s)
	for i, w := range words {
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	out := strings.Join(words, "_")
	if out == "" {
		panic(fmt.Sprintf("cannot derive type name from %q", s))
	}
	return out
}

// fillerWords are dropped when deriving proc names from operation summaries.
var fillerWords = map[string]bool{"a": true, "an": true, "the": true}

// verbNormalize maps third-person verbs in summaries to imperative form.
var verbNormalize = map[string]string{
	"lists": "list", "gets": "get", "creates": "create", "updates": "update",
	"deletes": "delete", "assigns": "assign", "unassigns": "unassign",
}

// ProcName derives an Odin proc name from an operation summary,
// e.g. "Get a transaction" -> "get_transaction".
func ProcName(summary string) string {
	words := snakeWords(summary)
	out := make([]string, 0, len(words))
	for i, w := range words {
		if fillerWords[w] {
			continue
		}
		if i == 0 {
			if v, ok := verbNormalize[w]; ok {
				w = v
			}
		}
		out = append(out, w)
	}
	if len(out) == 0 {
		panic(fmt.Sprintf("cannot derive proc name from summary %q", summary))
	}
	return strings.Join(out, "_")
}
