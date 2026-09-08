package launcher

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestConfigurationCanBeCancelledWhileWaitingForInput(t *testing.T) {
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	ctx, cancel := context.WithCancel(t.Context())
	var out bytes.Buffer
	a := &app{in: reader, out: &out, stderr: &out}
	done := make(chan error, 1)
	go func() { done <- a.configure(ctx) }()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("configuration ignored cancellation")
	}
}

func TestFolderChangeIsSavedOnHostAndChangesOnlyDockerMount(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	var out bytes.Buffer
	a := &app{in: strings.NewReader("1\n" + first + "\n1\n" + second + "\n0\n"), out: &out, stderr: &out, directory: t.TempDir()}
	if err := a.configure(t.Context()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(a.directory, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte("folder")) {
		t.Fatal("folder setting missing")
	}
	resolved, _ := hostFolder(second)
	if a.settings.Folder != resolved {
		t.Fatal(a.settings.Folder)
	}
	args := runArgs(resolved, "policy.json", false, false, []string{"configure"})
	if !strings.Contains(strings.Join(args, "\n"), "type=bind,src="+resolved+",dst=/workspace") {
		t.Fatal(args)
	}
	if !reflect.DeepEqual(args[len(args)-2:], []string{image, "configure"}) {
		t.Fatal(args)
	}
	for _, folder := range []string{first, second} {
		entries, err := os.ReadDir(folder)
		if err != nil || len(entries) != 0 {
			t.Fatal("configuration changed project files", err)
		}
	}
}

func TestHostFolderRejectsMissingRelativeAndDockerDelimiterPaths(t *testing.T) {
	for _, path := range []string{"relative", filepath.Join(t.TempDir(), "missing"), filepath.Join(t.TempDir(), "a,b")} {
		if _, err := hostFolder(path); err == nil {
			t.Fatal("accepted", path)
		}
	}
}

func TestInputDoesNotConsumeProviderLoginInput(t *testing.T) {
	r := strings.NewReader("3\nsecret-next-line\n")
	got, err := readLine(r)
	if err != nil || got != "3" {
		t.Fatal(got, err)
	}
	if r.Len() != len("secret-next-line\n") {
		t.Fatal("read ahead into provider input")
	}
}
