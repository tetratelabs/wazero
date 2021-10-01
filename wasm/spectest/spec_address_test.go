package spectest

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mathetake/gasm/wasm"
)

func Test_address1(t *testing.T) {
	vm := requireInitVM(t, "address1", nil)

	assertReturnCases := []struct {
		fnName     string
		input, exp int32
	}{
		{fnName: "8u_good1", exp: 97},
		{fnName: "8u_good2", exp: 97},
		{fnName: "8u_good3", exp: 98},
		{fnName: "8u_good4", exp: 99},
		{fnName: "8u_good5", exp: 122},
		{fnName: "8s_good1", exp: 97},
		{fnName: "8s_good2", exp: 97},
		{fnName: "8s_good3", exp: 98},
		{fnName: "8s_good4", exp: 99},
		{fnName: "8s_good5", exp: 122},
		{fnName: "16u_good1", exp: 25185},
		{fnName: "16u_good2", exp: 25185},
		{fnName: "16u_good3", exp: 25442},
		{fnName: "16u_good4", exp: 25699},
		{fnName: "16u_good5", exp: 122},
		{fnName: "16s_good1", exp: 25185},
		{fnName: "16s_good2", exp: 25185},
		{fnName: "16s_good3", exp: 25442},
		{fnName: "16s_good4", exp: 25699},
		{fnName: "16s_good5", exp: 122},
		{fnName: "32_good1", exp: 1684234849},
		{fnName: "32_good2", exp: 1684234849},
		{fnName: "32_good3", exp: 1701077858},
		{fnName: "32_good4", exp: 1717920867},
		{fnName: "32_good5", exp: 122},

		{fnName: "8u_good1", input: 65507},
		{fnName: "8u_good2", input: 65507},
		{fnName: "8u_good3", input: 65507},
		{fnName: "8u_good4", input: 65507},
		{fnName: "8u_good5", input: 65507},
		{fnName: "8s_good1", input: 65507},
		{fnName: "8s_good2", input: 65507},
		{fnName: "8s_good3", input: 65507},
		{fnName: "8s_good4", input: 65507},
		{fnName: "8s_good5", input: 65507},
		{fnName: "16u_good1", input: 65507},
		{fnName: "16u_good2", input: 65507},
		{fnName: "16u_good3", input: 65507},
		{fnName: "16u_good4", input: 65507},
		{fnName: "16u_good5", input: 65507},
		{fnName: "16s_good1", input: 65507},
		{fnName: "16s_good2", input: 65507},
		{fnName: "16s_good3", input: 65507},
		{fnName: "16s_good4", input: 65507},
		{fnName: "16s_good5", input: 65507},
		{fnName: "32_good1", input: 65507},
		{fnName: "32_good2", input: 65507},
		{fnName: "32_good3", input: 65507},
		{fnName: "32_good4", input: 65507},
		{fnName: "32_good5", input: 65507},

		{fnName: "8u_good1", input: 65508},
		{fnName: "8u_good2", input: 65508},
		{fnName: "8u_good3", input: 65508},
		{fnName: "8u_good4", input: 65508},
		{fnName: "8u_good5", input: 65508},
		{fnName: "8s_good1", input: 65508},
		{fnName: "8s_good2", input: 65508},
		{fnName: "8s_good3", input: 65508},
		{fnName: "8s_good4", input: 65508},
		{fnName: "8s_good5", input: 65508},
		{fnName: "16u_good1", input: 65508},
		{fnName: "16u_good2", input: 65508},
		{fnName: "16u_good3", input: 65508},
		{fnName: "16u_good4", input: 65508},
		{fnName: "16u_good5", input: 65508},
		{fnName: "16s_good1", input: 65508},
		{fnName: "16s_good2", input: 65508},
		{fnName: "16s_good3", input: 65508},
		{fnName: "16s_good4", input: 65508},
		{fnName: "16s_good5", input: 65508},
		{fnName: "32_good1", input: 65508},
		{fnName: "32_good2", input: 65508},
		{fnName: "32_good3", input: 65508},
		{fnName: "32_good4", input: 65508},
	}

	for _, c := range assertReturnCases {
		t.Run("assert_return_"+c.fnName, func(t *testing.T) {
			values, types, err := vm.ExecExportedFunction(c.fnName, uint64(uint32(c.input)))
			require.NoError(t, err)
			require.Len(t, values, 1)
			require.Len(t, types, 1)
			require.Equal(t, types[0], wasm.ValueTypeI32)
			require.Equal(t, c.exp, int32(uint32(values[0])))
		})
	}

	trapCases := []struct {
		fnName string
		input  int32
	}{
		{fnName: "32_good5", input: 65508},
		{fnName: "8u_good3", input: -1},
		{fnName: "8s_good3", input: -1},
		{fnName: "16u_good3", input: -1},
		{fnName: "16s_good3", input: -1},
		{fnName: "32_good3", input: -1},
		{fnName: "8u_bad", input: 0},
		{fnName: "8s_bad", input: 0},
		{fnName: "16u_bad", input: 0},
		{fnName: "16s_bad", input: 0},
		{fnName: "32_bad", input: 0},
		{fnName: "8u_bad", input: 1},
		{fnName: "8s_bad", input: 1},
		{fnName: "16u_bad", input: 1},
		{fnName: "16s_bad", input: 1},
		{fnName: "32_bad", input: 1},
	}

	for _, c := range trapCases {
		t.Run("assert_trap_"+c.fnName, func(t *testing.T) {
			require.Panics(t, func() {
				// Memory out of bounds.
				_, _, _ = vm.ExecExportedFunction(c.fnName, uint64(uint32(c.input)))
			})
		})
	}
}

func Test_address2(t *testing.T) {
	vm := requireInitVM(t, "address2", nil)
	assertReturnCases := []struct {
		fnName string
		input  int32
		exp    int64
	}{

		{fnName: "8u_good1", exp: 97},
		{fnName: "8u_good2", exp: 97},
		{fnName: "8u_good3", exp: 98},
		{fnName: "8u_good4", exp: 99},
		{fnName: "8u_good5", exp: 122},
		{fnName: "8s_good1", exp: 97},
		{fnName: "8s_good2", exp: 97},
		{fnName: "8s_good3", exp: 98},
		{fnName: "8s_good4", exp: 99},
		{fnName: "8s_good5", exp: 122},
		{fnName: "16u_good1", exp: 25185},
		{fnName: "16u_good2", exp: 25185},
		{fnName: "16u_good3", exp: 25442},
		{fnName: "16u_good4", exp: 25699},
		{fnName: "16u_good5", exp: 122},
		{fnName: "16s_good1", exp: 25185},
		{fnName: "16s_good2", exp: 25185},
		{fnName: "16s_good3", exp: 25442},
		{fnName: "16s_good4", exp: 25699},
		{fnName: "16s_good5", exp: 122},
		{fnName: "32u_good1", exp: 1684234849},
		{fnName: "32u_good2", exp: 1684234849},
		{fnName: "32u_good3", exp: 1701077858},
		{fnName: "32u_good4", exp: 1717920867},
		{fnName: "32u_good5", exp: 122},
		{fnName: "32s_good1", exp: 1684234849},
		{fnName: "32s_good2", exp: 1684234849},
		{fnName: "32s_good3", exp: 1701077858},
		{fnName: "32s_good4", exp: 1717920867},
		{fnName: "32s_good5", exp: 122},
		{fnName: "64_good1", exp: 0x6867666564636261},
		{fnName: "64_good2", exp: 0x6867666564636261},
		{fnName: "64_good3", exp: 0x6968676665646362},
		{fnName: "64_good4", exp: 0x6a69686766656463},
		{fnName: "64_good5", exp: 122},
		{fnName: "8u_good1", input: 65503},
		{fnName: "8u_good2", input: 65503},
		{fnName: "8u_good3", input: 65503},
		{fnName: "8u_good4", input: 65503},
		{fnName: "8u_good5", input: 65503},
		{fnName: "8s_good1", input: 65503},
		{fnName: "8s_good2", input: 65503},
		{fnName: "8s_good3", input: 65503},
		{fnName: "8s_good4", input: 65503},
		{fnName: "8s_good5", input: 65503},
		{fnName: "16u_good1", input: 65503},
		{fnName: "16u_good2", input: 65503},
		{fnName: "16u_good3", input: 65503},
		{fnName: "16u_good4", input: 65503},
		{fnName: "16u_good5", input: 65503},
		{fnName: "16s_good1", input: 65503},
		{fnName: "16s_good2", input: 65503},
		{fnName: "16s_good3", input: 65503},
		{fnName: "16s_good4", input: 65503},
		{fnName: "16s_good5", input: 65503},
		{fnName: "32u_good1", input: 65503},
		{fnName: "32u_good2", input: 65503},
		{fnName: "32u_good3", input: 65503},
		{fnName: "32u_good4", input: 65503},
		{fnName: "32u_good5", input: 65503},
		{fnName: "32s_good1", input: 65503},
		{fnName: "32s_good2", input: 65503},
		{fnName: "32s_good3", input: 65503},
		{fnName: "32s_good4", input: 65503},
		{fnName: "32s_good5", input: 65503},
		{fnName: "64_good1", input: 65503},
		{fnName: "64_good2", input: 65503},
		{fnName: "64_good3", input: 65503},
		{fnName: "64_good4", input: 65503},
		{fnName: "64_good5", input: 65503},
		{fnName: "8u_good1", input: 65504},
		{fnName: "8u_good2", input: 65504},
		{fnName: "8u_good3", input: 65504},
		{fnName: "8u_good4", input: 65504},
		{fnName: "8u_good5", input: 65504},
		{fnName: "8s_good1", input: 65504},
		{fnName: "8s_good2", input: 65504},
		{fnName: "8s_good3", input: 65504},
		{fnName: "8s_good4", input: 65504},
		{fnName: "8s_good5", input: 65504},
		{fnName: "16u_good1", input: 65504},
		{fnName: "16u_good2", input: 65504},
		{fnName: "16u_good3", input: 65504},
		{fnName: "16u_good4", input: 65504},
		{fnName: "16u_good5", input: 65504},
		{fnName: "16s_good1", input: 65504},
		{fnName: "16s_good2", input: 65504},
		{fnName: "16s_good3", input: 65504},
		{fnName: "16s_good4", input: 65504},
		{fnName: "16s_good5", input: 65504},
		{fnName: "32u_good1", input: 65504},
		{fnName: "32u_good2", input: 65504},
		{fnName: "32u_good3", input: 65504},
		{fnName: "32u_good4", input: 65504},
		{fnName: "32u_good5", input: 65504},
		{fnName: "32s_good1", input: 65504},
		{fnName: "32s_good2", input: 65504},
		{fnName: "32s_good3", input: 65504},
		{fnName: "32s_good4", input: 65504},
		{fnName: "32s_good5", input: 65504},
		{fnName: "64_good1", input: 65504},
		{fnName: "64_good2", input: 65504},
		{fnName: "64_good3", input: 65504},
		{fnName: "64_good4", input: 65504},
	}

	for _, c := range assertReturnCases {
		t.Run("assert_return_"+c.fnName, func(t *testing.T) {
			values, types, err := vm.ExecExportedFunction(c.fnName, uint64(uint32(c.input)))
			require.NoError(t, err)
			require.Len(t, values, 1)
			require.Len(t, types, 1)
			require.Equal(t, types[0], wasm.ValueTypeI64)
			require.Equal(t, c.exp, int64(values[0]))
		})
	}

	trapCases := []struct {
		fnName string
		input  int32
	}{
		{fnName: "8u_good3", input: -1},
		{fnName: "8s_good3", input: -1},
		{fnName: "16u_good3", input: -1},
		{fnName: "16s_good3", input: -1},
		{fnName: "32u_good3", input: -1},
		{fnName: "32s_good3", input: -1},
		{fnName: "64_good3", input: -1},
		{fnName: "8u_bad"},
		{fnName: "8s_bad"},
		{fnName: "16u_bad"},
		{fnName: "16s_bad"},
		{fnName: "32u_bad"},
		{fnName: "32s_bad"},
		{fnName: "64_bad"},
		{fnName: "8u_bad", input: 1},
		{fnName: "8s_bad", input: 1},
		{fnName: "16u_bad", input: 1},
		{fnName: "16s_bad", input: 1},
		{fnName: "32u_bad", input: 0},
		{fnName: "32s_bad", input: 0},
		{fnName: "64_bad", input: 1},
	}

	for _, c := range trapCases {
		t.Run("assert_trap_"+c.fnName, func(t *testing.T) {
			require.Panics(t, func() {
				// Memory out of bounds.
				_, _, _ = vm.ExecExportedFunction(c.fnName, uint64(uint32(c.input)))
			})
		})
	}
}

func Test_address3(t *testing.T) {
	vm := requireInitVM(t, "address3", nil)
	assertReturnCases := []struct {
		fnName string
		input  int32
		exp    float32
	}{
		{fnName: "32_good1"},
		{fnName: "32_good2"},
		{fnName: "32_good3"},
		{fnName: "32_good4"},
		{fnName: "32_good5", exp: float32(math.NaN())},
		{fnName: "32_good1", input: 65524},
		{fnName: "32_good2", input: 65524},
		{fnName: "32_good3", input: 65524},
		{fnName: "32_good4", input: 65524},
		{fnName: "32_good5", input: 65524},
		{fnName: "32_good1", input: 65525},
		{fnName: "32_good2", input: 65525},
		{fnName: "32_good3", input: 65525},
		{fnName: "32_good4", input: 65525},
	}

	for _, c := range assertReturnCases {
		t.Run("assert_return_"+c.fnName, func(t *testing.T) {
			values, types, err := vm.ExecExportedFunction(c.fnName, uint64(uint32(c.input)))
			require.NoError(t, err)
			require.Len(t, values, 1)
			require.Len(t, types, 1)
			require.Equal(t, types[0], wasm.ValueTypeF32)
			if math.IsNaN(float64(c.exp)) {
				require.True(t, math.IsNaN(float64(math.Float32frombits(uint32(values[0])))))
			}
		})
	}

	trapCases := []struct {
		fnName string
		input  int32
	}{
		{fnName: "32_good5", input: 65525},
		{fnName: "32_good3", input: -1},
		{fnName: "32_good3", input: -1},
		{fnName: "32_bad"},
		{fnName: "32_bad", input: 1},
	}

	for _, c := range trapCases {
		t.Run("assert_trap_"+c.fnName, func(t *testing.T) {
			require.Panics(t, func() {
				// Memory out of bounds.
				_, _, _ = vm.ExecExportedFunction(c.fnName, uint64(uint32(c.input)))
			})
		})
	}
}

func Test_address4(t *testing.T) {
	vm := requireInitVM(t, "address4", nil)
	assertReturnCases := []struct {
		fnName string
		input  int32
		exp    float64
	}{
		{fnName: "64_good1", exp: 0.0},
		{fnName: "64_good2", exp: 0.0},
		{fnName: "64_good3", exp: 0.0},
		{fnName: "64_good4", exp: 0.0},
		{fnName: "64_good5", exp: math.NaN()},
		{fnName: "64_good1", input: 65510},
		{fnName: "64_good2", input: 65510},
		{fnName: "64_good3", input: 65510},
		{fnName: "64_good4", input: 65510},
		{fnName: "64_good5", input: 65510},
		{fnName: "64_good1", input: 65511},
		{fnName: "64_good2", input: 65511},
		{fnName: "64_good3", input: 65511},
		{fnName: "64_good4", input: 65511},
	}

	for _, c := range assertReturnCases {
		t.Run("assert_return_"+c.fnName, func(t *testing.T) {
			values, types, err := vm.ExecExportedFunction(c.fnName, uint64(uint32(c.input)))
			require.NoError(t, err)
			require.Len(t, values, 1)
			require.Len(t, types, 1)
			require.Equal(t, types[0], wasm.ValueTypeF64)
			if math.IsNaN(float64(c.exp)) {
				require.True(t, math.IsNaN(float64(math.Float64frombits(values[0]))))
			}
		})
	}

	trapCases := []struct {
		fnName string
		input  int32
	}{
		{fnName: "64_good5", input: 65511},
		{fnName: "64_good3", input: -1},
		{fnName: "64_good3", input: -1},
		{fnName: "64_bad", input: 0},
		{fnName: "64_bad", input: 1},
	}

	for _, c := range trapCases {
		t.Run("assert_trap_"+c.fnName, func(t *testing.T) {
			require.Panics(t, func() {
				// Memory out of bounds.
				_, _, _ = vm.ExecExportedFunction(c.fnName, uint64(uint32(c.input)))
			})
		})
	}
}
