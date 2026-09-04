package structured

import (
	"fmt"

	"multiharness-core/internal/store"
)

func ImplementationSchema() []byte {
	return []byte(`{"type":"object","additionalProperties":false,"properties":{"schema_version":{"type":"string","enum":["1"]},"summary":{"type":"string"},"changed_files":{"type":"array","items":{"type":"string"}}},"required":["schema_version","summary","changed_files"]}`)
}

func ParseImplementation(data []byte) (store.ImplementationResult, error) {
	var response struct {
		SchemaVersion *string   `json:"schema_version"`
		Summary       *string   `json:"summary"`
		ChangedFiles  *[]string `json:"changed_files"`
	}
	if err := decodeStrict(data, &response); err != nil {
		return store.ImplementationResult{}, err
	}
	if response.SchemaVersion == nil || response.Summary == nil || response.ChangedFiles == nil {
		return store.ImplementationResult{}, fmt.Errorf("required implementation fields are missing or null")
	}
	if *response.SchemaVersion != "1" {
		return store.ImplementationResult{}, fmt.Errorf("unsupported schema_version %q", *response.SchemaVersion)
	}
	result := store.ImplementationResult{Summary: *response.Summary, ChangedFiles: *response.ChangedFiles}
	return result, result.Validate()
}
