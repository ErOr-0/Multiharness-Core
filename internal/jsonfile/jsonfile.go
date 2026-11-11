// Package jsonfile reads and writes small JSON files. A write replaces the
// file in one step.
package jsonfile

import (
	"encoding/json"
	"os"
)

// Write replaces path in one step so readers never see a partial file.
func Write(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func Read(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}
