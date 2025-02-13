package adhoc

import (
	"bufio"
	_ "embed"
	"strings"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/testing/dwarftestdata"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

var dwarfTests = map[string]testCase{
	"tinygo": {f: testTinyGoDWARF},
	"zig":    {f: testZigDWARF},
	"cc":     {f: testCCDWARF},
	"rust":   {f: testRustDWARF},
}

func TestEngineCompiler_DWARF(t *testing.T) {
	if !platform.CompilerSupported() {
		t.Skip()
	}
	runAllTests(t, dwarfTests, wazero.NewRuntimeConfigCompiler(), false)
}

func TestEngineInterpreter_DWARF(t *testing.T) {
	runAllTests(t, dwarfTests, wazero.NewRuntimeConfigInterpreter(), false)
}

func testTinyGoDWARF(t *testing.T, r wazero.Runtime) {
	runDWARFTest(t, r, dwarftestdata.TinyGoWasm, `module[] function[_start] failed: wasm error: unreachable
wasm stack trace:
	.runtime._panic(i32)
		0x18f3: /runtime_tinygowasm.go:70:6
	.main.c()
		0x2ff9: /main.go:16:7
	.main.b()
		0x2f97: /main.go:12:3
	.main.a()
		0x2f39: /main.go:8:3
	.main.main()
		0x2149: /main.go:4:3
	.runtime.run$1()
		0x1fcb: /scheduler_any.go:25:11
	.runtime.run$1$gowrapper(i32)
		0x6f0: /scheduler_any.go:23:2
	.tinygo_launch(i32)
		0x23: /task_asyncify_wasm.S:59
	.runtime.scheduler()
		0x1ec4: /task_asyncify.go:109:17 (inlined)
		        /scheduler.go:236:11
	.runtime.run()
		0x1d92: /scheduler_any.go:28:11
	._start()
		0x1d12: /runtime_wasm_wasi.go:21:5`)
}

func testZigDWARF(t *testing.T, r wazero.Runtime) {
	runDWARFTest(t, r, dwarftestdata.ZigWasm, `module[main.wasm] function[_start] failed: wasm error: unreachable
wasm stack trace:
	main.wasm.builtin.default_panic(i32,i32,i32,i32)
		0x63: /builtin.zig:792:17
	main.wasm.main.main(i32) i32
		0x25: /main.zig:10:5 (inlined)
		      /main.zig:6:5 (inlined)
		      /main.zig:2:5
	main.wasm._start()
		0xd1: /start.zig:524:37 (inlined)
		      /start.zig:199:5`)
}

func testCCDWARF(t *testing.T, r wazero.Runtime) {
	runDWARFTest(t, r, dwarftestdata.ZigCCWasm, `module[] function[_start] failed: wasm error: unreachable
wasm stack trace:
	.a()
		0x312: /main.c:7:18
	.__original_main() i32
		0x47c: /main.c:11:3
	._start()
	._start.command_export()`)
}

func testRustDWARF(t *testing.T, r wazero.Runtime) {
	runDWARFTest(t, r, dwarftestdata.RustWasm, `module[main-144f120e836a09da.wasm] function[_start] failed: wasm error: unreachable
wasm stack trace:
	main-144f120e836a09da.wasm.__rust_start_panic(i32,i32) i32
		0x3276: /lib.rs:100:17 (inlined)
		        /lib.rs:43:5
	main-144f120e836a09da.wasm.rust_panic(i32,i32)
		0x31c8: /panicking.rs:862:25
	main-144f120e836a09da.wasm._ZN3std9panicking20rust_panic_with_hook17hf4c55e90d4731159E(i32,i32,i32,i32,i32)
		0x319b: /panicking.rs:826:5
	main-144f120e836a09da.wasm._ZN3std9panicking19begin_panic_handler28_$u7b$$u7b$closure$u7d$$u7d$17h9e9ba254d816924bE(i32)
		0x25e2: /panicking.rs:667:13
	main-144f120e836a09da.wasm._ZN3std3sys9backtrace26__rust_end_short_backtrace17h5fb21e191bc452e3E(i32)
		0x251c: /backtrace.rs:170:18
	main-144f120e836a09da.wasm.rust_begin_unwind(i32)
		0x2b2f: /panicking.rs:665:5
	main-144f120e836a09da.wasm._ZN4core9panicking9panic_fmt17hfe24bec0337a4754E(i32,i32)
		0x798a: /panicking.rs:76:14
	main-144f120e836a09da.wasm._ZN4main4main17hef810e4bf58d9cdfE()
		0x373: /main.rs:12:5 (inlined)
		       /main.rs:7:5 (inlined)
		       /main.rs:2:5
	main-144f120e836a09da.wasm._ZN4core3ops8function6FnOnce9call_once17hb3419529f8e10fb1E(i32)
		0x124: /function.rs:250:5
	main-144f120e836a09da.wasm._ZN3std3sys9backtrace28__rust_begin_short_backtrace17h6b7139fa671fb72eE(i32)
		0x289: /backtrace.rs:154:18
	main-144f120e836a09da.wasm._ZN3std2rt10lang_start28_$u7b$$u7b$closure$u7d$$u7d$17hefb60d097516fc9fE(i32) i32
		0x20c: /rt.rs:195:18
	main-144f120e836a09da.wasm._ZN3std2rt19lang_start_internal17h1fceb22bbe5297a1E(i32,i32,i32,i32,i32) i32
		0x1780: /function.rs:284:13 (inlined)
		        /panicking.rs:557:40 (inlined)
		        /panicking.rs:520:19 (inlined)
		        /panic.rs:358:14 (inlined)
		        /rt.rs:174:48 (inlined)
		        /panicking.rs:557:40 (inlined)
		        /panicking.rs:520:19 (inlined)
		        /panic.rs:358:14 (inlined)
		        /rt.rs:174:20
	main-144f120e836a09da.wasm._ZN3std2rt10lang_start17he470b12ea6d4e370E(i32,i32,i32,i32) i32
		0x1a8: /rt.rs:194:17
	main-144f120e836a09da.wasm.__main_void() i32
	main-144f120e836a09da.wasm._start()
		0x37: wasisdk:/crt1-command.c:43:13`)
}

func runDWARFTest(t *testing.T, r wazero.Runtime, bin []byte, exp string) {
	if len(bin) == 0 {
		t.Skip() // Skip if the binary is empty which can happen when xz is not installed on the system
	}

	_, err := wasi_snapshot_preview1.Instantiate(testCtx, r)
	require.NoError(t, err)
	_, err = r.Instantiate(testCtx, bin)
	require.Error(t, err)

	errStr := err.Error()

	// Since stack traces change where the binary is compiled, we sanitize each line
	// so that it doesn't contain any file system dependent info.
	scanner := bufio.NewScanner(strings.NewReader(errStr))
	scanner.Split(bufio.ScanLines)
	var sanitizedLines []string
	for scanner.Scan() {
		line := scanner.Text()
		start, last := strings.Index(line, "/"), strings.LastIndex(line, "/")
		if start >= 0 {
			l := len(line) - last
			buf := []byte(line)
			copy(buf[start:], buf[last:])
			line = string(buf[:start+l])
		}
		sanitizedLines = append(sanitizedLines, line)
	}

	sanitizedTraces := strings.Join(sanitizedLines, "\n")
	require.Equal(t, exp, sanitizedTraces)
}
