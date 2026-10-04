package console_test

import (
	"bytes"
	"os"
	"testing"

	"multiharness-core/internal/adapter/setup"
	"multiharness-core/internal/transport/cli/console"
)

func TestPipedYesIsNeverConsent(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	_, _ = writer.WriteString("yes\n")
	writer.Close()
	var output bytes.Buffer
	install := console.NewInstaller(reader, &output)
	if install == nil {
		return
	}
	yes, err := install(t.Context(), setup.Request{Tool: "codex", Command: "npm install"})
	if err != nil || yes || output.Len() != 0 {
		t.Fatal("non-interactive input authorized an installation")
	}
}
