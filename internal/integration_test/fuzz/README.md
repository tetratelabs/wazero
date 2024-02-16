Fuzzing infrastructure for wazero engines via [wasm-tools](https://github.com/bytecodealliance/wasm-tools).

### Dependency

- [cargo](https://doc.rust-lang.org/cargo/getting-started/installation.html)
  - Needs to enable nightly (for libFuzzer).
- [cargo-fuzz](https://github.com/rust-fuzz/cargo-fuzz)
  - `cargo install cargo-fuzz`

### Run Fuzzing

Currently, we have the following fuzzing targets:

- `no_diff`: compares the results from the compiler and interpreter engines, and see if there's a diff in them.
- `memory_no_diff`: same as `no_diff` except that in addition to the results, it also compares the entire memory buffer between engines to ensure the consistency around memory access.
  Therefore, this takes much longer than `no_diff`.
- `validation`: try compiling maybe-invalid Wasm module binaries. This is to ensure that our validation phase works correctly as well as the engines do not panic during compilation.


To run the fuzzer on a target, execute the following command:

```
# Running on the host archictecture.
cargo fuzz run <target>
```

where you replace `<target>` with one of the targets described above.

See `cargo fuzz run --help` for the options. Especially, the following flags are useful:

- `-jobs=N`: `cargo fuzz run` by default only spawns one worker, so this flag helps do the parallel fuzzing.
  - usage: `cargo fuzz run no_diff -- -jobs=5` will run 5 parallel workers to run fuzzing jobs.
- `-max_total_time`: the maximum total time in seconds to run the fuzzer.
  - usage: `cargo fuzz run no_diff -- -max_total_time=100` will run fuzzing for 100 seconds.
- `-timeout` sets the timeout seconds _per fuzzing run_, not the entire job.
- `-rss_limit_mb` sets the memory usage limit which is 2GB by default. Usually 2GB is not enough for some large Wasm binary.

#### Example commands

```
# Running the `no_diff` target with 15 concurrent jobs with total runnig time with 2hrs and 8GB memory limit.
$ cargo fuzz run no_diff --sanitizer=none -- -rss_limit_mb=8192 -max_len=5000000 -max_total_time=7200 -jobs=15

# Running the `memory_no_diff` target with 15 concurrent jobs with timeout 2hrs and setting timeout per fuzz case to 30s.
$ cargo fuzz run memory_no_diff --sanitizer=none -- -timeout=30 -max_total_time=7200 -jobs=15

# Running the `validation` target with 4 concurrent jobs with timeout 2hrs and setting timeout per fuzz case to 30s.
# cargo fuzz run validation --sanitizer=none -- -timeout=30 -max_total_time=7200 -jobs=4
```

Note that `--sanitizer=none` is always recommended to use because the sanitizer is not useful for our use case plus this will speed up the fuzzing by like multiple times.

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

        cargo fuzz tmin no_diff fuzz/artifacts/no_diff/crash-d2c1f5307fde6f057454606bcc21d5653be9be8d

────────────────────────────────────────────────────────────────────────────────
```

and you can use that command to "minimize" the input binary while keeping the same error.


Alternatively, you can use the following command to minimize the arbitrary input binary:

```
go test -c ./wazerolib -o nodiff.test && wasm-tools shrink ./predicate.sh original.{wasm,wat} -o shrinken.wasm --attempts 4294967295
```

which uses `wasm-tools shrinken` command to minimize the input binary. Internally, the `predicate.sh` is invoked for each input binary
where it executes the `nodiff.test` binary which runs `TestReRunFailedRequireNoDiffCase`.
