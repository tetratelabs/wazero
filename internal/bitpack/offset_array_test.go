package bitpack_test

import (
	"fmt"
	"math"
	"testing"

	"github.com/tetratelabs/wazero/internal/bitpack"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestOffsetArray(t *testing.T) {
	tests := [][]uint64{
		{},
		{0},
		{1, 2, 3, 4, 5, 6, 7, 8, 9},
		{16: 1},
		{17: math.MaxUint16 + 1},
		{21: 10, 22: math.MaxUint16},
		{0: 42, 100: math.MaxUint64},
		{0: 42, 1: math.MaxUint32, 101: math.MaxUint64},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("len=%d", len(test)), func(t *testing.T) {
			array := bitpack.NewOffsetArray(test)
			require.Equal(t, len(test), array.Len())

			for i, v := range test {
				require.Equal(t, v, array.Index(i))
			}
		})
	}
}
