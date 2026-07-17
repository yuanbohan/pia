package utils_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/yuanbohan/pi-go/internal/coding/tools/utils"
)

func TestNormalizeWorkspacePath(t *testing.T) {
	t.Parallel()

	rootPath, displayPath, err := utils.NormalizeWorkspacePath("dir/./file.go")
	if err != nil {
		t.Fatalf("NormalizeWorkspacePath() error = %v", err)
	}
	if got, want := rootPath, filepath.FromSlash("dir/file.go"); got != want {
		t.Fatalf("root path = %q, want %q", got, want)
	}
	if got, want := displayPath, "dir/file.go"; got != want {
		t.Fatalf("display path = %q, want %q", got, want)
	}

	for _, test := range []struct {
		name string
		path string
		want string
	}{
		{name: "blank", path: "  ", want: "path is required"},
		{name: "absolute", path: filepath.Join(string(filepath.Separator), "tmp", "file.go"), want: "workspace-relative"},
		{name: "escape", path: "../file.go", want: "workspace-relative"},
		{name: "nested parent component", path: "dir/../file.go", want: "must not contain .."},
		{name: "too long", path: strings.Repeat("x", (4<<10)+1), want: "exceeds the 4096-byte limit"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, _, err := utils.NormalizeWorkspacePath(test.path)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("NormalizeWorkspacePath() error = %v, want substring %q", err, test.want)
			}
		})
	}
}
