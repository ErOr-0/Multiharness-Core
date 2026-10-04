package folder

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"multiharness-core/internal/contract"
)

func TestUnstableCaptureReportsExactChangedPaths(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "source.txt")
	if err := os.WriteFile(file, []byte("before"), 0600); err != nil {
		t.Fatal(err)
	}
	w, err := NewWorkspace(Config{Observe: func(p ScanProgress) {
		if p.Phase == "listing files (pass 2/2)" && p.Files == 0 {
			if err := os.WriteFile(file, []byte("after"), 0600); err != nil {
				t.Fatal(err)
			}
		}
	}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.stableCapture(t.Context(), dir, nil)
	var changed *contract.WorkspaceChangedError
	if !errors.Is(err, ErrChangedDuringCapture) || !errors.As(err, &changed) || !reflect.DeepEqual(changed.Files, []string{"source.txt"}) {
		t.Fatalf("missing capture diagnostics: %v", err)
	}
}
