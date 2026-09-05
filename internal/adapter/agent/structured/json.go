package structured

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"unicode/utf8"
)

// encoding/json v1 silently overwrites duplicate keys, matches struct fields
// case-insensitively and replaces invalid UTF-8. Agent decisions must be
// unambiguous and conform to our exact, lower-case versioned schema keys.
// This is not the configuration merge policy: optional/null fields are checked
// by each response contract, and arbitrary map keys are not part of these wires.
func decodeStrict(data []byte, target any) error {
	if err := validateJSON(data, true); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		// Unknown keys and malformed values may themselves contain secrets.
		return errors.New("structured JSON does not match the response schema")
	}
	return nil
}

// ValidateJSON rejects ambiguous keys, invalid UTF-8 and excessive nesting in
// provider envelopes. Unlike final responses, metadata may use arbitrary keys.
// String values remain opaque, including tool output containing JSON examples.
func ValidateJSON(data []byte) error {
	return validateJSON(data, false)
}

// ValidateObject additionally requires an object and exact spelling for the
// control fields consumed by a caller. Unknown metadata is allowed; its casing
// must not be mistaken for a case-insensitive alias of a control field.
func ValidateObject(data []byte, controlFields ...string) error {
	if err := ValidateJSON(data); err != nil {
		return err
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(data, &object) != nil || object == nil {
		return errors.New("expected a JSON object")
	}
	for key := range object {
		for _, field := range controlFields {
			if key != field && strings.EqualFold(key, field) {
				return errors.New("noncanonical JSON control field")
			}
		}
	}
	return nil
}

func validateJSON(data []byte, canonicalKeys bool) error {
	if !utf8.Valid(data) {
		return errors.New("structured response must be valid UTF-8")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := checkResponseJSON(decoder, 0, canonicalKeys); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return errors.New("expected one structured JSON document")
	}
	return nil
}

func checkResponseJSON(decoder *json.Decoder, depth int, canonicalKeys bool) error {
	if depth > 64 {
		return errors.New("structured JSON exceeds nesting limit")
	}
	token, err := decoder.Token()
	if err != nil {
		return errors.New("malformed structured JSON")
	}
	var closing json.Delim
	switch token {
	case json.Delim('{'):
		closing = '}'
		seen := make(map[string]bool)
		for decoder.More() {
			key, err := decoder.Token()
			name, ok := key.(string)
			if err != nil || !ok || seen[name] || (canonicalKeys && !responseKey(name)) {
				return errors.New("duplicate or noncanonical structured JSON key")
			}
			seen[name] = true
			if err := checkResponseJSON(decoder, depth+1, canonicalKeys); err != nil {
				return err
			}
		}
	case json.Delim('['):
		closing = ']'
		for decoder.More() {
			if err := checkResponseJSON(decoder, depth+1, canonicalKeys); err != nil {
				return err
			}
		}
	default:
		return nil
	}
	if token, err := decoder.Token(); err != nil || token != closing {
		return errors.New("malformed structured JSON")
	}
	return nil
}

func responseKey(name string) bool {
	if name == "" {
		return false
	}
	for _, char := range name {
		if char != '_' && (char < 'a' || char > 'z') {
			return false
		}
	}
	return true
}
