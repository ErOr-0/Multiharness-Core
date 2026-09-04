package store

import (
	"errors"
	"fmt"
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
