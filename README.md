# wazero

WebAssembly is a way to safely run code compiled in other languages. Runtimes
execute WebAssembly Modules (Wasm), which are most often binaries with a `.wasm`
extension.

wazero is a [WebAssembly 1.0 (20191205)](https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/) spec compliant runtime written in Go.
It has *zero dependencies*, and doesn't rely on CGO. This means you can
run applications in other languages and still keep cross compilation.

Import wazero and extend your Go application with code written in any language!

## Example

The best way to learn this and other features you get with wazero is by trying one of our [examples](examples).
For the impatient, here's how invoking a factorial function looks in wazero:

```golang
func main() {
	// Choose the context to use for function calls.
	ctx := context.Background()

	// Read a WebAssembly binary containing an exported "fac" function.
	// * Ex. (func (export "fac") (param i64) (result i64) ...
	source, _ := os.ReadFile("./tests/bench/testdata/fac.wasm")

	// Instantiate the module and return its exported functions
	module, _ := wazero.NewRuntime().InstantiateModuleFromCode(ctx, source)
	defer module.Close()

	// Discover 7! is 5040
	fmt.Println(module.ExportedFunction("fac").Call(ctx, 7))
}
```

Note: While the [source for this](internal/integration_test/bench/testdata/fac.wat) is in the
WebAssembly 1.0 (20191205) Text Format, it could have been written in another
language that compiles to (targets) WebAssembly, such as AssemblyScript, C, C++, Rust, TinyGo or Zig.

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
defer env.Close()
```

While not a standards body like W3C, there is another dominant community in the
WebAssembly ecosystem: [The Bytecode Alliance](https://github.com/bytecodealliance/governance).
The Bytecode Alliance controls the WebAssembly System Interface ([WASI](https://github.com/WebAssembly/WASI)),
which is a set of function imports similar to Go's [x/sys/unix](https://pkg.go.dev/golang.org/x/sys/unix).
The "wasi_snapshot_preview1" version of WASI is widely implemented, so wazero
bundles an implementation. That way, you don't have to write these functions.

For example, here's how you can allow WebAssembly modules to read
"/work/home/a.txt" as "/a.txt" or "./a.txt":
```go
wm, err := wasi.InstantiateSnapshotPreview1(ctx, r)
defer wm.Close()

config := wazero.ModuleConfig().WithFS(os.DirFS("/work/home"))
module, err := r.InstantiateModule(ctx, binary, config)
defer module.Close()
...
```

While we hope this deeper dive was useful, we also provide [examples](examples)
to elaborate each point. Please try these before raising usage questions as
they may answer them for you!

## Runtime

There are two runtime configurations supported in wazero, where _JIT_ is default:

1. _Interpreter_: a naive interpreter-based implementation of Wasm virtual machine. Its implementation doesn't have any platform (GOARCH, GOOS) specific code, therefore _interpreter_ can be used for any compilation target available for Go (such as `riscv64`).
2. _JIT_: compiles WebAssembly modules, generates the machine code, and executing it all at runtime. Currently wazero implements the JIT compiler for `amd64` and `arm64` target. Generally speaking, _JIT_ is faster than _Interpreter_ by order of magnitude. However, the implementation is immature and has a bunch of aspects that could be improved (for example, it just does a singlepass compilation and doesn't do any optimizations, etc.). Please refer to [internal/wasm/jit/RATIONALE.md](internal/wasm/jit/RATIONALE.md) for the design choices and considerations in our JIT compiler.

Both configurations pass 100% of [WebAssembly spec test suites]((https://github.com/WebAssembly/spec/tree/wg-1.0/test/core)) (on supported platforms).

| Runtime     | Usage| amd64 | arm64 | others |
|:---:|:---:|:---:|:---:|:---:|
| Interpreter|`wazero.NewRuntimeConfigInterpreter()`|✅ |✅|✅|
| JIT |`wazero.NewRuntimeConfigJIT()`|✅|✅ |❌|

If you don't choose, ex `wazero.NewRuntime()`, JIT is used if supported. You can also force the interpreter like so:
```go
r := wazero.NewRuntimeWithConfig(wazero.NewRuntimeConfigInterpreter())
```

## Support Policy

The below support policy focuses on compatability concerns of those embedding wazero into their Go applications.

### wazero

wazero is an early project, so APIs are subject to change until version 1.0.

We expect 1.0 to be at or before Q3 2022, so please practice the current APIs to ensure they work for you!

### Go

wazero has no dependencies except Go, so the only source of conflict in your project's use of Wazero is the Go version.

To simplify our support policy, we adopt Go's [Release Policy](https://go.dev/doc/devel/release) (two versions).

This means wazero will remain compilable and tested on the the version prior to the latest release of Go.

For example, once Go 1.29 is released, wazero may choose to use a Go 1.28 feature.

### Platform

wazero has two runtime modes: Interpreter and JIT. The only supported operating
systems are ones we test, but that doesn't necessarily mean other operating
system versions won't work.

We currently test Linux (Ubuntu and scratch), MacOS and Windows as packaged by
[GitHub Actions](https://github.com/actions/virtual-environments).

* Interpreter
  * Linux is tested on amd64 (native) as well arm64 and riscv64 via emulation.
  * MacOS and Windows are only tested on amd64.
* JIT
  * Linux is tested on amd64 (native) as well arm64 via emulation.
  * MacOS and Windows are only tested on amd64.

wazero has no dependencies and doesn't require CGO. This means it can also be
embedded in an application that doesn't use an operating system. This is a main
differentiator between wazero and alternatives.

We verify wazero's independence by running tests in Docker's [scratch image](https://docs.docker.com/develop/develop-images/baseimages/#create-a-simple-parent-image-using-scratch).
This approach ensures compatibility with any parent image.

## Standards Compliance

wazero understands that while no-one desired to create confusion, confusion
exists both in what is a standard and what in practice is in fact a standard
feature. To help with this, we created some guidance both on the status quo
of WebAssembly portability and what we support.

The [WebAssembly Core Specification 1.0 (20191205)](https://www.w3.org/TR/2019/REC-wasm-core-1-20191205)
is the only part of the WebAssembly ecosystem that is a W3C recommendation.

In practice, this specification is not enough. Most compilers that target Wasm
rely on features that have not yet gone through W3C recommendation process,
such as `bulk-memory-operations`.

Also, most compilers implement system calls using the WebAssembly System
Interface ([WASI](https://github.com/WebAssembly/WASI)). While WASI aims to be
a common layer for portability, it is not governed by a standards body, like
W3C. Moreover, while commonly implemented, WASI is not yet versioned. Its last
stable point was the "wasi_snapshot_preview1" tag released at the end of 2020.

While this seems scary, the confusion caused by non-standard features and
non-standard specifications is not as bad as it sounds. The WebAssembly
ecosystem is generally responsive regardless of where things are written down
and wazero provides tools, such as built-in support for WASI, to reduce pain.

The goal of this section isn't to promote a W3C recommendation exclusive
approach, rather to help you understand common language around portable
features and which of those wazero supports at the moment. While we consider
features formalized through W3C recommendation status mandatory, we actively
pursue non-standard features as well interop with commonly used infrastructure
such as AssemblyScript.

In summary, we hope this section can guide you in terms of what wazero supports
as well as how to classify a request for a feature we don't yet support.

### WebAssembly Core
wazero supports the only WebAssembly specification which has reached W3C
Recommendation (REC) phase: [WebAssembly Core Specification 1.0 (20191205)](https://www.w3.org/TR/2019/REC-wasm-core-1-20191205).

Independent verification is possible via the [WebAssembly spec test suite](https://github.com/WebAssembly/spec/tree/wg-1.0/test/core),
which we run on every change and against all supported platforms.

One current limitation of wazero is that it doesn't fully implement the Text
Format, yet, e.g. compiling `.wat` files. The intent is to finish this, and
meanwhile users can work around this using tools such as `wat2wasm` to compile
the text format into the binary format.

#### Post 1.0 Features

The last W3C REC was at the end of 2019. There were other features that didn't
make the cut or were developed in the years since. The community unofficially
refers to these as [Finished Proposals](https://github.com/WebAssembly/proposals/blob/main/finished-proposals.md).

To ensure compatability, wazero does not opt-in to any non-standard feature by
default. However, the following status covers what's currently possible with
`wazero.RuntimeConfig`.

| Feature                               | Status |
|:-------------------------------------:|:------:|
| mutable-global                        |   ✅   |
| nontrapping-float-to-int-conversions  |   ❌   |
| sign-extension-ops                    |   ✅   |
| multi-value                           |   ✅   |
| JS-BigInt-integration                 |  N/A   |
| reference-types                       |   ❌   |
| bulk-memory-operations                |   ❌   |
| simd                                  |   ❌   |

Note: While the above are specified in a WebAssembly GitHub repository, they
are not W3C recommendations (standards). This means they can change further
between now and any future update to the W3C WebAssembly core specification.
Due to this, we cannot guarantee future compatability. Please encourage the
[WebAssembly community](https://www.w3.org/community/webassembly/) to formalize
features you rely on, specifically to reach the W3C recommendation (REC) phase.

### WebAssembly System Interface (WASI)

As of early 2022, the WebAssembly System Interface (WASI) has never reached the
recommendation phase of the W3C. The closest to stability is a draft ([snapshot-01](https://github.com/WebAssembly/WASI/tree/snapshot-01))
released in late 2020. Some functions of this draft are used in practice while
some features are not known to be used at all. Further confusion exists because
some compilers (ex GrainLang) import functions not used. Finally, `snapshot-01`
includes features such as "rights" that [will be removed](https://github.com/WebAssembly/WASI/issues/469#issuecomment-1045251844).

For all of these reasons, wazero will not implement all WASI features, just to
complete the below chart. If you desire something not yet implemented, please
[raise an issue](https://github.com/tetratelabs/wazero/issues/new) and include
your use case (ex which language you are using to compile, a.k.a. target Wasm).

<details><summary>Click to see the full list of supported WASI systemcalls</summary>
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

wazero was originally developed by [Takeshi Yoneda](https://github.com/mathetake)
as a hobby project in mid 2020. In late 2021, it was sponsored by Tetrate as a
top-level project. That said, Takeshi's original motivation is as relevant
today as when he started the project, and worthwhile reading:

If you want to provide Wasm host environments in your Go programs, currently
there's no other choice than using CGO andleveraging the state-of-the-art
runtimes written in C++/Rust (e.g. V8, Wasmtime, Wasmer, WAVM, etc.), and
there's no pure Go Wasm runtime out there. (There's only one exception named
[wagon](https://github.com/go-interpreter/wagon), but it was archived with the
mention to this project.)

First, why do you want to write host environments in Go? You might want to have
plugin systems in your Go project and want these plugin systems to be
safe/fast/flexible, and enable users to write plugins in their favorite
languages. That's where Wasm comes into play. You write your own Wasm host
environments and embed Wasm runtime in your projects, and now users are able to
write plugins in their own favorite lanugages (AssembyScript, C, C++, Rust,
Zig, etc.). As a specific example, you maybe write proxy severs in Go and want
to allow users to extend the proxy via [Proxy-Wasm ABI](https://github.com/proxy-wasm/spec).
Maybe you are writing server-side rendering applications via Wasm, or
[OpenPolicyAgent](https://www.openpolicyagent.org/docs/latest/wasm/) is using
Wasm for plugin system.

However, experienced Golang developers often avoid using CGO because it
introduces complexity. For example, CGO projects are larger and complicated to
consume due to their libc + shared library dependency. Debugging is more
difficult for Go developers when most of a library is written in Rustlang.
[_CGO is not Go_](https://dave.cheney.net/2016/01/18/cgo-is-not-go)[ -- _Rob_ _Pike_](https://www.youtube.com/watch?v=PAAkCSZUG1c&t=757s) dives in deeper.
In short, the primary motivation to start wazero was to avoid CGO.

wazero compiles WebAssembly modules into native assembly (JIT) by default. You
may be surprised to find equal or better performance vs mature JIT-style
runtimes because [CGO is slow](https://github.com/golang/go/issues/19574). More
specifically, if you make large amount of CGO calls which cross the boundary
between Go and C (stack) space, then the usage of CGO could be a bottleneck.

