+++
title = "Languages"
layout = "single"
+++

WebAssembly has a virtual machine architecture where the host is the embedding
process and the guest is a program compiled into the WebAssembly Binary Format,
also known as wasm. The first step is to take a source file and compile it into
the wasm bytecode.

Ex. If your source is in Go, you might compile it with TinyGo.
```goat
    .-----------.    .----------------------.      .-----------.
   /  main.go  /---->|  tinygo -target=wasi +---->/ main.wasm /
  '-----+-----'      '----------------------'    '-----------'
```

Below are a list of languages notes wazero contributed so far.

* [TinyGo](tinygo) Ex. `tinygo build -o X.wasm -scheduler=none --no-debug -target=wasi X.go`

Note: These are not official documentation, and may be out of date. Please help
us [maintain][1] these and [star our GitHub repository][2] if they are helpful.
Together, we can help make WebAssembly easier on the next person.

[1]: https://github.com/tetratelabs/wazero/tree/main/site/content/languages
[2]: https://github.com/tetratelabs/wazero/stargazers
