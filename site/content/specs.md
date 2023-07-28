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

### WebAssembly Core {#core}
wazero conforms with tests defined alongside WebAssembly Core
Specification [1.0][1] and [2.0][14].

By default, the runtime configuration enables features in WebAssembly Core
Specification, despite it not yet being a Web Standard (REC). You can select
version 1.0 like so:
```go
rConfig = wazero.NewRuntimeConfig().WithCoreFeatures(api.CoreFeaturesV1)
```

One current limitation of wazero is that it doesn't implement the Text
Format, e.g. compiling `.wat` files. Users can work around this using tools such as `wat2wasm` to
compile the text format into the binary format. In practice, the text format is
too low level for most users, so delays here have limited impact.

#### Post 2.0 Features
Features regardless of W3C release are inventoried in the [Proposals][10].
repository. wazero implements [Finished Proposals][11] based on user demand,
using [wazero.RuntimeConfig][7] feature flags. As of late 2022, all finished
proposals are included in [2.0][14] Working Draft.

Features not yet assigned to a W3C release are not reliable. Encourage the
[WebAssembly community][12] to formalize features you rely on, so that they
become assigned to a release, and reach the W3C recommendation (REC) phase.

### WebAssembly System Interface (WASI) {#wasi}

Many compilers implement system calls using the WebAssembly System Interface,
[WASI][5]. WASI is a WebAssembly community [subgroup][3], but has not published
any working drafts as a result of their work. WASI's last stable point was
[wasi_snapshot_preview1][4], tagged at the end of 2020.

Some functions in this tag are used in practice while some others are not known
to be used at all. Further confusion exists because some compilers, like
GrainLang, import functions not used. Finally, some functions were added and
removed after the git tag. For example, [`proc_raise`][13] was removed and
[`sock_accept`][15] added.

For all of these reasons, wazero will not implement all WASI features, just to
complete the below chart. If you desire something not yet implemented, please
[raise an issue](https://github.com/tetratelabs/wazero/issues/new) and include
your use case (ex which language you are using to compile, a.k.a. target Wasm).

Notes:
 * AssemblyScript has its own ABI which can optionally use [wasi-shim][17]
 * C (via clang) supports the maximum WASI functions due to [wasi-libc][16]
 * Rust supports WASI via its [wasm32-wasi][18] target.

<details><summary>Click to see the full list of supported WASI functions</summary>
<p>

| Function                | Status |     Known Usage |
|:------------------------|:------:|----------------:|
| args_get                |   ✅    |          TinyGo |
| args_sizes_get          |   ✅    |          TinyGo |
| environ_get             |   ✅    |          TinyGo |
| environ_sizes_get       |   ✅    |          TinyGo |
| clock_res_get           |   ✅    |                 |
| clock_time_get          |   ✅    |          TinyGo |
| fd_advise               |   👷   |                 |
| fd_allocate             |   ✅    |            Rust |
| fd_close                |   ✅    |          TinyGo |
| fd_datasync             |   ✅    |            Rust |
| fd_fdstat_get           |   ✅    |          TinyGo |
| fd_fdstat_set_flags     |   ✅    |            Rust |
| fd_fdstat_set_rights    |   💀   |                 |
| fd_filestat_get         |   ✅    |             Zig |
| fd_filestat_set_size    |   ✅    |        Rust,Zig |
| fd_filestat_set_times   |   ✅    |        Rust,Zig |
| fd_pread                |   ✅    |             Zig |
| fd_prestat_get          |   ✅    | Rust,TinyGo,Zig |
| fd_prestat_dir_name     |   ✅    | Rust,TinyGo,Zig |
| fd_pwrite               |   ✅    |        Rust,Zig |
| fd_read                 |   ✅    | Rust,TinyGo,Zig |
| fd_readdir              |   ✅    |        Rust,Zig |
| fd_renumber             |   ✅    |            libc |
| fd_seek                 |   ✅    |          TinyGo |
| fd_sync                 |   ✅    |              Go |
| fd_tell                 |   ✅    |            Rust |
| fd_write                |   ✅    | Rust,TinyGo,Zig |
| path_create_directory   |   ✅    | Rust,TinyGo,Zig |
| path_filestat_get       |   ✅    | Rust,TinyGo,Zig |
| path_filestat_set_times |   ✅    |       Rust,libc |
| path_link               |   ✅    |        Rust,Zig |
| path_open               |   ✅    | Rust,TinyGo,Zig |
| path_readlink           |   ✅    |        Rust,Zig |
| path_remove_directory   |   ✅    | Rust,TinyGo,Zig |
| path_rename             |   ✅    | Rust,TinyGo,Zig |
| path_symlink            |   ✅    |        Rust,Zig |
| path_unlink_file        |   ✅    | Rust,TinyGo,Zig |
| poll_oneoff             |   ✅    | Rust,TinyGo,Zig |
| proc_exit               |   ✅    | Rust,TinyGo,Zig |
| proc_raise              |   💀   |                 |
| sched_yield             |   ✅    |            Rust |
| random_get              |   ✅    | Rust,TinyGo,Zig |
| sock_accept             |   ✅    |        Rust,Zig |
| sock_recv               |   ✅    |        Rust,Zig |
| sock_send               |   ✅    |        Rust,Zig |
| sock_shutdown           |   ✅    |        Rust,Zig |

Note: 💀 means the function was later removed from WASI.

</p>
</details>

[1]: https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/
[2]: https://www.w3.org/TR/2022/WD-wasm-core-2-20220419/
[3]: https://github.com/WebAssembly/meetings/blob/main/process/subgroups.md
[4]: https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md
[5]: https://github.com/WebAssembly/WASI
[6]: https://github.com/WebAssembly/spec/tree/wg-1.0/test/core
[7]: https://pkg.go.dev/github.com/tetratelabs/wazero#RuntimeConfig
[9]: https://github.com/tetratelabs/wazero/issues/59
[10]: https://github.com/WebAssembly/proposals
[11]: https://github.com/WebAssembly/proposals/blob/main/finished-proposals.md
[12]: https://www.w3.org/community/webassembly/
[13]: https://github.com/WebAssembly/WASI/pull/136
[14]: https://github.com/WebAssembly/spec/tree/d39195773112a22b245ffbe864bab6d1182ccb06/test/core
[15]: https://github.com/WebAssembly/WASI/pull/458
[16]: https://github.com/WebAssembly/wasi-libc
[17]: https://github.com/AssemblyScript/wasi-shim
[18]: https://github.com/rust-lang/rust/tree/1.68.0/library/std/src/sys/wasi
