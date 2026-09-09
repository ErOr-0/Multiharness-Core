package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"multiharness-core/internal/config"
	"multiharness-core/internal/store"
)

const diagnosticFilename = "last-provider-failure.json"

// One bounded record, outside the workspace. Never persist the task, repository
// evidence, raw provider output, headers, credentials, or free-form error text.
type providerDiagnostic struct {
	Version  int                   `json:"version"`
	Time     string                `json:"time"`
	RunID    string                `json:"run_id"`
	Stage    store.WorkflowStage   `json:"stage"`
	Provider store.ProviderFailure `json:"provider"`
}

var diagnosticRunID = regexp.MustCompile(`^run_[A-Za-z0-9]{1,64}$`)

func (d providerDiagnostic) validate() error {
	if d.Version != 1 || !diagnosticRunID.MatchString(d.RunID) || (store.TaskFailure{Stage: d.Stage, Code: store.FailureCodeAgent, Message: "diagnostic"}).Validate() != nil {
		return errors.New("invalid provider diagnostic metadata")
	}
	if _, err := time.Parse(time.RFC3339Nano, d.Time); err != nil {
		return err
	}
	return d.Provider.Validate()
}

func saveProviderDiagnostic(directory, runID string, failure *store.TaskFailure) error {
	record := providerDiagnostic{Version: 1, Time: time.Now().UTC().Format(time.RFC3339Nano), RunID: runID, Stage: failure.Stage, Provider: *failure.Provider}
	if err := record.validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(directory, ".provider-diagnostic-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, writeErr := file.Write(append(data, '\n'))
	if err := errors.Join(writeErr, file.Close()); err != nil {
		return err
	}
	return os.Rename(file.Name(), filepath.Join(directory, diagnosticFilename))
}

func (v *interactiveView) diagnostics(directory string) error {
	data, err := config.ReadFile(filepath.Join(directory, diagnosticFilename), 8192)
	if errors.Is(err, os.ErrNotExist) {
		return v.notice("No saved provider failure diagnostics.", false)
	}
	if err != nil {
		return v.notice("Could not read saved provider diagnostics.", true)
	}
	var record providerDiagnostic
	if json.Unmarshal(data, &record) != nil || record.validate() != nil {
		return v.notice("Saved provider diagnostics are invalid.", true)
	}
	return v.notice(fmt.Sprintf("Last provider failure\nTime: %s\nRun: %s\nStage: %s\n%s", record.Time, record.RunID, record.Stage, record.Provider.Error()), false)
}
