package folder

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"
)

// Full replacement hunks provide deterministic review evidence in linear time.
// Binary changes carry content hashes; this report is not an executable patch.
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
		for _, side := range []struct {
			prefix string
			file   *fileState
		}{{"-", a}, {"+", b}} {
			if side.file == nil {
				continue
			}
			if side.file.mode&os.ModeSymlink != 0 {
				if err := write("symlink target:\n"); err != nil {
					return "", err
				}
			}
			text := string(side.file.data)
			for text != "" {
				line, rest, found := strings.Cut(text, "\n")
				if err := write(side.prefix + line + "\n"); err != nil {
					return "", err
				}
				if !found {
					if err := write("\\ No newline at end of file\n"); err != nil {
						return "", err
					}
				}
				text = rest
			}
		}
	}
	return output.String(), nil
}
