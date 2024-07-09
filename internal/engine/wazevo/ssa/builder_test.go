package ssa

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestBuilder_resolveAlias(t *testing.T) {
	b := NewBuilder().(*builder)
	v1 := b.allocateValue(TypeI32)
	v2 := b.allocateValue(TypeI32)
	v3 := b.allocateValue(TypeI32)
	v4 := b.allocateValue(TypeI32)
	v5 := b.allocateValue(TypeI32)

	b.alias(v1, v2)
	b.alias(v2, v3)
	b.alias(v3, v4)
	b.alias(v4, v5)
	require.Equal(t, v5, b.resolveAlias(v1))
	require.Equal(t, v5, b.resolveAlias(v2))
	require.Equal(t, v5, b.resolveAlias(v3))
	require.Equal(t, v5, b.resolveAlias(v4))
	require.Equal(t, v5, b.resolveAlias(v5))
}
