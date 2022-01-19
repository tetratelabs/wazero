package binary

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEncodeStartSection(t *testing.T) {
	require.Equal(t, []byte{SectionIDStart, 0x01, 0x05}, encodeStartSection(5))
}
