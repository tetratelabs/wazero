## wazero CLI

The wazero CLI can be used to execute a standalone WebAssembly binary.

### Installation

```bash
$ go install github.com/tetratelabs/wazero/cmd/wazero@latest
```

### Usage

The wazero CLI accepts a single argument, the path to a WebAssembly binary.
Arguments can be passed to the WebAssembly binary itself after the path.

```bash
wazero run calc.wasm 1 + 2
```

In addition to arguments, the WebAssembly binary has access to stdout, stderr,
and stdin.


### Docker / Podman

wazero doesn't currently publish binaries, but you can make your own with our
example [Dockerfile](Dockerfile). It should amount to about 4.5MB total.

```bash
# build the image
$ docker build -t wazero:latest -f Dockerfile .
# volume mount wasi or GOOS=js wasm you are interested in, and run it.
$ docker run -v ./testdata/:/wasm wazero:latest /wasm/wasi_arg.wasm 1 2 3
wasi_arg.wasm123
```
