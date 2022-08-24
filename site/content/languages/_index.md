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

Below are notes wazero contributed so far, in alphabetical order by language.

* [TinyGo](tinygo) Ex. `tinygo build -o X.wasm -target=wasi X.go`

wazero is a runtime that embeds in Golang applications, not a web browser. As
such, these notes bias towards backend use of WebAssembly, not browser use.

Disclaimer: These are not official documentation, nor represent the teams who
maintain language compilers. If you see any errors, please help [maintain][1]
these and [star our GitHub repository][2] if they are helpful. Together, we can
make WebAssembly easier on the next person.

[1]: https://github.com/tetratelabs/wazero/tree/main/site/content/languages
[2]: https://github.com/tetratelabs/wazero/stargazers
