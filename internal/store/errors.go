package store

import (
	"errors"
	"fmt"
	"strings"
)

// ContractValidationError identifies an invalid field in a store contract.
type ContractValidationError struct {
	Field   string
	Problem string
}

func (err *ContractValidationError) Error() string {
	return fmt.Sprintf("%s: %s", err.Field, err.Problem)
}

func invalid(field, problem string) *ContractValidationError {
	return &ContractValidationError{Field: field, Problem: problem}
}

func nested(prefix string, err error) *ContractValidationError {
	return invalid(prefix+"."+fieldOf(err), problemOf(err))
}

func fieldOf(err error) string {
	var validationErr *ContractValidationError
	if errors.As(err, &validationErr) {
		return validationErr.Field
	}
	return "contract"
}

func problemOf(err error) string {
	var validationErr *ContractValidationError
	if errors.As(err, &validationErr) {
		return validationErr.Problem
	}
	return err.Error()
}

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
