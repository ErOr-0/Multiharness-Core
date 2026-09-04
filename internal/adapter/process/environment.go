package process

import (
	"fmt"
	"runtime"
	"sort"
	"strings"
)

func mergeEnvironment(base []string, overrides map[string]string) ([]string, error) {
	merged := make([]string, 0, len(base)+len(overrides))
	positions := make(map[string]int, len(base)+len(overrides))

	for _, entry := range base {
		key, _, ok := strings.Cut(entry, "=")
		if !ok || key == "" {
			continue
		}
		canonical := canonicalEnvironmentKey(key)
		if position, exists := positions[canonical]; exists {
			merged[position] = entry
			continue
		}
		positions[canonical] = len(merged)
		merged = append(merged, entry)
	}

	keys := make([]string, 0, len(overrides))
	for key := range overrides {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		value := overrides[key]
		if err := validateEnvironmentEntry(key, value); err != nil {
			return nil, err
		}

		entry := key + "=" + value
		canonical := canonicalEnvironmentKey(key)
		if position, exists := positions[canonical]; exists {
			merged[position] = entry
			continue
		}
		positions[canonical] = len(merged)
		merged = append(merged, entry)
	}

	return merged, nil
}

func validateEnvironmentEntry(key, value string) error {
	if key == "" || strings.ContainsAny(key, "=\x00") {
		return fmt.Errorf("invalid environment key %q", key)
	}
	if strings.ContainsRune(value, '\x00') {
		return fmt.Errorf("environment value for %q contains NUL", key)
	}
	return nil
}

func canonicalEnvironmentKey(key string) string {
	if runtime.GOOS == "windows" {
		return strings.ToUpper(key)
	}
	return key
}

func removeEnvironment(base, unset []string) ([]string, error) {
	removed := make(map[string]bool, len(unset))
	for _, key := range unset {
		if err := validateEnvironmentEntry(key, ""); err != nil {
			return nil, err
		}
		removed[canonicalEnvironmentKey(key)] = true
	}
	result := make([]string, 0, len(base))
	for _, entry := range base {
		key, _, _ := strings.Cut(entry, "=")
		if !removed[canonicalEnvironmentKey(key)] {
			result = append(result, entry)
		}
	}
	return result, nil
}
