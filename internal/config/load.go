package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const MaxConfigBytes = 1 << 20

// Load reads an explicitly selected file (if any). Empty environment values
// are real overrides, not an instruction to fall back to a lower layer.
func Load(filename, baseDir string, lookupEnv func(string) (string, bool), overrides map[string]string) (Config, error) {
	c := Defaults()
	if filename != "" {
		if !filepath.IsAbs(filename) {
			filename = filepath.Join(baseDir, filename)
		}
		data, err := ReadFile(filename, MaxConfigBytes)
		if err != nil {
			return Config{}, fmt.Errorf("read config: %w", err)
		}
		if err := decodeStrict(data, &c); err != nil {
			return Config{}, fmt.Errorf("config file: %w", err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(data, &fields); err != nil || fields["version"] == nil {
			return Config{}, fmt.Errorf("config file must declare version 1")
		}
		if c.Version != 1 {
			return Config{}, fmt.Errorf("unsupported configuration version (expected 1)")
		}
	}
	known := map[string]bool{}
	for _, option := range Options() {
		known[option.Name] = true
		value, supplied := overrides[option.Name]
		if !supplied && lookupEnv != nil {
			value, supplied = lookupEnv(option.Environment())
		}
		if supplied {
			if err := apply(&c, option, value); err != nil {
				return Config{}, fmt.Errorf("setting %s: %w", option.Name, err)
			}
		}
	}
	for name := range overrides {
		if !known[name] {
			return Config{}, fmt.Errorf("unknown setting %q", name)
		}
	}
	if err := c.Validate(); err != nil {
		return Config{}, err
	}
	if !filepath.IsAbs(baseDir) {
		return Config{}, fmt.Errorf("configuration base directory must be absolute")
	}
	c.ResolvePaths(baseDir)
	return c, nil
}

func apply(c *Config, option Option, value string) error {
	var raw json.RawMessage
	if option.JSON {
		raw = json.RawMessage(value)
	} else {
		encoded, err := json.Marshal(value)
		if err != nil {
			return err
		}
		raw = encoded
	}
	var node any = raw
	parts := strings.Split(option.Path, ".")
	for i := len(parts) - 1; i >= 0; i-- {
		node = map[string]any{parts[i]: node}
	}
	data, err := json.Marshal(node)
	if err != nil {
		return fmt.Errorf("expected a valid JSON value")
	}
	return decodeStrict(data, c)
}

// ReadFile bounds config/task input and rejects directories and named pipes.
func ReadFile(filename string, limit int) ([]byte, error) {
	if limit <= 0 || int64(limit) == math.MaxInt64 {
		return nil, fmt.Errorf("input byte limit must be positive")
	}
	info, err := os.Stat(filename)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("input must be a regular file")
	}
	if info.Size() > int64(limit) {
		return nil, fmt.Errorf("input exceeds %d bytes", limit)
	}
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, int64(limit)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > limit {
		return nil, fmt.Errorf("input exceeds %d bytes", limit)
	}
	return data, nil
}

func decodeStrict(data []byte, target any) error {
	if !utf8.Valid(data) {
		return fmt.Errorf("configuration must be valid UTF-8")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := checkJSON(decoder); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return fmt.Errorf("expected one JSON document")
	}
	decoder = json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

// Reject duplicate keys and null: encoding/json otherwise silently overwrites
// values or treats null as a no-op when merging into an existing struct.
func checkJSON(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if token == nil {
		return fmt.Errorf("null configuration values are not allowed")
	}
	switch token {
	case json.Delim('{'):
		seen := map[string]bool{}
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			canonical := strings.ToLower(name)
			if !ok || seen[canonical] {
				return fmt.Errorf("invalid or duplicate JSON key")
			}
			seen[canonical] = true
			if err := checkJSON(decoder); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
		return err
	case json.Delim('['):
		for decoder.More() {
			if err := checkJSON(decoder); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
		return err
	}
	return nil
}
