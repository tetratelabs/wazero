# wazero

WebAssembly is a way to safely run code compiled in other languages. Runtimes
execute WebAssembly Modules (Wasm), which are most often binaries with a `.wasm`
extension.

wazero is a [WebAssembly 1.0][1] spec compliant runtime written in Go. It has
*zero dependencies*, and doesn't rely on CGO. This means you can run
applications in other languages and still keep cross compilation.

Import wazero and extend your Go application with code written in any language!

## Example

The best way to learn this and other features you get with wazero is by trying
one of our [examples](examples).

For the impatient, here's how invoking a factorial function looks in wazero:

```golang
func main() {
	// Choose the context to use for function calls.
	ctx := context.Background()

	// Read a WebAssembly binary containing an exported "fac" function.
	// * Ex. (func (export "fac") (param i64) (result i64) ...
	source, _ := os.ReadFile("./path/to/fac.wasm")

	// Instantiate the module and return its exported functions
	module, _ := wazero.NewRuntime().InstantiateModuleFromCode(ctx, source)
	defer module.Close(ctx)

	// Discover 7! is 5040
	fmt.Println(module.ExportedFunction("fac").Call(ctx, 7))
}
```

Note: `fac.wasm` was compiled from [fac.wat][3], in the [WebAssembly 1.0][1]
Text Format, it could have been written in another language that compiles to
(targets) WebAssembly, such as AssemblyScript, C, C++, Rust, TinyGo or Zig.

## Deeper dive

The former example is a pure function. While a good start, you probably are
wondering how to do something more realistic, like read a file. WebAssembly
Modules (Wasm) are sandboxed similar to containers. They can't read anything
on your machine unless you explicitly allow it.

The WebAssembly Core Specification is a standard, governed by W3C process, but
it has no scope to specify how system resources like files are accessed.
Instead, WebAssembly defines "host functions" and the signatures they can use.
In wazero, "host functions" are written in Go, and let you do anything
including access files. The main constraint is that WebAssembly only allows
numeric types.

For example, you can grant WebAssembly code access to your console by exporting
a function written in Go. The below function can be imported into standard
WebAssembly as the module "env" and the function name "log_i32".
```go
env, err := r.NewModuleBuilder("env").
	ExportFunction("log_i32", func(v uint32) {
		fmt.Println("log_i32 >>", v)
	}).
	Instantiate(ctx)
if err != nil {
	log.Fatal(err)
}
defer env.Close(ctx)
```

The WebAssembly community has [subgroups][4] which maintain work that may not
result in a Web Standard. One such group is the WebAssembly System Interface
([WASI][5]), which defines functions similar to Go's [x/sys/unix][6].

The [wasi_snapshot_preview1][13] tag of WASI is widely implemented, so wazero
bundles an implementation. That way, you don't have to write these functions.

For example, here's how you can allow WebAssembly modules to read
"/work/home/a.txt" as "/a.txt" or "./a.txt":
```go
wm, err := wasi.InstantiateSnapshotPreview1(ctx, r)
defer wm.Close(ctx)

config := wazero.ModuleConfig().WithFS(os.DirFS("/work/home"))
module, err := r.InstantiateModule(ctx, binary, config)
defer module.Close(ctx)
...
```

While we hope this deeper dive was useful, we also provide [examples](examples)
to elaborate each point. Please try these before raising usage questions as
they may answer them for you!

## Runtime

There are two runtime configurations supported in wazero: _JIT_ is default:

If you don't choose, ex `wazero.NewRuntime()`, JIT is used if supported. You can also force the interpreter like so:
```go
r := wazero.NewRuntimeWithConfig(wazero.NewRuntimeConfigInterpreter())
```

### Interpreter
Interpreter is a naive interpreter-based implementation of Wasm virtual
machine. Its implementation doesn't have any platform (GOARCH, GOOS) specific
code, therefore _interpreter_ can be used for any compilation target available
for Go (such as `riscv64`).

### JIT
JIT (Just In Time) compiles WebAssembly modules into machine code during
`Runtime.CompileModule` so that they are executed natively at runtime. JIT is
faster than Interpreter, often by order of magnitude (10x) or more. This is
done while still having no host-specific dependencies.

If interested, check out the [RATIONALE.md][8] and help us optimize further!

### Conformance

Both runtimes pass [WebAssembly 1.0 spectests][7] on supported platforms:

| Runtime     | Usage| amd64 | arm64 | others |
|:---:|:---:|:---:|:---:|:---:|
| Interpreter|`wazero.NewRuntimeConfigInterpreter()`|✅ |✅|✅|
| JIT |`wazero.NewRuntimeConfigJIT()`|✅|✅ |❌|

## Support Policy

The below support policy focuses on compatability concerns of those embedding
wazero into their Go applications.

### wazero

wazero is an early project, so APIs are subject to change until version 1.0.

We expect [wazero 1.0][9] to be at or before Q3 2022, so please practice the
current APIs to ensure they work for you!

### Go

wazero has no dependencies except Go, so the only source of conflict in your
project's use of wazero is the Go version.

To simplify our support policy, we adopt Go's [Release Policy][10] (two versions).

This means wazero will remain compilable and tested on the version prior to the
latest release of Go.

For example, once Go 1.29 is released, wazero may use a Go 1.28 feature.

### Platform

wazero has two runtime modes: Interpreter and JIT. The only supported operating
systems are ones we test, but that doesn't necessarily mean other operating
system versions won't work.

We currently test Linux (Ubuntu and scratch), MacOS and Windows as packaged by
[GitHub Actions][11].

* Interpreter
  * Linux is tested on amd64 (native) as well arm64 and riscv64 via emulation.
  * MacOS and Windows are only tested on amd64.
* JIT
  * Linux is tested on amd64 (native) as well arm64 via emulation.
  * MacOS and Windows are only tested on amd64.

wazero has no dependencies and doesn't require CGO. This means it can also be
embedded in an application that doesn't use an operating system. This is a main
differentiator between wazero and alternatives.

We verify zero dependencies by running tests in Docker's [scratch image][12].
This approach ensures compatibility with any parent image.

## Specifications

wazero understands that while no-one desired to create confusion, confusion
exists both in what is a standard and what in practice is in fact a standard
feature. To help with this, we created some guidance both on the status quo
of WebAssembly portability and what we support.

The WebAssembly Core Specification is the only specification relevant to
wazero, governed by a standards body. Release [1.0][1] is a Web Standard (REC).
Release [2.0][2] is a Working Draft (WD), so not yet a Web Standard.

Many compilers implement system calls using the WebAssembly System Interface,
[WASI][5]. WASI is a WebAssembly community [subgroup][4], but has not published
any working drafts as a result of their work. WASI's last stable point was
[wasi_snapshot_preview1][13], tagged at the end of 2020.

While this seems scary, the confusion caused by pre-standard features is not as
bad as it sounds. The WebAssembly ecosystem is generally responsive regardless
of where things are written down and wazero provides tools, such as built-in
support for WASI, to reduce pain.

The goal of this section isn't to promote a W3C recommendation exclusive
approach, rather to help you understand common language around portable
features and which of those wazero supports at the moment. While we consider
features formalized through W3C recommendation status mandatory, we actively
pursue pre-standard features as well interop with commonly used infrastructure
such as AssemblyScript.

In summary, we hope this section can guide you in terms of what wazero supports
as well as how to classify a request for a feature we don't yet support.

### WebAssembly Core
wazero conforms with spectests [7] defined alongside WebAssembly Core
Specification [1.0][1]. This is the default [RuntimeConfig][18].

The WebAssembly Core Specification [2.0][2] is in draft form and wazero has
[work in progress][14] towards that. Opt in via the below configuration:
```go
rConfig = wazero.NewRuntimeConfig().WithWasmCore2()
```

One current limitation of wazero is that it doesn't fully implement the Text
Format, yet, e.g. compiling `.wat` files. The intent is to [finish this][15],
and meanwhile users can work around this using tools such as `wat2wasm` to
compile the text format into the binary format. In practice, the text format is
too low level for most users, so delays here have limited impact.

#### Post 2.0 Features
Features regardless of W3C release are inventoried in the [Proposals][16].
repository. wazero implements [Finished Proposals][17] based on user demand,
using [wazero.RuntimeConfig][18] feature flags.

Features not yet assigned to a W3C release are not reliable. Encourage the
[WebAssembly community][19] to formalize features you rely on, so that they
become assigned to a release, and reach the W3C recommendation (REC) phase.

### WebAssembly System Interface (WASI)

Many compilers implement system calls using the WebAssembly System Interface,
[WASI][5]. WASI is a WebAssembly community [subgroup][4], but has not published
any working drafts as a result of their work. WASI's last stable point was
[wasi_snapshot_preview1][13], tagged at the end of 2020.

Some functions in this tag are used in practice while some others are not known
to be used at all. Further confusion exists because some compilers, like
GrainLang, import functions not used. Finally, [wasi_snapshot_preview1][13]
includes features such as "rights" that [will be removed][20].

For all of these reasons, wazero will not implement all WASI features, just to
complete the below chart. If you desire something not yet implemented, please
[raise an issue](https://github.com/tetratelabs/wazero/issues/new) and include
your use case (ex which language you are using to compile, a.k.a. target Wasm).

<details><summary>Click to see the full list of supported WASI functions</summary>
<p>

| Function                | Status | Known Usage    |
|:-----------------------:|:------:|---------------:|
| args_get                |   ✅   | TinyGo         |
| args_sizes_get          |   ✅   | TinyGo         |
| environ_get             |   ✅   | TinyGo         |
| environ_sizes_get       |   ✅   | TinyGo         |
| clock_res_get           |   ❌   |                |
| clock_time_get          |   ✅   | TinyGo         |
| fd_advise               |   ❌   |                |
| fd_allocate             |   ❌   |                |
| fd_close                |   ✅   | TinyGo         |
| fd_datasync             |   ❌   |                |
| fd_fdstat_get           |   ✅   | TinyGo         |
| fd_fdstat_set_flags     |   ❌   |                |
| fd_fdstat_set_rights    |   ❌   |                |
| fd_filestat_get         |   ❌   |                |
| fd_filestat_set_size    |   ❌   |                |
| fd_filestat_set_times   |   ❌   |                |
| fd_pread                |   ❌   |                |
| fd_prestat_get          |   ✅   | TinyGo,`fs.FS` |
| fd_prestat_dir_name     |   ✅   | TinyGo         |
| fd_pwrite               |   ❌   |                |
| fd_read                 |   ✅   | TinyGo,`fs.FS` |
| fd_readdir              |   ❌   |                |
| fd_renumber             |   ❌   |                |
| fd_seek                 |   ✅   | TinyGo         |
| fd_sync                 |   ❌   |                |
| fd_tell                 |   ❌   |                |
| fd_write                |   ✅   | `fs.FS`        |
| path_create_directory   |   ❌   |                |
| path_filestat_get       |   ❌   |                |
| path_filestat_set_times |   ❌   |                |
| path_link               |   ❌   |                |
| path_open               |   ✅   | TinyGo,`fs.FS` |
| path_readlink           |   ❌   |                |
| path_remove_directory   |   ❌   |                |
| path_rename             |   ❌   |                |
| path_symlink            |   ❌   |                |
| path_unlink_file        |   ❌   |                |
| poll_oneoff             |   ✅   | TinyGo         |
| proc_exit               |   ✅   | AssemblyScript |
| proc_raise              |   ❌   |                |
| sched_yield             |   ❌   |                |
| random_get              |   ✅   |                |
| sock_recv               |   ❌   |                |
| sock_send               |   ❌   |                |
| sock_shutdown           |   ❌   |                |

</p>
</details>

## History of wazero

wazero was originally developed by [Takeshi Yoneda][21] as a hobby project in
mid 2020. In late 2021, it was sponsored by Tetrate as a top-level project.
That said, Takeshi's original motivation is as relevant today as when he
started the project, and worthwhile reading:

If you want to provide Wasm host environments in your Go programs, currently
there's no other choice than using CGO leveraging the state-of-the-art
runtimes written in C++/Rust (e.g. V8, Wasmtime, Wasmer, WAVM, etc.), and
there's no pure Go Wasm runtime out there. (There's only one exception named
[wagon][22], but it was archived with the mention to this project.)

First, why do you want to write host environments in Go? You might want to have
plugin systems in your Go project and want these plugin systems to be
safe/fast/flexible, and enable users to write plugins in their favorite
languages. That's where Wasm comes into play. You write your own Wasm host
environments and embed Wasm runtime in your projects, and now users are able to
write plugins in their own favorite languages (AssemblyScript, C, C++, Rust,
Zig, etc.). As a specific example, you maybe write proxy severs in Go and want
to allow users to extend the proxy via [Proxy-Wasm ABI][23]. Maybe you are
writing server-side rendering applications via Wasm, or [OpenPolicyAgent][24]
is using Wasm for plugin system.

However, experienced Golang developers often avoid using CGO because it
introduces complexity. For example, CGO projects are larger and complicated to
consume due to their libc + shared library dependency. Debugging is more
difficult for Go developers when most of a library is written in Rustlang.
[_CGO is not Go_][25] [ -- _Rob_ _Pike_][26] dives in deeper. In short, the
primary motivation to start wazero was to avoid CGO.

wazero compiles WebAssembly modules into native assembly (JIT) by default. You
may be surprised to find equal or better performance vs mature JIT-style
runtimes because [CGO is slow][27]. More specifically, if you make large amount
of CGO calls which cross the boundary between Go and C (stack) space, then the
usage of CGO could be a bottleneck.

[1]: https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/
[2]: https://www.w3.org/TR/2022/WD-wasm-core-2-20220419/
[3]: ./internal/integration_test/post1_0/multi-value/testdata/fac.wat
[4]: https://github.com/WebAssembly/meetings/blob/main/process/subgroups.md
[5]: https://github.com/WebAssembly/WASI
[6]: https://pkg.go.dev/golang.org/x/sys/unix
[7]: https://github.com/WebAssembly/spec/tree/wg-1.0/test/core
[8]: ./internal/wasm/jit/RATIONALE.md
[9]: https://github.com/tetratelabs/wazero/issues/506
[10]: https://go.dev/doc/devel/release
[11]: https://github.com/actions/virtual-environments
[12]: https://docs.docker.com/develop/develop-images/baseimages/#create-a-simple-parent-image-using-scratch
[13]: https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md
[14]: https://github.com/tetratelabs/wazero/issues/484
[15]: https://github.com/tetratelabs/wazero/issues/59
[16]: https://github.com/WebAssembly/proposals
[17]: https://github.com/WebAssembly/proposals/blob/main/finished-proposals.md
[18]: https://pkg.go.dev/github.com/tetratelabs/wazero#RuntimeConfig
[19]: https://www.w3.org/community/webassembly/
[20]: https://github.com/WebAssembly/WASI/issues/469#issuecomment-1045251844
[21]: https://github.com/mathetake
[22]: https://github.com/go-interpreter/wagon
[23]: https://github.com/proxy-wasm/spec
[24]: https://www.openpolicyagent.org/docs/latest/wasm/
[25]: https://dave.cheney.net/2016/01/18/cgo-is-not-go
[26]: https://www.youtube.com/watch?v=PAAkCSZUG1c&t=757s
[27]: https://github.com/golang/go/issues/19574
