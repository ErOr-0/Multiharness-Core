package process

import (
	"reflect"
	"testing"
)

func TestMergeEnvironmentInheritsAndOverrides(t *testing.T) {
	base := []string{"ALPHA=one", "BETA=old", "BETA=new"}
	merged, err := mergeEnvironment(base, map[string]string{
		"BETA":  "override",
		"GAMMA": "three",
	})
	if err != nil {
		t.Fatalf("mergeEnvironment() returned an error: %v", err)
	}

	expected := []string{"ALPHA=one", "BETA=override", "GAMMA=three"}
	if !reflect.DeepEqual(merged, expected) {
		t.Fatalf("mergeEnvironment() = %#v; want %#v", merged, expected)
	}
}

func TestMergeEnvironmentRejectsInvalidOverrides(t *testing.T) {
	tests := []struct {
		name      string
		overrides map[string]string
	}{
		{name: "blank key", overrides: map[string]string{"": "value"}},
		{name: "key containing equals", overrides: map[string]string{"BAD=KEY": "value"}},
		{name: "key containing NUL", overrides: map[string]string{"BAD\x00KEY": "value"}},
		{name: "value containing NUL", overrides: map[string]string{"KEY": "bad\x00value"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := mergeEnvironment(nil, test.overrides); err == nil {
				t.Fatal("mergeEnvironment() returned nil; want an error")
			}
		})
	}
}

func TestRemoveEnvironment(t *testing.T) {
	base := []string{"KEEP=yes", "DROP=one", "DROP=two"}
	actual, err := removeEnvironment(base, []string{"DROP"})
	if err != nil || !reflect.DeepEqual(actual, []string{"KEEP=yes"}) || len(base) != 3 {
		t.Fatalf("remove: %v %v", actual, err)
	}
	if _, err := removeEnvironment(base, []string{"BAD=KEY"}); err == nil {
		t.Fatal("invalid removal accepted")
	}
}
