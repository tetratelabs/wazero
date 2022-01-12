package wat

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeUint32(t *testing.T) {
	for _, tt := range []struct {
		name, input string
		expected    uint32
		expectedErr bool
	}{
		{name: "zero", input: "0", expected: 0},
		{name: "largest uint16", input: "65535", expected: 0xffff},
		{name: "largest uint32", input: "4294967295", expected: 0xffffffff},
		{name: "largest uint32 with underscores", input: "4_2_9_4_9_6_7_2_9_5", expected: 0xffffffff},
		{name: "overflow by one", input: "4294967296", expectedErr: true},
		{name: "overflow by one with underscores", input: "4_2_9_4_9_6_7_2_9_6", expectedErr: true},
		{name: "overflow by pow", input: "42949672950", expectedErr: true},
		{name: "overflow by pow with underscores", input: "4_2_9_4_9_6_7_2_9_5_0", expectedErr: true},
	} {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			actual, err := decodeUint32([]byte(tc.input))
			if tc.expectedErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expected, actual)
			}
		})
	}
}

func TestDecodeUint64(t *testing.T) {
	for _, tt := range []struct {
		name, input string
		expected    uint64
		expectedErr bool
	}{
		{name: "zero", input: "0", expected: 0},
		{name: "largest uint16", input: "65535", expected: 0xffff},
		{name: "largest uint32", input: "4294967295", expected: 0xffffffff},
		{name: "largest uint64", input: "18446744073709551615", expected: 0xffffffffffffffff},
		{name: "largest uint64 with underscores", input: "1_8_4_4_6_7_4_4_0_7_3_7_0_9_5_5_1_6_1_5", expected: 0xffffffffffffffff},
		{name: "overflow by one", input: "18446744073709551616", expectedErr: true},
		{name: "overflow by one with underscores", input: "1_8_4_4_6_7_4_4_0_7_3_7_0_9_5_5_1_6_1_6", expectedErr: true},
		{name: "overflow by pow", input: "184467440737095516150", expectedErr: true},
		{name: "overflow by pow with underscores", input: "1_8_4_4_6_7_4_4_0_7_3_7_0_9_5_5_1_6_1_5_0", expectedErr: true},
	} {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			actual, err := decodeUint64([]byte(tc.input))
			if tc.expectedErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expected, actual)
			}
		})
	}
}
