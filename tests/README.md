This directory contains tests which use multiple packages. For example:

* `bench` contains benchmark tests.
* `engine` contains variety of e2e tests, mainly to ensure the consistency in the behavior between engines.
* `spectest` contains end-to-end tests with the [WebAssembly specification tests](https://github.com/WebAssembly/spec/tree/wg-1.0/test/core).

*Note*: this doesn't contain WASI tests, as there's not yet an [official testsuite](https://github.com/WebAssembly/WASI/issues/9).
Meanwhile, WASI functions are unit tested including via Text Format imports [here](../wasi/wasi_test.go)
