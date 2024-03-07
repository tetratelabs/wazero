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
	runDWARFTest(t, r, dwarftestdata.ZigWasm, `module[] function[_start] failed: wasm error: unreachable
wasm stack trace:
	.builtin.default_panic(i32,i32,i32,i32)
		0x63: /builtin.zig:889:17
	.main.main() i32
		0x25: /main.zig:10:5 (inlined)
		      /main.zig:6:5 (inlined)
		      /main.zig:2:5
	._start()
		0x6a: /start.zig:609:37 (inlined)
		      /start.zig:224:5`)
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
	runDWARFTest(t, r, dwarftestdata.RustWasm, `module[] function[_start] failed: wasm error: unreachable
wasm stack trace:
	.__rust_start_panic(i32) i32
		0xc474: /index.rs:286:39 (inlined)
		        /const_ptr.rs:870:18 (inlined)
		        /index.rs:286:39 (inlined)
		        /mod.rs:1630:46 (inlined)
		        /mod.rs:405:20 (inlined)
		        /mod.rs:1630:46 (inlined)
		        /mod.rs:1548:18 (inlined)
		        /iter.rs:1478:30 (inlined)
		        /count.rs:74:18
	.rust_panic(i32,i32)
		0xa3f8: /validations.rs:57:19 (inlined)
		        /validations.rs:57:19 (inlined)
		        /iter.rs:140:15 (inlined)
		        /iter.rs:140:15 (inlined)
		        /iterator.rs:330:13 (inlined)
		        /iterator.rs:377:9 (inlined)
		        /mod.rs:1455:35
	.std::panicking::rust_panic_with_hook::h93e119628869d575(i32,i32,i32,i32,i32)
		0x42df: /alloc.rs:244:22 (inlined)
		        /alloc.rs:244:22 (inlined)
		        /alloc.rs:342:9 (inlined)
		        /mod.rs:487:1 (inlined)
		        /mod.rs:487:1 (inlined)
		        /mod.rs:487:1 (inlined)
		        /mod.rs:487:1 (inlined)
		        /mod.rs:487:1 (inlined)
		        /panicking.rs:292:17 (inlined)
		        /panicking.rs:292:17
	.std::panicking::begin_panic_handler::{{closure}}::h2b8c0798e533b227(i32,i32,i32)
		0xaa8c: /mod.rs:362:12 (inlined)
		        /mod.rs:1257:22 (inlined)
		        /mod.rs:1235:21 (inlined)
		        /mod.rs:1214:26
	.std::sys_common::backtrace::__rust_end_short_backtrace::h030a533bc034da65(i32)
		0xc144: /mod.rs:188:26
	.rust_begin_unwind(i32)
		0xb7df: /mod.rs:1629:9 (inlined)
		        /builders.rs:199:17 (inlined)
		        /result.rs:1352:22 (inlined)
		        /builders.rs:187:23
	.core::panicking::panic_fmt::hb1bfc4175f838eff(i32,i32)
		0xbd3d: /mod.rs:1384:17
	.main::main::hfd44f54575e6bfdf()
		0xad2c: /memchr.rs
	.core::ops::function::FnOnce::call_once::h87e5f77996df3e28(i32)
		0xbd61: /mod.rs
	.std::sys_common::backtrace::__rust_begin_short_backtrace::h7ca17eb6aa97f768(i32)
		0xbd95: /mod.rs:1504:35 (inlined)
		        /mod.rs:1407:36
	.std::rt::lang_start::{{closure}}::he4aa401e76315dfe(i32) i32
		0xae9a: /location.rs:196:6
	.std::rt::lang_start_internal::h3c39e5d3c278a90f(i32,i32,i32,i32) i32
	.std::rt::lang_start::h779801844bd22a3c(i32,i32,i32) i32
		0xab94: /mod.rs:1226:2
	.__original_main() i32
		0xc0ae: /methods.rs:1677:13 (inlined)
		        /mod.rs:165:24 (inlined)
		        /mod.rs:165:24
	._start()
		0xc10f: /mod.rs:187
	._start.command_export()
		0xc3de: /iterator.rs:2414:21 (inlined)
		        /map.rs:124:9 (inlined)
		        /accum.rs:42:17 (inlined)
		        /iterator.rs:3347:9 (inlined)
		        /count.rs:135:5 (inlined)
		        /count.rs:135:5 (inlined)
		        /count.rs:71:21`)
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
