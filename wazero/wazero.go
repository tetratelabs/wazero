package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

func main() {
	doMain(os.Stdout, os.Stderr, os.Exit)
}

// doMain is separated out for the purpose of unit testing.
func doMain(stdOut io.Writer, stdErr io.Writer, exit func(code int)) {
	flag.CommandLine.SetOutput(stdErr)

	var help bool
	flag.BoolVar(&help, "h", false, "print usage")

	flag.Parse()

	if help {
		printUsage(stdErr)
		exit(0)
	}

	if flag.NArg() < 1 {
		fmt.Fprintln(stdErr, "missing path to wasm file")
		printUsage(stdErr)
		exit(1)
	}

	var wasmArgs []string
	if flag.NArg() > 1 {
		if arg := flag.Arg(1); arg != "--" {
			fmt.Fprintf(stdErr, "invalid argument: %s\n", arg)
			printUsage(stdErr)
			exit(1)
		}
		wasmArgs = flag.Args()[2:]
	}

	wasmPath := flag.Arg(0)
	wasm, err := os.ReadFile(wasmPath)
	if err != nil {
		fmt.Fprintf(stdErr, "error reading wasm binary: %v\n", err)
		printUsage(stdErr)
		exit(1)
	}

	wasmExe := filepath.Base(wasmPath)

	ctx := context.Background()
	rt := wazero.NewRuntime(ctx)
	defer rt.Close(ctx)

	// Because we are running a binary directly rather than embedding in an application,
	// we default to wiring up standard streams and args.
	conf := wazero.NewModuleConfig().
		WithStdout(stdOut).
		WithStderr(stdErr).
		WithStdin(os.Stdin).
		WithArgs(append([]string{wasmExe}, wasmArgs...)...)
	code, err := rt.CompileModule(ctx, wasm, wazero.NewCompileConfig())
	if err != nil {
		fmt.Fprintf(stdErr, "error compiling wasm binary: %v\n", err)
		exit(1)
	}

	// WASI is needed to access args and very commonly required by self-contained wasm
	// binaries, so we instantiate it by default.
	_, _ = wasi_snapshot_preview1.Instantiate(ctx, rt)

	_, err = rt.InstantiateModule(ctx, code, conf)
	if err != nil {
		fmt.Fprintf(stdErr, "error instantiating wasm binary: %v\n", err)
		os.Exit(1)
	}

	// We're done, _start was called as part of instantiating the module.
	os.Exit(0)
}

func printUsage(stdErr io.Writer) {
	fmt.Fprintln(stdErr, "wazero CLI")
	fmt.Fprintln(stdErr)
	fmt.Fprintln(stdErr, "Usage:\n\twazero <path to wasm file> [-- <wasm args>]")
}
