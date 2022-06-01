package internal

import (
	"math"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

var maxUint32 = uint32(math.MaxUint32)

func TestDecodeUint32(t *testing.T) {
	for _, tt := range []struct {
		name, input      string
		expected         uint32
		expectedOverflow bool
	}{
		{name: "zero", input: "0", expected: 0},
		{name: "largest uint16", input: "65535", expected: 0xffff},
		{name: "largest uint16 with underscores", input: "6_5_5_3_5", expected: 0xffff},
		{name: "under largest uint32 by factor of 10", input: "429496729", expected: 429496729},
		{name: "under largest uint32 by factor of 10 with underscores", input: "4_2_9_4_9_6_7_2_9", expected: 429496729},
		{name: "largest uint32", input: "4294967295", expected: maxUint32},
		{name: "largest uint32 with underscores", input: "4_2_9_4_9_6_7_2_9_5", expected: maxUint32},
		{name: "overflow by one", input: "4294967296", expectedOverflow: true},
		{name: "overflow by one with underscores", input: "4_2_9_4_9_6_7_2_9_6", expectedOverflow: true},
		{name: "overflow by factor of 10", input: "42949672950", expectedOverflow: true},
		{name: "overflow by factor of 10 with underscores", input: "4_2_9_4_9_6_7_2_9_5_0", expectedOverflow: true},
	} {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			actual, overflow := decodeUint32([]byte(tc.input))
			require.Equal(t, tc.expected, actual)
			require.Equal(t, tc.expectedOverflow, overflow)
		})
	}
}

func TestDecodeUint64(t *testing.T) {
	for _, tt := range []struct {
		name, input      string
		expected         uint64
		expectedOverflow bool
	}{
		{name: "zero", input: "0", expected: 0},
		{name: "largest uint32", input: "4294967295", expected: uint64(maxUint32)},
		{name: "under largest uint64 by factor of 10", input: "1844674407370955161", expected: 1844674407370955161},
		{name: "under largest uint64 by factor of 10 with underscores", input: "1_8_4_4_6_7_4_4_0_7_3_7_0_9_5_5_1_6_1", expected: 1844674407370955161},
		{name: "largest uint64", input: "18446744073709551615", expected: 0xffffffffffffffff},
		{name: "largest uint64 with underscores", input: "1_8_4_4_6_7_4_4_0_7_3_7_0_9_5_5_1_6_1_5", expected: 0xffffffffffffffff},
		{name: "overflow by one", input: "18446744073709551616", expectedOverflow: true},
		{name: "overflow by one with underscores", input: "1_8_4_4_6_7_4_4_0_7_3_7_0_9_5_5_1_6_1_6", expectedOverflow: true},
		{name: "overflow by factor of 10", input: "184467440737095516150", expectedOverflow: true},
		{name: "overflow by factor of 10 with underscores", input: "1_8_4_4_6_7_4_4_0_7_3_7_0_9_5_5_1_6_1_5_0", expectedOverflow: true},
	} {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			actual, overflow := decodeUint64([]byte(tc.input))
			require.Equal(t, tc.expected, actual)
			require.Equal(t, tc.expectedOverflow, overflow)
		})
	}
}
