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
		if pkg.ImportPath != "multiharness-core/internal/workflow" && pkg.ImportPath != "multiharness-core/internal/store" {
			continue
		}
		corePackages++
		for _, dependency := range pkg.Imports {
			if !coreDependencyAllowed(pkg.ImportPath, dependency) {
				t.Errorf("%s imports outer/OS dependency %s", pkg.ImportPath, dependency)
			}
		}
	}
	if corePackages != 2 {
		t.Fatal("production graph did not include both core packages")
	}
}

// Composition owns concrete agent selection. Adapters cannot reach back into
// configuration or transport, and the CLI cannot construct an agent itself.
func moduleDependencyAllowed(source, dependency string) bool {
	const root = "multiharness-core/internal/"
	if strings.HasPrefix(source, root+"adapter/") {
		return dependency != root+"config" && !strings.HasPrefix(dependency, root+"transport/")
	}
	if strings.HasPrefix(source, root+"transport/") {
		return dependency != root+"adapter/agent/schemaexec" && dependency != root+"adapter/agent/sessionexec"
	}
	return true
}

func coreDependencyAllowed(source, dependency string) bool {
	if strings.Contains(strings.Split(dependency, "/")[0], ".") || strings.HasPrefix(dependency, "multiharness-core/") {
		return source == "multiharness-core/internal/workflow" && dependency == "multiharness-core/internal/store"
	}
	for _, prefix := range []string{"os", "syscall", "net", "path/filepath", "plugin", "unsafe", "C"} {
		if dependency == prefix || strings.HasPrefix(dependency, prefix+"/") {
			return false
		}
	}
	return true
}
