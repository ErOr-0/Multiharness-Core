package folder

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"
)

// Unified diff with context lines for model review. Canonical evidence is
// retained in full; reviewers receive bounded chunks built from this diff.
// Binary changes carry content hashes; this report is not an executable patch.
const diffContextLines = 3

func (w *Workspace) diff(ctx context.Context, before, after map[string]*fileState, names []string) (string, error) {
	var output strings.Builder
	write := func(text string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if w.config.MaxOutputBytes > 0 && len(text) > w.config.MaxOutputBytes-output.Len() {
			return fmt.Errorf("file diff exceeds %d bytes", w.config.MaxOutputBytes)
		}
		output.WriteString(text)
		return nil
	}
	for _, name := range names {
		a, b := before[name], after[name]
		if err := write(fmt.Sprintf("--- %q\n+++ %q\n", "before/"+name, "after/"+name)); err != nil {
			return "", err
		}
		for _, side := range []struct {
			label string
			file  *fileState
		}{{"before", a}, {"after", b}} {
			if side.file != nil {
				if err := write(fmt.Sprintf("%s mode: %s\n", side.label, side.file.mode)); err != nil {
					return "", err
				}
			}
		}
		binary := func(f *fileState) bool {
			return f != nil && (!utf8.Valid(f.data) || strings.ContainsRune(string(f.data), 0))
		}
		if binary(a) || binary(b) {
			for _, side := range []struct {
				label string
				file  *fileState
			}{{"before", a}, {"after", b}} {
				if side.file != nil {
					if err := write(fmt.Sprintf("%s binary: %d bytes sha256:%x\n", side.label, len(side.file.data), sha256.Sum256(side.file.data))); err != nil {
						return "", err
					}
				}
			}
			continue
		}
		var aLines, bLines []string
		if a != nil {
			if a.mode&os.ModeSymlink != 0 {
				if err := write("symlink target:\n"); err != nil {
					return "", err
				}
			}
			aLines = splitLines(string(a.data))
		}
		if b != nil {
			if b.mode&os.ModeSymlink != 0 && (a == nil || a.mode&os.ModeSymlink == 0) {
				if err := write("symlink target:\n"); err != nil {
					return "", err
				}
			}
			bLines = splitLines(string(b.data))
		}
		for _, h := range unifiedHunks(aLines, bLines) {
			if err := write(h); err != nil {
				return "", err
			}
		}
	}
	return output.String(), nil
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.SplitAfter(s, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// maxDiffEdits bounds Myers' O(D^2) trace. Larger rewrites fall back to one
// replacement hunk, which is no larger than the rewrite itself.
const maxDiffEdits = 2000

// unifiedHunks emits separate context hunks for each changed region, so a
// one-line edit in a large file yields one small hunk, not full copies.
func unifiedHunks(a, b []string) []string {
	prefix := 0
	for prefix < len(a) && prefix < len(b) && a[prefix] == b[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < len(a)-prefix && suffix < len(b)-prefix && a[len(a)-1-suffix] == b[len(b)-1-suffix] {
		suffix++
	}
	if prefix == len(a) && prefix == len(b) {
		return nil
	}
	ops := make([]byte, 0, len(a)+len(b)-prefix-suffix)
	for range prefix {
		ops = append(ops, ' ')
	}
	middle, ok := lineEdits(a[prefix:len(a)-suffix], b[prefix:len(b)-suffix], maxDiffEdits)
	if !ok {
		middle = middle[:0]
		for range len(a) - prefix - suffix {
			middle = append(middle, '-')
		}
		for range len(b) - prefix - suffix {
			middle = append(middle, '+')
		}
	}
	ops = append(ops, middle...)
	for range suffix {
		ops = append(ops, ' ')
	}

	var hunks []string
	ai, bi := 0, 0 // a/b line positions before ops[i]
	for i := 0; i < len(ops); {
		if ops[i] == ' ' {
			ai, bi, i = ai+1, bi+1, i+1
			continue
		}
		// Extend the hunk while changes are separated by at most 2*context lines.
		end := i
		for j := i; j < len(ops); j++ {
			if ops[j] != ' ' {
				end = j + 1
			} else if j-end >= 2*diffContextLines {
				break
			}
		}
		lead := min(diffContextLines, i, ai, bi)
		start, aStart, bStart := i-lead, ai-lead, bi-lead
		stop := min(len(ops), end+diffContextLines)
		var body strings.Builder
		aCount, bCount := 0, 0
		for j := start; j < stop; j++ {
			var line string
			switch ops[j] {
			case ' ':
				line = a[aStart+aCount]
				aCount++
				bCount++
			case '-':
				line = a[aStart+aCount]
				aCount++
			default:
				line = b[bStart+bCount]
				bCount++
			}
			body.WriteByte(ops[j])
			body.WriteString(strings.TrimSuffix(line, "\n"))
			body.WriteByte('\n')
			if !strings.HasSuffix(line, "\n") {
				body.WriteString("\\ No newline at end of file\n")
			}
		}
		hunks = append(hunks, fmt.Sprintf("@@ -%d,%d +%d,%d @@\n", aStart+1, aCount, bStart+1, bCount)+body.String())
		ai, bi, i = aStart+aCount, bStart+bCount, stop
	}
	return hunks
}

// lineEdits returns a minimal Myers edit script (' ', '-', '+') or false
// when more than maxEdits insertions and deletions are required.
func lineEdits(a, b []string, maxEdits int) ([]byte, bool) {
	n, m := len(a), len(b)
	maxEdits = min(maxEdits, n+m)
	offset := maxEdits + 1
	v := make([]int, 2*maxEdits+3)
	var trace [][]int // trace[d] holds v[k] for k in [-d-1, d+1] before step d
	for d := 0; d <= maxEdits; d++ {
		trace = append(trace, append([]int(nil), v[offset-d-1:offset+d+2]...))
		for k := -d; k <= d; k += 2 {
			var x int
			if k == -d || (k != d && v[offset+k-1] < v[offset+k+1]) {
				x = v[offset+k+1]
			} else {
				x = v[offset+k-1] + 1
			}
			y := x - k
			for x < n && y < m && a[x] == b[y] {
				x, y = x+1, y+1
			}
			v[offset+k] = x
			if x >= n && y >= m {
				return backtrackEdits(trace, n, m), true
			}
		}
	}
	return nil, false
}

func backtrackEdits(trace [][]int, x, y int) []byte {
	var reversed []byte
	for d := len(trace) - 1; d >= 0; d-- {
		at := func(k int) int { return trace[d][k+d+1] }
		k := x - y
		prevK := k - 1
		if k == -d || (k != d && at(k-1) < at(k+1)) {
			prevK = k + 1
		}
		prevX := at(prevK)
		prevY := prevX - prevK
		for x > prevX && y > prevY {
			reversed = append(reversed, ' ')
			x, y = x-1, y-1
		}
		if d > 0 {
			if x == prevX {
				reversed = append(reversed, '+')
			} else {
				reversed = append(reversed, '-')
			}
		}
		x, y = prevX, prevY
	}
	for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
		reversed[i], reversed[j] = reversed[j], reversed[i]
	}
	return reversed
}
