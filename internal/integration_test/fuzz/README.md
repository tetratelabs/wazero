Fuzzing infrastructure for wazero engines via [wasm-tools](https://github.com/bytecodealliance/wasm-tools).

### Dependency

- [cargo](https://doc.rust-lang.org/cargo/getting-started/installation.html)
  - Needs to enable nightly (for libFuzzer).
- [cargo-fuzz](https://github.com/rust-fuzz/cargo-fuzz)
  - `cargo install cargo-fuzz`

### Run Fuzzing

Currently, we only have one kind of fuzzing named `basic` where we compare the results from the compiler
and interpreter engines, and see if there's a diff in them. To run the test, execute the following command:

```
# Running on the host archictecture.
cargo fuzz run basic

# Running on the specified architecture which is handy when developping on M1 Mac.
cargo fuzz run basic-x86_64-apple-darwin
```

See `cargo fuzz run --help` for the options. Especially, the following flags are useful:

- `-jobs=N`: `cargo fuzz run` by default only spawns one worker, so this flag helps do the parallel fuzzing.
  - usage: `cargo fuzz run basic -- -jobs=5` will run 5 parallel workers to run fuzzing jobs.
- `-max_total_time`: the maximum total time in seconds to run the fuzzer.
  - usage: `cargo fuzz run basic -- -max_total_time=100` will run fuzzing for 100 seconds.
- `-timeout` sets the timeout seconds _per fuzzing run_, not the entire job.


### Reproduce errors

If the fuzzer encounters error, you would get the output like the following:

```
Failed Wasm binary has been written to /Users/mathetake/wazero/internal/integration_test/fuzz/wazerolib/testdata/73c61e218b8547ef35271a22ca95f932dcc102bda9b3a9bdf1976e6ed36da31d.wasm
Failed Wasm Text has been written to /Users/mathetake/wazero/internal/integration_test/fuzz/wazerolib/testdata/73c61e218b8547ef35271a22ca95f932dcc102bda9b3a9bdf1976e6ed36da31d.wat
To reproduce the failure, execute: WASM_BINARY_PATH=/Users/mathetake/wazero/internal/integration_test/fuzz/wazerolib/testdata/73c61e218b8547ef35271a22ca95f932dcc102bda9b3a9bdf1976e6ed36da31d.wasm go test ./wazerolib/...
```

then you can check the wasm and wat as well as reproduce the error by running
```
WASM_BINARY_PATH=/Users/mathetake/wazero/internal/integration_test/fuzz/wazerolib/testdata/73c61e218b8547ef35271a22ca95f932dcc102bda9b3a9bdf1976e6ed36da31d.wasm go test ./wazerolib/...
```


Also, in the bottom of the output, you can find the message as

```

Minimize test case with:

        cargo fuzz tmin basic fuzz/artifacts/basic/crash-d2c1f5307fde6f057454606bcc21d5653be9be8d

────────────────────────────────────────────────────────────────────────────────
```

and you can use that command to "minimize" the input binary while keeping the same error.
