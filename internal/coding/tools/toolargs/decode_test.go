package toolargs_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/yuanbohan/pia/internal/coding/tools/toolargs"
)

func TestDecode(t *testing.T) {
	t.Parallel()

	type arguments struct {
		Path string `json:"path"`
	}

	got, err := toolargs.Decode[arguments](json.RawMessage(`{"path":"main.go"}`))
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if got.Path != "main.go" {
		t.Fatalf("Decode() path = %q, want main.go", got.Path)
	}

	for _, test := range []struct {
		name  string
		input string
		want  string
	}{
		{name: "malformed", input: `{`, want: "unexpected EOF"},
		{name: "null", input: `null`, want: "must be a JSON object"},
		{name: "array", input: `[]`, want: "must be a JSON object"},
		{name: "unknown field", input: `{"path":"main.go","extra":true}`, want: "unknown field"},
		{name: "trailing object", input: `{"path":"main.go"}{}`, want: "exactly one JSON object"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := toolargs.Decode[arguments](json.RawMessage(test.input))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Decode() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestDecodeBoundsModelControlledDiagnostics(t *testing.T) {
	t.Parallel()

	type arguments struct {
		Path string `json:"path"`
	}
	field := strings.Repeat("x", 64<<10)
	_, err := toolargs.Decode[arguments](json.RawMessage(`{"` + field + `":true}`))
	if err == nil {
		t.Fatal("Decode() error = nil, want unknown-field error")
	}
	if !strings.Contains(err.Error(), "truncated") {
		t.Fatalf("Decode() error = %q, want truncation marker", err)
	}
	if len(err.Error()) > 600 {
		t.Fatalf("Decode() error length = %d, want bounded diagnostic", len(err.Error()))
	}
}
