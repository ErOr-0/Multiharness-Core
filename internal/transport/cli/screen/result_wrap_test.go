package screen

import (
	"strings"
	"testing"
)

func TestResultWrappingPreservesCodeSpacing(t *testing.T) {
	line := "func f() {  fmt.Println(\"a  b\")  }"
	parts := wrapResultLine(line, 12)
	if len(parts) < 2 || strings.Join(parts, "") != line {
		t.Fatalf("result line changed while wrapping: %#v", parts)
	}
}
