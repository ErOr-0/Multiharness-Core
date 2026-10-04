// Package architecture protects dependency rules with production build metadata.
package architecture_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestProductionDependencyBoundaries(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, "go", "list", "-mod=readonly", "-deps", "-json", "./cmd/multiharness")
	command.Dir = "../.."
	data, err := command.Output()
	if err != nil {
		t.Fatalf("inspect production import graph: %v", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	corePackages := 0
	for {
		var pkg struct {
			ImportPath string
			Imports    []string
		}
		if err := decoder.Decode(&pkg); err == io.EOF {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		if pkg.ImportPath == "testing" || strings.HasPrefix(pkg.ImportPath, "github.com/cucumber/") || strings.Contains(pkg.ImportPath, "genkit") {
			t.Errorf("test or removed runtime dependency in production: %s", pkg.ImportPath)
		}
		for _, dependency := range pkg.Imports {
			if !moduleDependencyAllowed(pkg.ImportPath, dependency) {
				t.Errorf("module boundary violation: %s imports %s", pkg.ImportPath, dependency)
			}
		}
		if pkg.ImportPath != "multiharness-core/internal/delegation" && pkg.ImportPath != "multiharness-core/internal/workflow" && pkg.ImportPath != "multiharness-core/internal/contract" {
			continue
		}
		corePackages++
		for _, dependency := range pkg.Imports {
			if !coreDependencyAllowed(pkg.ImportPath, dependency) {
				t.Errorf("%s imports outer/OS dependency %s", pkg.ImportPath, dependency)
			}
		}
	}
	if corePackages != 3 {
		t.Fatal("production graph did not include all three core packages")
	}
}

// Composition owns concrete agent selection. Adapters cannot reach back into
// configuration, saved history or transport, and the CLI cannot construct an
// agent itself. History persists contracts and depends on nothing else here.
func moduleDependencyAllowed(source, dependency string) bool {
	const root = "multiharness-core/internal/"
	if strings.HasPrefix(source, root+"adapter/") {
		return dependency != root+"config" && dependency != root+"history" && !strings.HasPrefix(dependency, root+"transport/")
	}
	if source == root+"history" {
		return !strings.HasPrefix(dependency, root) || dependency == root+"contract"
	}
	if sourceLayer, ok := cliLayer(source); ok {
		if dependencyLayer, ok := cliLayer(dependency); ok && dependencyLayer >= sourceLayer {
			return false
		}
	}
	if strings.HasPrefix(source, root+"transport/") {
		return dependency != root+"adapter/agent/directexec" && dependency != root+"adapter/agent/schemaexec" && dependency != root+"adapter/agent/sessionexec"
	}
	return true
}

// The terminal transport is layered: primitives and prompts at the bottom,
// then rendering, live progress, terminal input, and the command handler on
// top. A package may import only layers below its own.
func cliLayer(path string) (int, bool) {
	const cli = "multiharness-core/internal/transport/cli"
	layer, ok := map[string]int{
		cli + "/term": 0, cli + "/approval": 0, cli + "/screen": 1,
		cli + "/progress": 2, cli + "/console": 3, cli: 4,
	}[path]
	return layer, ok
}

func coreDependencyAllowed(source, dependency string) bool {
	if strings.Contains(strings.Split(dependency, "/")[0], ".") || strings.HasPrefix(dependency, "multiharness-core/") {
		return (source == "multiharness-core/internal/workflow" || source == "multiharness-core/internal/delegation") && dependency == "multiharness-core/internal/contract"
	}
	for _, prefix := range []string{"os", "syscall", "net", "path/filepath", "plugin", "unsafe", "C"} {
		if dependency == prefix || strings.HasPrefix(dependency, prefix+"/") {
			return false
		}
	}
	return true
}
