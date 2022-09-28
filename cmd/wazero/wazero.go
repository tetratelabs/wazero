package main

import (
	"context"
	"crypto/rand"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/sys"
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

	if help || flag.NArg() == 0 {
		printUsage(stdErr)
		exit(0)
	}

	if flag.NArg() < 1 {
		fmt.Fprintln(stdErr, "missing path to wasm file")
		printUsage(stdErr)
		exit(1)
	}

	subCmd := flag.Arg(0)
	switch subCmd {
	case "run":
		doRun(flag.Args()[1:], stdOut, stdErr, exit)
	default:
		fmt.Fprintln(stdErr, "invalid command")
		printUsage(stdErr)
		exit(1)
	}

}

func doRun(args []string, stdOut io.Writer, stdErr io.Writer, exit func(code int)) {
	flags := flag.NewFlagSet("run", flag.ExitOnError)
	flags.SetOutput(stdErr)

	_ = flags.Parse(args)

	if flags.NArg() < 1 {
		fmt.Fprintln(stdErr, "missing path to wasm file")
		printRunUsage(stdErr, flags)
		exit(1)
	}
	wasmPath := flags.Arg(0)

	wasmArgs := flags.Args()[1:]
	if len(wasmArgs) > 1 {
		// Skip "--" if provided
		if wasmArgs[0] == "--" {
			wasmArgs = wasmArgs[1:]
		}
	}

	wasm, err := os.ReadFile(wasmPath)
	if err != nil {
		fmt.Fprintf(stdErr, "error reading wasm binary: %v\n", err)
		exit(1)
	}

	wasmExe := filepath.Base(wasmPath)

	ctx := context.Background()
	rt := wazero.NewRuntime(ctx)
	defer rt.Close(ctx)

	// Because we are running a binary directly rather than embedding in an application,
	// we default to wiring up commonly used OS functionality.
	conf := wazero.NewModuleConfig().
		WithStdout(stdOut).
		WithStderr(stdErr).
		WithStdin(os.Stdin).
		WithRandSource(rand.Reader).
		WithSysNanosleep().
		WithSysNanotime().
		WithSysWalltime().
		WithArgs(append([]string{wasmExe}, wasmArgs...)...)
	code, err := rt.CompileModule(ctx, wasm, wazero.NewCompileConfig())
	if err != nil {
		fmt.Fprintf(stdErr, "error compiling wasm binary: %v\n", err)
		exit(1)
	}

	// WASI is needed to access args and very commonly required by self-contained wasm
	// binaries, so we instantiate it by default.
	wasi_snapshot_preview1.MustInstantiate(ctx, rt)

	_, err = rt.InstantiateModule(ctx, code, conf)
	if err != nil {
		if exitErr, ok := err.(*sys.ExitError); ok {
			exit(int(exitErr.ExitCode()))
		}
		fmt.Fprintf(stdErr, "error instantiating wasm binary: %v\n", err)
		exit(1)
	}

	// We're done, _start was called as part of instantiating the module.
	exit(0)
}

func printUsage(stdErr io.Writer) {
	fmt.Fprintln(stdErr, "wazero CLI")
	fmt.Fprintln(stdErr)
	fmt.Fprintln(stdErr, "Usage:\n  wazero <command>")
	fmt.Fprintln(stdErr)
	fmt.Fprintln(stdErr, "Commands:")
	fmt.Fprintln(stdErr, "  run\t\tRuns a WebAssembly binary")
}

func printRunUsage(stdErr io.Writer, flags *flag.FlagSet) {
	fmt.Fprintln(stdErr, "wazero CLI")
	fmt.Fprintln(stdErr)
	fmt.Fprintln(stdErr, "Usage:\n  wazero run <path to wasm file> [--] <wasm args>")
	fmt.Fprintln(stdErr)
}
