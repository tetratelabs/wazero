package bench

// What WithCloseOnContextDone costs a loop-heavy module: the same wasm, on a runtime with
// ensureTermination off and on. The check is compiled into every loop back-edge, so a
// module whose work IS a loop pays it on every iteration.
//
//	go test -bench BenchmarkEnsureTermination -benchtime 2s -count 5 ./internal/integration_test/bench/

import (
	"context"
	"testing"

	"github.com/tetratelabs/wazero"
)

// sumLoopWasm is a module whose only export is a counting loop, assembled here because
// this package has no wat2wasm. Section sizes are computed rather than hand-counted.
//
//	(module
//	  (func (export "sum") (param i32) (result i64) (local i64)
//	    (block
//	      (loop
//	        (br_if 1 (i32.eqz (local.get 0)))
//	        (local.set 1 (i64.add (local.get 1) (i64.extend_i32_u (local.get 0))))
//	        (local.set 0 (i32.sub (local.get 0) (i32.const 1)))
//	        (br 0)))
//	    (local.get 1)))
func sumLoopWasm() []byte {
	section := func(id byte, content ...byte) []byte {
		return append([]byte{id, byte(len(content))}, content...)
	}
	body := []byte{
		0x01, 0x01, 0x7e, // one i64 local: the accumulator
		0x02, 0x40, // block
		0x03, 0x40, // loop
		0x20, 0x00, 0x45, 0x0d, 0x01, // br_if 1 (i32.eqz (local.get 0))
		0x20, 0x01, 0x20, 0x00, 0xad, 0x7c, 0x21, 0x01, // acc += u64(n)
		0x20, 0x00, 0x41, 0x01, 0x6b, 0x21, 0x00, // n -= 1
		0x0c, 0x00, // br 0
		0x0b, 0x0b, // end loop, end block
		0x20, 0x01, // local.get 1
		0x0b, // end func
	}
	out := []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00} // magic + version
	out = append(out, section(1, 0x01, 0x60, 0x01, 0x7f, 0x01, 0x7e)...)         // type: (i32)->i64
	out = append(out, section(3, 0x01, 0x00)...)                                 // func 0: type 0
	out = append(out, section(7, 0x01, 0x03, 's', 'u', 'm', 0x00, 0x00)...)      // export "sum"
	return append(out, section(10, append([]byte{0x01, byte(len(body))}, body...)...)...)
}

func BenchmarkEnsureTermination(b *testing.B) {
	const iterations = 1 << 16
	wasmBytes := sumLoopWasm()
	for _, tc := range []struct {
		name              string
		ensureTermination bool
	}{{"off", false}, {"on", true}} {
		b.Run(tc.name, func(b *testing.B) {
			ctx := context.Background()
			r := wazero.NewRuntimeWithConfig(ctx,
				wazero.NewRuntimeConfigCompiler().WithCloseOnContextDone(tc.ensureTermination))
			defer r.Close(ctx)
			m, err := r.Instantiate(ctx, wasmBytes)
			if err != nil {
				b.Fatal(err)
			}
			sum := m.ExportedFunction("sum")
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := sum.Call(ctx, iterations); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
