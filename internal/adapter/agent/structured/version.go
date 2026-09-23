package structured

import (
	"encoding/json"
	"errors"
)

// schemaVersion tolerates the integer spelling of our existing wire versions.
// Native agents are still asked to emit canonical strings. This compatibility
// applies only to version metadata, never to decisions, findings or evidence.
type schemaVersion string

func (v *schemaVersion) UnmarshalJSON(data []byte) error {
	if string(data) == "1" || string(data) == "2" || string(data) == "3" {
		*v = schemaVersion(data)
		return nil
	}
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return errors.New("schema_version must be a version string or supported integer")
	}
	*v = schemaVersion(value)
	return nil
}
