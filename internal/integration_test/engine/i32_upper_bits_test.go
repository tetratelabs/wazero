package adhoc

import (
	"context"
	_ "embed"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/internal/platform"
)

//go:embed testdata/i32_upper_bits.wasm
var i32UpperBitsWasm []byte

// TestI32UpperBits verifies that i32 operations correctly ignore the upper 32 bits
// of uint64 values on the interpreter stack. The wasm spec requires all i32 operations
// to use only the lower 32 bits.
//
// For instance, this may happen when propagating values from an external call,
// if the caller does sanitize input: the compiler correctly truncates and
// ignores the "garbage" bits, the interpreter should do the same for consistency.
//
// This test covers:
//   - i32.eqz: was comparing full 64-bit value
//   - i32.ne: was comparing full 64-bit values (i32 shared code with i64)
//   - i32.lt_u, i32.gt_u, i32.le_u, i32.ge_u: unsigned comparisons used full 64 bits
//   - i32.div_u, i32.div_s, i32.rem_u, i32.rem_s: zero-divisor check used full 64 bits
//   - i32.load, i32.store: memory offset calculation used full 64 bits
func TestI32UpperBits(t *testing.T) {
	tests := []struct {
		name    string
		fn      string
		params  []uint64
		want    uint32
		wantErr string // if non-empty, expect this error
	}{
		// ---- i32.eqz ----
		// Bug: interpreter compared full 64-bit value to zero.
		// With dirty upper bits but lower 32 = 0, i32.eqz should return 1.
		{"eqz/clean_zero", "i32_eqz", []uint64{0}, 1, ""},
		{"eqz/clean_nonzero", "i32_eqz", []uint64{42}, 0, ""},
		{"eqz/dirty_zero", "i32_eqz", []uint64{0xDEADBEEF00000000}, 1, ""},
		{"eqz/dirty_nonzero", "i32_eqz", []uint64{0xDEADBEEF00000001}, 0, ""},

		// ---- i32.ne ----
		// Bug: interpreter compared full 64-bit values (i32 and i64 shared case).
		// Same lower 32 bits but different upper bits should be i32.ne = 0 (equal).
		{"ne/same_lower_diff_upper", "i32_ne", []uint64{0xDEADBEEF00000005, 0xCAFEBABE00000005}, 0, ""},
		{"ne/diff_lower_same_upper", "i32_ne", []uint64{0xDEADBEEF00000005, 0xDEADBEEF00000006}, 1, ""},
		{"ne/clean_equal", "i32_ne", []uint64{5, 5}, 0, ""},

		// ---- i32.eq (already correct, but verify) ----
		{"eq/same_lower_diff_upper", "i32_eq", []uint64{0xDEADBEEF00000005, 0xCAFEBABE00000005}, 1, ""},

		// ---- i32.lt_u ----
		// Bug: unsigned comparison used full 64 bits.
		// 5 < 10 should be true even when 5 has huge upper bits.
		{"lt_u/dirty_a_less", "i32_lt_u", []uint64{0xDEADBEEF00000005, 10}, 1, ""},
		{"lt_u/dirty_b_less", "i32_lt_u", []uint64{5, 0xCAFEBABE0000000A}, 1, ""},
		{"lt_u/dirty_both", "i32_lt_u", []uint64{0xDEADBEEF00000005, 0xCAFEBABE0000000A}, 1, ""},
		{"lt_u/dirty_not_less", "i32_lt_u", []uint64{0xDEADBEEF0000000A, 5}, 0, ""},

		// ---- i32.gt_u ----
		{"gt_u/dirty_a_greater", "i32_gt_u", []uint64{0xDEADBEEF0000000A, 5}, 1, ""},
		{"gt_u/dirty_a_less", "i32_gt_u", []uint64{0xDEADBEEF00000005, 10}, 0, ""},

		// ---- i32.le_u ----
		{"le_u/dirty_a_less", "i32_le_u", []uint64{0xDEADBEEF00000005, 10}, 1, ""},
		{"le_u/dirty_a_equal", "i32_le_u", []uint64{0xDEADBEEF0000000A, 10}, 1, ""},
		{"le_u/dirty_a_greater", "i32_le_u", []uint64{0xDEADBEEF0000000B, 10}, 0, ""},

		// ---- i32.ge_u ----
		{"ge_u/dirty_a_greater", "i32_ge_u", []uint64{0xDEADBEEF0000000A, 5}, 1, ""},
		{"ge_u/dirty_a_equal", "i32_ge_u", []uint64{0xDEADBEEF0000000A, 10}, 1, ""},
		{"ge_u/dirty_a_less", "i32_ge_u", []uint64{0xDEADBEEF00000005, 10}, 0, ""},

		// ---- i32.lt_s, i32.gt_s, i32.le_s, i32.ge_s (signed, already correct, but verify) ----
		{"lt_s/dirty_neg", "i32_lt_s", []uint64{0xDEADBEEF00000000 | uint64(^uint32(4)), 1}, 1, ""},
		{"gt_s/dirty_neg", "i32_gt_s", []uint64{0xDEADBEEF00000001, uint64(^uint32(4))}, 1, ""},
		{"le_s/dirty_neg", "i32_le_s", []uint64{0xDEADBEEF00000000 | uint64(^uint32(4)), uint64(^uint32(4))}, 1, ""},
		{"ge_s/dirty_neg", "i32_ge_s", []uint64{0xDEADBEEF00000000 | uint64(^uint32(4)), uint64(^uint32(4))}, 1, ""},

		// ---- i32.div_u ----
		// Bug: zero-divisor check used full 64 bits. A divisor with lower 32 = 0
		// but dirty upper bits would bypass the check and cause a Go-level panic.
		{"div_u/clean", "i32_div_u", []uint64{10, 3}, 3, ""},
		{"div_u/dirty_zero_divisor", "i32_div_u", []uint64{10, 0xDEADBEEF00000000}, 0, "integer divide by zero"},
		{"div_u/dirty_nonzero_divisor", "i32_div_u", []uint64{0xDEADBEEF0000000A, 0xCAFEBABE00000003}, 3, ""},

		// ---- i32.div_s ----
		{"div_s/dirty_zero_divisor", "i32_div_s", []uint64{10, 0xDEADBEEF00000000}, 0, "integer divide by zero"},

		// ---- i32.rem_u ----
		{"rem_u/clean", "i32_rem_u", []uint64{10, 3}, 1, ""},
		{"rem_u/dirty_zero_divisor", "i32_rem_u", []uint64{10, 0xDEADBEEF00000000}, 0, "integer divide by zero"},

		// ---- i32.rem_s ----
		{"rem_s/dirty_zero_divisor", "i32_rem_s", []uint64{10, 0xDEADBEEF00000000}, 0, "integer divide by zero"},

		// ---- i32.load (memory offset) ----
		// Bug: popMemoryOffset added full 64-bit value. A dirty address with
		// lower 32 = 0 should load from offset 0, not trigger out-of-bounds.
		{"load/dirty_addr_zero", "i32_load", []uint64{0xDEADBEEF00000000}, 0, ""},
		{"load/dirty_addr_valid", "i32_load", []uint64{0xDEADBEEF00000064}, 0, ""},

		// ---- i32.store + i32.load (memory offset) ----
		{"store_load/dirty_addr", "i32_store_load", []uint64{0xDEADBEEF000000C8, 0x12345678}, 0x12345678, ""},
	}

	for _, engine := range []struct {
		name   string
		config wazero.RuntimeConfig
	}{
		{"interpreter", wazero.NewRuntimeConfigInterpreter()},
		{"compiler", wazero.NewRuntimeConfigCompiler()},
	} {
		t.Run(engine.name, func(t *testing.T) {
			if engine.name == "compiler" && !platform.CompilerSupported() {
				t.Skip("Compiler is not supported on this host")
			}

			ctx := context.Background()
			r := wazero.NewRuntimeWithConfig(ctx, engine.config)
			defer r.Close(ctx)

			mod, err := r.Instantiate(ctx, i32UpperBitsWasm)
			if err != nil {
				t.Fatal(err)
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					fn := mod.ExportedFunction(tt.fn)
					results, err := fn.Call(ctx, tt.params...)

					if tt.wantErr != "" {
						if err == nil {
							t.Fatalf("expected error %q, got nil (result: %v)", tt.wantErr, results)
						}
						return
					}

					if err != nil {
						t.Fatalf("unexpected error: %v", err)
					}

					// Only compare lower 32 bits — the wasm spec allows
					// implementations to leave garbage in upper bits of i32 results.
					got := uint32(results[0])
					if got != tt.want {
						t.Errorf("got %d (full: 0x%016x), want %d (params: %x)", got, results[0], tt.want, tt.params)
					}
				})
			}
		})
	}
}

// TestI32UpperBitsNoDiff verifies that interpreter and compiler produce the same
// lower-32-bit results even when params have garbage upper bits.
func TestI32UpperBitsNoDiff(t *testing.T) {
	if !platform.CompilerSupported() {
		t.Skip("Compiler is not supported on this host")
	}

	ctx := context.Background()

	rInterp := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfigInterpreter())
	defer rInterp.Close(ctx)

	rComp := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfigCompiler())
	defer rComp.Close(ctx)

	modInterp, err := rInterp.Instantiate(ctx, i32UpperBitsWasm)
	if err != nil {
		t.Fatal(err)
	}

	modComp, err := rComp.Instantiate(ctx, i32UpperBitsWasm)
	if err != nil {
		t.Fatal(err)
	}

	paramSets := [][]uint64{
		{0xDEADBEEF00000000},                     // dirty zero
		{0xDEADBEEF0000002A},                     // dirty nonzero
		{0xDEADBEEF00000005, 10},                 // dirty first
		{5, 0xCAFEBABE0000000A},                  // dirty second
		{0xDEADBEEF00000005, 0xCAFEBABE00000005}, // dirty both, same lower
		{0xDEADBEEF00000005, 0xCAFEBABE0000000A}, // dirty both, diff lower
		{0xDEADBEEF0000000A, 3},                  // for div/rem
		{0xDEADBEEF00000000},                     // dirty zero address for load
		{0xDEADBEEF000000C8, 0x12345678},         // dirty address for store
	}

	fns := []string{
		"i32_eqz", "i32_eq", "i32_ne",
		"i32_lt_u", "i32_gt_u", "i32_le_u", "i32_ge_u",
		"i32_lt_s", "i32_gt_s", "i32_le_s", "i32_ge_s",
		"i32_div_u", "i32_rem_u",
		"i32_load", "i32_store_load",
	}

	for _, fnName := range fns {
		fnInterp := modInterp.ExportedFunction(fnName)
		fnComp := modComp.ExportedFunction(fnName)
		if fnInterp == nil || fnComp == nil {
			continue
		}
		paramCount := len(fnInterp.Definition().ParamTypes())

		for _, params := range paramSets {
			if len(params) != paramCount {
				continue
			}
			resI, errI := fnInterp.Call(ctx, params...)
			resC, errC := fnComp.Call(ctx, params...)

			// Both should error or both should succeed
			if (errI != nil) != (errC != nil) {
				t.Errorf("%s(%x): interpreter err=%v, compiler err=%v", fnName, params, errI, errC)
				continue
			}

			// Compare lower 32 bits of results
			if errI == nil && len(resI) > 0 && len(resC) > 0 {
				if uint32(resI[0]) != uint32(resC[0]) {
					t.Errorf("%s(%x): interpreter=%d (0x%x), compiler=%d (0x%x)",
						fnName, params, uint32(resI[0]), resI[0], uint32(resC[0]), resC[0])
				}
			}
		}
	}
}
