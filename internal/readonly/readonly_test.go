package readonly

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

type mockCfg struct {
	readonly bool
}

func (m *mockCfg) ReadOnly() bool {
	return m.readonly
}

func TestEnforceWrite(t *testing.T) {
	cfg := &mockCfg{readonly: false}
	err := EnforceWrite(cfg)
	assert.NoError(t, err)

	cfg.readonly = true
	err = EnforceWrite(cfg)
	assert.Error(t, err)
	assert.Equal(t, ErrWriteInReadOnlyMode, err)
}

func TestCheckWrite(t *testing.T) {
	cfg := &mockCfg{readonly: false}
	_, err := CheckWrite[string](cfg)
	assert.NoError(t, err)

	cfg.readonly = true
	result, err := CheckWrite[string](cfg)
	assert.Error(t, err)
	assert.Equal(t, "", result)
}

func TestErrorMessage(t *testing.T) {
	assert.Contains(t, ErrWriteInReadOnlyMode.Error(), "readonly")
}

func TestIsReadOnlyError(t *testing.T) {
	assert.True(t, errors.Is(ErrWriteInReadOnlyMode, ErrWriteInReadOnlyMode))
}