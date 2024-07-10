package ssa

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestValue_InstructionID(t *testing.T) {
	v := Value(1234).setType(TypeI32).setInstructionID(5678)
	require.Equal(t, ValueID(1234), v.ID())
	require.Equal(t, 5678, v.instructionID())
	require.Equal(t, TypeI32, v.Type())
}
