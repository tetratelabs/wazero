This directory contains tests which use multiple packages. For example:

- `engine` contains variety of e2e tests, mainly to ensure the consistency in the behavior between engines.
- `codec` contains a test and benchmark on text and binary decoders.
- `spectest` contains end-to-end tests with the [WebAssembly specification tests](https://github.com/WebAssembly/spec/tree/wg-1.0/test/core).
- `wasi` contains end-to-end tests on the interoperability of our WASI implementation with language runtimes. This also substitutes for WASI spec tests until we have another option.
