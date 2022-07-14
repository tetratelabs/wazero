This directory contains tests which use multiple packages. For example:

* `bench` contains benchmark tests.
* `engine` contains variety of end-to-end tests, mainly to ensure the consistency in the behavior between engines.
* `fuzzcases` contains variety of test cases found by [wazero-fuzz](https://github.com/tetratelabs/wazero-fuzz).
* `post1_0` contains end-to-end tests for features [finished](https://github.com/WebAssembly/proposals/blob/main/finished-proposals.md) after WebAssembly 1.0 (20191205).
* `spectest` contains end-to-end tests with the [WebAssembly specification tests](https://github.com/WebAssembly/spec/tree/wg-1.0/test/core).
* `vs` tests and benchmarks VS other WebAssembly runtimes.

*Note*: This doesn't contain WASI tests, as there's not yet an [official testsuite](https://github.com/WebAssembly/WASI/issues/9).
Meanwhile, WASI functions are unit tested including via Text Format imports [here](../../wasi/wasi_test.go)
