package binaryencoding

import (
	"bytes"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func Test_ensureElementKindFuncRef(t *testing.T) {
	require.NoError(t, ensureElementKindFuncRef(bytes.NewReader([]byte{0x0})))
	require.Error(t, ensureElementKindFuncRef(bytes.NewReader([]byte{0x1})))
}
