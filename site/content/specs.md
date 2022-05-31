+++
title = "Specifications"
+++

wazero understands that while no-one desired to create confusion, confusion
exists both in what is a standard and what in practice is in fact a standard
feature. To help with this, we created some guidance both on the status quo
of WebAssembly portability and what we support.

The WebAssembly Core Specification is the only specification relevant to
wazero, governed by a standards body. Release [1.0][1] is a Web Standard (REC).
Release [2.0][2] is a Working Draft (WD), so not yet a Web Standard.

Many compilers implement system calls using the WebAssembly System Interface,
[WASI][5]. WASI is a WebAssembly community [subgroup][3], but has not published
any working drafts as a result of their work. WASI's last stable point was
[wasi_snapshot_preview1][4], tagged at the end of 2020.

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
wazero conforms with [spectests][6] defined alongside WebAssembly Core
Specification [1.0][1]. This is the default [RuntimeConfig][7].

The WebAssembly Core Specification [2.0][2] is in draft form and wazero has
[work in progress][8] towards that. Opt in via the below configuration:
```go
rConfig = wazero.NewRuntimeConfig().WithWasmCore2()
```

One current limitation of wazero is that it doesn't fully implement the Text
Format, yet, e.g. compiling `.wat` files. The intent is to [finish this][9],
and meanwhile users can work around this using tools such as `wat2wasm` to
compile the text format into the binary format. In practice, the text format is
too low level for most users, so delays here have limited impact.

#### Post 2.0 Features
Features regardless of W3C release are inventoried in the [Proposals][10].
repository. wazero implements [Finished Proposals][11] based on user demand,
using [wazero.RuntimeConfig][7] feature flags.

Features not yet assigned to a W3C release are not reliable. Encourage the
[WebAssembly community][12] to formalize features you rely on, so that they
become assigned to a release, and reach the W3C recommendation (REC) phase.

### WebAssembly System Interface (WASI)

Many compilers implement system calls using the WebAssembly System Interface,
[WASI][5]. WASI is a WebAssembly community [subgroup][3], but has not published
any working drafts as a result of their work. WASI's last stable point was
[wasi_snapshot_preview1][4], tagged at the end of 2020.

Some functions in this tag are used in practice while some others are not known
to be used at all. Further confusion exists because some compilers, like
GrainLang, import functions not used. Finally, [wasi_snapshot_preview1][4]
includes features such as "rights" that [will be removed][13].

For all of these reasons, wazero will not implement all WASI features, just to
complete the below chart. If you desire something not yet implemented, please
[raise an issue](https://github.com/tetratelabs/wazero/issues/new) and include
your use case (ex which language you are using to compile, a.k.a. target Wasm).

<details><summary>Click to see the full list of supported WASI functions</summary>
<p>

| Function                | Status |               Known Usage |
|:------------------------|:------:|--------------------------:|
| args_get                |   ✅    |                    TinyGo |
| args_sizes_get          |   ✅    |                    TinyGo |
| environ_get             |   ✅    |                    TinyGo |
| environ_sizes_get       |   ✅    |                    TinyGo |
| clock_res_get           |   ✅    |                 SwiftWasm |
| clock_time_get          |   ✅    |         SwiftWasm, TinyGo |
| fd_advise               |   ❌    |                 SwiftWasm |
| fd_allocate             |   ❌    |                 SwiftWasm |
| fd_close                |   ✅    |         SwiftWasm, TinyGo |
| fd_datasync             |   ❌    |                 SwiftWasm |
| fd_fdstat_get           |   ✅    |         SwiftWasm, TinyGo |
| fd_fdstat_set_flags     |   ❌    |                 SwiftWasm |
| fd_fdstat_set_rights    |   ❌    |                 SwiftWasm |
| fd_filestat_get         |   ❌    |                 SwiftWasm |
| fd_filestat_set_size    |   ❌    |                 SwiftWasm |
| fd_filestat_set_times   |   ❌    |                 SwiftWasm |
| fd_pread                |   ❌    |                 SwiftWasm |
| fd_prestat_get          |   ✅    |         SwiftWasm, TinyGo |
| fd_prestat_dir_name     |   ✅    |         SwiftWasm, TinyGo |
| fd_pwrite               |   ❌    |                 SwiftWasm |
| fd_read                 |   ✅    |         SwiftWasm, TinyGo |
| fd_readdir              |   ❌    |                 SwiftWasm |
| fd_renumber             |   ❌    |                 SwiftWasm |
| fd_seek                 |   ✅    |         SwiftWasm, TinyGo |
| fd_sync                 |   ❌    |                 SwiftWasm |
| fd_tell                 |   ❌    |                 SwiftWasm |
| fd_write                |   ✅    |                 SwiftWasm |
| path_create_directory   |   ❌    |                 SwiftWasm |
| path_filestat_get       |   ❌    |                 SwiftWasm |
| path_filestat_set_times |   ❌    |                 SwiftWasm |
| path_link               |   ❌    |                 SwiftWasm |
| path_open               |   ✅    |         SwiftWasm, TinyGo |
| path_readlink           |   ❌    |                 SwiftWasm |
| path_remove_directory   |   ❌    |                 SwiftWasm |
| path_rename             |   ❌    |                 SwiftWasm |
| path_symlink            |   ❌    |                 SwiftWasm |
| path_unlink_file        |   ❌    |                 SwiftWasm |
| poll_oneoff             |   ✅    |         SwiftWasm, TinyGo |
| proc_exit               |   ✅    | AssemblyScript, SwiftWasm |
| proc_raise              |   ❌    |                 SwiftWasm |
| sched_yield             |   ❌    |                 SwiftWasm |
| random_get              |   ✅    |                 SwiftWasm |
| sock_recv               |   ❌    |                 SwiftWasm |
| sock_send               |   ❌    |                 SwiftWasm |
| sock_shutdown           |   ❌    |                 SwiftWasm |

</p>
</details>

[1]: https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/
[2]: https://www.w3.org/TR/2022/WD-wasm-core-2-20220419/
[3]: https://github.com/WebAssembly/meetings/blob/main/process/subgroups.md
[4]: https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md
[5]: https://github.com/WebAssembly/WASI
[6]: https://github.com/WebAssembly/spec/tree/wg-1.0/test/core
[7]: https://pkg.go.dev/github.com/tetratelabs/wazero#RuntimeConfig
[8]: https://github.com/tetratelabs/wazero/issues/484
[9]: https://github.com/tetratelabs/wazero/issues/59
[10]: https://github.com/WebAssembly/proposals
[11]: https://github.com/WebAssembly/proposals/blob/main/finished-proposals.md
[12]: https://www.w3.org/community/webassembly/
[13]: https://github.com/WebAssembly/WASI/issues/469#issuecomment-1045251844
