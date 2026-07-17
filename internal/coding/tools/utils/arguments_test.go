package utils_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/yuanbohan/pi-go/internal/coding/tools/utils"
)

func TestDecodeArguments(t *testing.T) {
	t.Parallel()

	type arguments struct {
		Path string `json:"path"`
	}

	got, err := utils.DecodeArguments[arguments](json.RawMessage(`{"path":"main.go"}`))
	if err != nil {
		t.Fatalf("DecodeArguments() error = %v", err)
	}
	if got.Path != "main.go" {
		t.Fatalf("DecodeArguments() path = %q, want main.go", got.Path)
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
			_, err := utils.DecodeArguments[arguments](json.RawMessage(test.input))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("DecodeArguments() error = %v, want substring %q", err, test.want)
			}
		})
	}
}
