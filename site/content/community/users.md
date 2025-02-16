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
| [wasi-go][33]    | WASI host module for Wazero with experimental support for socket extensions                          |
| [wazergo][29]    | Generics library for type-safe and high performance wazero host modules                              |
| [Wetware][28]    | Simple, secure & scalable clusters                                                                   |
| [mjml-go][19]    | Compile [MJML][20] to HTML directly in Go                                                            |
| [wzprof][32]     | CPU and Memory profiler for WebAssembly modules, based on Wazero                                     |
| [avif][39]       | AVIF encoder/decoder based on libavif                                                                |
| [heic][40]       | HEIC decoder based on libheif                                                                        |
| [jpegxl][41]     | JPEG XL encoder/decoder based on libjxl                                                              |
| [jpegli][42]     | JPEG encoder/decoder based on jpegli                                                                 |
| [webp][43]       | WEBP encoder/decoder based on libwebp                                                                |

### General purpose plugins

| Name                           | Description                                                                   |
|:-------------------------------|-------------------------------------------------------------------------------|
| [Extism][38]                   | Simplified cross-language extensibility in Go & a dozen+ languages|
| [go-plugin][2]                 | implements [Protocol Buffers][8] services with WebAssembly vi code generation |
| [waPC][5]                      | implements [Apex][6] interfaces with WebAssembly via code generation          |
| [wazero-emscripten-embind][36] | Emscripten [Embind][37] and code generation support for Wazero                |

### Infrastructure-as-Code

| Name                   | Description                                        |
|:-----------------------|----------------------------------------------------|
| [yoke][47] | A WebAssembly-based package deployer for Kubernetes, enabling declarative and programmable deployments with Wasm |

### Middleware

| Name                   | Description                                        |
|:-----------------------|----------------------------------------------------|
| [http-wasm-host-go][3] | serves HTTP handlers implemented in [http-wasm][4] |

### Network

| Name                   | Description                                                                                          |
|:-----------------------|------------------------------------------------------------------------------------------------------|
| [Redpanda Connect][30] | implements 3rd party extension via [WASM processor][31] and [Redpanda Data Transform processor][45]  |
| [dapr][15]             | implements 3rd party extension via [WASM middleware][16]                                             |
| [mosn][9]              | implements 3rd party extension via [proxy-wasm][10]                                                  |

### Security

| Name                  | Description                                                                                        |
|:----------------------|----------------------------------------------------------------------------------------------------|
| [trivy][17]           | implements 3rd party extension via [wasm modules][18]                                              |
| [RunReveal][34]       | Security data platform which uses Wazero for transforms and alerting                               |
| [Impart Security][35] | API security solution with a WASM based rules engine using wazero as part of a security mesh layer |

### Cloud Platforms

| Name          | Description                                                                      |
|:--------------|----------------------------------------------------------------------------------|
| [Modus][46]   | An open source, serverless framework for building intelligent functions and APIs |
| [scale][13]   | implements [Polyglot][14] interfaces with WebAssembly via code generation        |
| [taubyte][21] | edge computing and web3 platform that runs [serverless functions][22]            |
| [YoMo][26]    | Streaming [Serverless][27] Framework for building Geo-distributed system         |
| [Wetware][28] | Web3's answer to Cloud hosting                                                   |

### Database

| Name          | Description                              |
|:--------------|------------------------------------------|
| [wescale][44] | a database proxy that supports OnlineDDL |


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

[30]: https://docs.redpanda.com/redpanda-connect

[31]: https://docs.redpanda.com/redpanda-connect/components/processors/wasm/

[32]: https://github.com/stealthrocket/wzprof

[33]: https://github.com/stealthrocket/wasi-go

[34]: https://runreveal.com/

[35]: https://impart.security/

[36]: https://github.com/jerbob92/wazero-emscripten-embind

[37]: https://emscripten.org/docs/porting/connecting_cpp_and_javascript/embind.html

[38]: https://github.com/extism/extism

[39]: https://github.com/gen2brain/avif

[40]: https://github.com/gen2brain/heic

[41]: https://github.com/gen2brain/jpegxl

[42]: https://github.com/gen2brain/jpegli

[43]: https://github.com/gen2brain/webp

[44]: https://github.com/wesql/wescale

[45]: https://docs.redpanda.com/redpanda-connect/components/processors/redpanda_data_transform/

[46]: https://github.com/hypermodeinc/modus

[47]: https://github.com/yokecd/yoke
