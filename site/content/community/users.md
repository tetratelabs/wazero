+++
title = "Who is using wazero"
+++

## Who is using wazero?

Below are an incomplete list of projects that use wazero strategically as a
part of their open source or commercial work. Please support our community by
considering their efforts before starting your own!

### Libraries

| Name             | Description                                                                                          |
|:-----------------|------------------------------------------------------------------------------------------------------|
| [go-pdfium][23]  | [PDFium][24] bindings to do PDF operations in Go, also available as end application [pdfium-cli][25] |
| [go-re2][7]      | high performance regular expressions                                                                 |
| [go-sqlite3][11] | [SQLite][12] bindings, `database/sql` driver                                                         |
| [mjml-go][19]    | Compile [MJML][20] to HTML directly in Go                                                            |
| [wazergo][29]    | Generics library for type-safe and high performance wazero host modules                              |
| [Wetware][28]    | Simple, secure & scalable clusters                                                                   |

### General purpose plugins

| Name           | Description                                                                   |
|:---------------|-------------------------------------------------------------------------------|
| [go-plugin][2] | implements [Protocol Buffers][8] services with WebAssembly vi code generation |
| [waPC][5]      | implements [Apex][6] interfaces with WebAssembly via code generation          |

### Middleware

| Name                   | Description                                        |
|:-----------------------|----------------------------------------------------|
| [http-wasm-host-go][3] | serves HTTP handlers implemented in [http-wasm][4] |

### Network

| Name          | Description                                              |
|:--------------|----------------------------------------------------------|
| [Benthos][30] | implements 3rd party extension via [WASM processor][31]  |
| [dapr][15]    | implements 3rd party extension via [WASM middleware][16] |
| [mosn][9]     | implements 3rd party extension via [proxy-wasm][10]      |

### Security

| Name        | Description                                           |
|:------------|-------------------------------------------------------|
| [trivy][17] | implements 3rd party extension via [wasm modules][18] |

### Cloud Platforms

| Name          | Description                                                               |
|:--------------|---------------------------------------------------------------------------|
| [scale][13]   | implements [Polyglot][14] interfaces with WebAssembly via code generation |
| [taubyte][21] | edge computing and web3 platform that runs [serverless functions][22]     |
| [YoMo][26]    | Streaming [Serverless][27] Framework for buildnig Geo-distributed system  |
| [Wetware][28] | Web3's answer to Cloud hosting                                            |

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

[8]: https://protobuf.dev/overview/

[9]: https://mosn.io/

[10]: https://github.com/proxy-wasm/spec

[11]: https://github.com/ncruces/go-sqlite3

[12]: https://sqlite.org

[13]: https://scale.sh

[14]: https://github.com/loopholelabs/polyglot-go

[15]: https://dapr.io/

[16]: https://docs.dapr.io/reference/components-reference/supported-middleware/middleware-wasm/

[17]: https://trivy.dev/

[18]: https://aquasecurity.github.io/trivy/dev/docs/advanced/modules/

[19]: https://github.com/Boostport/mjml-go

[20]: https://mjml.io/

[21]: https://www.taubyte.com/

[22]: https://tau.how/docs/category/taubyte-serverless-functions

[23]: https://github.com/klippa-app/go-pdfium

[24]: https://pdfium.googlesource.com/pdfium/

[25]: https://github.com/klippa-app/pdfium-cli

[26]: https://github.com/yomorun/yomo

[27]: https://github.com/yomorun/yomo/tree/master/example/7-wasm

[28]: https://github.com/wetware/ww

[29]: https://github.com/stealthrocket/wazergo

[30]: https://www.benthos.dev/

[31]: https://www.benthos.dev/docs/components/processors/wasm
