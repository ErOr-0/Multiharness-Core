package folder

import (
	"math/rand"
	"strings"
	"testing"
)

func TestLineEditsReproduceTarget(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	words := []string{"a\n", "b\n", "c\n", "d\n"}
	for range 500 {
		a := make([]string, r.Intn(12))
		b := make([]string, r.Intn(12))
		for i := range a {
			a[i] = words[r.Intn(len(words))]
		}
		for i := range b {
			b[i] = words[r.Intn(len(words))]
		}
		ops, ok := lineEdits(a, b, len(a)+len(b))
		if !ok {
			t.Fatal("edit bound must cover n+m")
		}
		var got []string
		ai, bi := 0, 0
		for _, op := range ops {
			switch op {
			case ' ':
				if a[ai] != b[bi] {
					t.Fatalf("kept unequal lines %q/%q", a[ai], b[bi])
				}
				got, ai, bi = append(got, a[ai]), ai+1, bi+1
			case '-':
				ai++
			case '+':
				got, bi = append(got, b[bi]), bi+1
			}
		}
		if ai != len(a) || strings.Join(got, "") != strings.Join(b, "") {
			t.Fatalf("edits do not transform %q into %q: %q", a, b, ops)
		}
	}
}

func TestUnifiedHunksSeparateDistantChanges(t *testing.T) {
	var a []string
	for i := range 100 {
		a = append(a, strings.Repeat("l", i+1)+"\n")
	}
	b := append([]string(nil), a...)
	b[1], b[90] = "first\n", "second\n"
	hunks := unifiedHunks(a, b)
	if len(hunks) != 2 {
		t.Fatalf("want 2 hunks, got %d: %q", len(hunks), hunks)
	}
	if !strings.HasPrefix(hunks[0], "@@ -1,5 +1,5 @@\n") || !strings.HasPrefix(hunks[1], "@@ -88,7 +88,7 @@\n") {
		t.Fatalf("unexpected hunk headers: %q", hunks)
	}
	if got := unifiedHunks([]string{"x"}, []string{"y"}); len(got) != 1 || !strings.Contains(got[0], "\\ No newline at end of file") {
		t.Fatalf("missing final-newline marker: %q", got)
	}
}

func TestUnifiedHunksFallBackOnLargeRewrite(t *testing.T) {
	var a, b []string
	for i := range maxDiffEdits {
		a = append(a, "old"+strings.Repeat("x", i%7)+"\n")
		b = append(b, "new"+strings.Repeat("y", i%5)+"\n")
	}
	hunks := unifiedHunks(a, b)
	if len(hunks) != 1 || strings.Count(hunks[0], "\n-") != len(a) || strings.Count(hunks[0], "\n+") != len(b) {
		t.Fatalf("large rewrite should become one replacement hunk")
	}
}
