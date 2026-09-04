package store

import (
	"fmt"
	"strings"
)

func validateStrings(field string, values []string, required bool) error {
	if required && len(values) == 0 {
		return invalid(field, "must not be empty")
	}
	for i, value := range values {
		if strings.TrimSpace(value) == "" {
			return invalid(fmt.Sprintf("%s[%d]", field, i), "must not be blank")
		}
	}
	return nil
}
