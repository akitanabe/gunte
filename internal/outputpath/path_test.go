package outputpath

import "testing"

func TestJoinNormalizesRepositoryRootWithoutDotPrefix(t *testing.T) {
	tests := []struct {
		root, relative, want string
	}{
		{root: ".", relative: "AGENTS.md", want: "AGENTS.md"},
		{root: "dist/codex", relative: "AGENTS.md", want: "dist/codex/AGENTS.md"},
	}
	for _, test := range tests {
		if got := Join(test.root, test.relative); got != test.want {
			t.Fatalf("Join(%q, %q) = %q, want %q", test.root, test.relative, got, test.want)
		}
	}
}
