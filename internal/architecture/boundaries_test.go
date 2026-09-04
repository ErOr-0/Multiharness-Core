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

func TestDependencyPolicy(t *testing.T) {
	for _, dependency := range []string{"os", "os/exec", "syscall", "net/http", "path/filepath", "golang.org/x/sys/unix", "multiharness-core/internal/adapter/process", "multiharness-core/internal/env", "multiharness-core/internal/config"} {
		if coreDependencyAllowed("multiharness-core/internal/workflow", dependency) {
			t.Errorf("accepted outer dependency %s", dependency)
		}
	}
	for _, dependency := range []string{"context", "errors", "fmt", "math/rand/v2", "time", "multiharness-core/internal/store"} {
		if !coreDependencyAllowed("multiharness-core/internal/workflow", dependency) {
			t.Errorf("rejected core dependency %s", dependency)
		}
	}
	if coreDependencyAllowed("multiharness-core/internal/store", "multiharness-core/internal/workflow") {
		t.Fatal("store depends on workflow policy")
	}
}
