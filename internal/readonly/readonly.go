package readonly

import "fmt"

var (
	ErrWriteInReadOnlyMode = fmt.Errorf("operation not permitted in readonly mode")
)

type ReadOnlyChecker interface {
	ReadOnly() bool
}

func CheckWrite[T any](checker ReadOnlyChecker) (T, error) {
	var zero T
	if checker.ReadOnly() {
		return zero, ErrWriteInReadOnlyMode
	}
	return zero, nil
}

func EnforceWrite(checker ReadOnlyChecker) error {
	if checker.ReadOnly() {
		return ErrWriteInReadOnlyMode
	}
	return nil
}