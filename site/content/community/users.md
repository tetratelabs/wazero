+++
title = "Who is using wazero"
+++

## Who is using wazero?

Below are an incomplete list of projects that use wazero strategically as a
part of their open source or commercial work. Please support our community by
considering their efforts before starting your own!

### General purpose plugins

| Name           | Description                                                                                 |
|:---------------|---------------------------------------------------------------------------------------------|
| [go-plugin][2] | use gRPC to define WebAssembly plugins                                                      |
| [waPC][5]      | uses an intuitive [IDL][6] and code generation to create WebAssembly plugins |

### HTTP

| Name                   | Description                                        |
|:-----------------------|----------------------------------------------------|
| [http-wasm-host-go][3] | serves HTTP handlers implemented in [http-wasm][4] |

### Libraries

| Name        | Description                          |
|:------------|--------------------------------------|
| [go-re2][7] | high performance regular expressions |

## Updating this list

This is a community maintained list. It may have an inaccurate or outdated
entries, or missing something entirely. Changes to the [source][1] are
welcome, but please be conscious that not all projects desire to be on lists.
To ensure we promote community members, please do not add works that don't use
wazero to this list. Please keep descriptions short for a better table
experience.

[1]: https://github.com/tetratelabs/wazero/tree/main/site/content/community/users.md
[2]: https://github.com/knqyf263/go-plugin
[3]: https://github.com/http-wasm/http-wasm-host-go
[4]: https://http-wasm.io
[5]: https://wapc.io
[6]: https://apexlang.io
[7]: https://github.com/wasilibs/go-re2
