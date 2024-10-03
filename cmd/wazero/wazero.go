package main

import (
	"context"
	"crypto/rand"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strconv"
	"strings"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/experimental/logging"
	"github.com/tetratelabs/wazero/experimental/sock"
	"github.com/tetratelabs/wazero/experimental/sysfs"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	internalsys "github.com/tetratelabs/wazero/internal/sys"
	"github.com/tetratelabs/wazero/internal/version"
	"github.com/tetratelabs/wazero/sys"
)

func main() {
	os.Exit(doMain(os.Stdout, os.Stderr))
}

// doMain is separated out for the purpose of unit testing.
func doMain(stdOut io.Writer, stdErr logging.Writer) int {
	flag.CommandLine.SetOutput(stdErr)

	var help bool
	flag.BoolVar(&help, "h", false, "Prints usage.")

	flag.Parse()

	if help || flag.NArg() == 0 {
		printUsage(stdErr)
		return 0
	}

	if flag.NArg() < 1 {
		fmt.Fprintln(stdErr, "missing path to wasm file")
		printUsage(stdErr)
		return 1
	}

	subCmd := flag.Arg(0)
	switch subCmd {
	case "compile":
		return doCompile(flag.Args()[1:], stdErr)
	case "run":
		return doRun(flag.Args()[1:], stdOut, stdErr)
	case "version":
		fmt.Fprintln(stdOut, version.GetWazeroVersion())
		return 0
	default:
		fmt.Fprintln(stdErr, "invalid command")
		printUsage(stdErr)
		return 1
	}
}

func doCompile(args []string, stdErr io.Writer) int {
	flags := flag.NewFlagSet("compile", flag.ExitOnError)
	flags.SetOutput(stdErr)

	var help bool
	flags.BoolVar(&help, "h", false, "Prints usage.")

	var count int
	var cpuProfile string
	var memProfile string
	if version.GetWazeroVersion() != version.Default {
		count = 1
	} else {
		flags.IntVar(&count, "count", 1,
			"Number of times to perform the compilation. This is useful to benchmark performance of the wazero compiler.")

		flags.StringVar(&cpuProfile, "cpuprofile", "",
			"Enables cpu profiling and writes the profile at the given path.")

		flags.StringVar(&memProfile, "memprofile", "",
			"Enables memory profiling and writes the profile at the given path.")
	}

	cacheDir := cacheDirFlag(flags)

	_ = flags.Parse(args)

	if help {
		printCompileUsage(stdErr, flags)
		return 0
	}

	if flags.NArg() < 1 {
		fmt.Fprintln(stdErr, "missing path to wasm file")
		printCompileUsage(stdErr, flags)
		return 1
	}

	if memProfile != "" {
		defer writeHeapProfile(stdErr, memProfile)
	}

	if cpuProfile != "" {
		stopCPUProfile := startCPUProfile(stdErr, cpuProfile)
		defer stopCPUProfile()
	}

	wasmPath := flags.Arg(0)

	wasm, err := os.ReadFile(wasmPath)
	if err != nil {
		fmt.Fprintf(stdErr, "error reading wasm binary: %v\n", err)
		return 1
	}

	c := wazero.NewRuntimeConfig()
	if rc, cache := maybeUseCacheDir(cacheDir, stdErr); rc != 0 {
		return rc
	} else if cache != nil {
		c = c.WithCompilationCache(cache)
	}

	ctx := context.Background()
	rt := wazero.NewRuntimeWithConfig(ctx, c)
	defer rt.Close(ctx)

	for count > 0 {
		compiledModule, err := rt.CompileModule(ctx, wasm)
		if err != nil {
			fmt.Fprintf(stdErr, "error compiling wasm binary: %v\n", err)
			return 1
		}
		if err := compiledModule.Close(ctx); err != nil {
			fmt.Fprintf(stdErr, "error releasing compiled module: %v\n", err)
			return 1
		}
		count--
	}
	return 0
}

func doRun(args []string, stdOut io.Writer, stdErr logging.Writer) int {
	flags := flag.NewFlagSet("run", flag.ExitOnError)
	flags.SetOutput(stdErr)

	var help bool
	flags.BoolVar(&help, "h", false, "Prints usage.")

	var useInterpreter bool
	flags.BoolVar(&useInterpreter, "interpreter", false,
		"Interprets WebAssembly modules instead of compiling them into native code.")

	var envs sliceFlag
	flags.Var(&envs, "env", "key=value pair of environment variable to expose to the binary. "+
		"Can be specified multiple times.")

	var envInherit bool
	flags.BoolVar(&envInherit, "env-inherit", false,
		"Inherits any environment variables from the calling process. "+
			"Variables specified with the <env> flag are appended to the inherited list.")

	var mounts sliceFlag
	flags.Var(&mounts, "mount",
		"Filesystem path to expose to the binary in the form of <path>[:<wasm path>][:ro]. "+
			"This may be specified multiple times. When <wasm path> is unset, <path> is used. "+
			"For example, -mount=/:/ or c:\\:/ makes the entire host volume writeable by wasm. "+
			"For read-only mounts, append the suffix ':ro'. "+
			"Note that the volume mount inherently allows the guest to escape the volume via relative path lookups like '../../'. "+
			"If that is not desired, use wazero as a library and implement a custom fs.FS.")

	var listens sliceFlag
	flags.Var(&listens, "listen",
		"Open a TCP socket on the specified address of the form <host:port>. "+
			"This may be specified multiple times. Host is optional, and port may be 0 to "+
			"indicate a random port.")

	var timeout time.Duration
	flags.DurationVar(&timeout, "timeout", 0*time.Second,
		"If a wasm binary runs longer than the given duration string, then exit abruptly. "+
			"The duration string is an unsigned sequence of decimal numbers, "+
			"each with optional fraction and a unit suffix, such as \"300ms\", \"1.5h\" or \"2h45m\". "+
			"Valid time units are \"ns\", \"us\" (or \"µs\"), \"ms\", \"s\", \"m\", \"h\". "+
			"If the duration is 0, the timeout is disabled. The default is disabled.")

	var hostlogging logScopesFlag
	flags.Var(&hostlogging, "hostlogging",
		"A comma-separated list of host function scopes to log to stderr. "+
			"This may be specified multiple times. Supported values: all,clock,filesystem,memory,proc,poll,random,sock")

	var cpuProfile string
	var memProfile string
	if version.GetWazeroVersion() == version.Default {
		flags.StringVar(&cpuProfile, "cpuprofile", "",
			"Enables cpu profiling and writes the profile at the given path.")

		flags.StringVar(&memProfile, "memprofile", "",
			"Enables memory profiling and writes the profile at the given path.")
	}

	cacheDir := cacheDirFlag(flags)

	_ = flags.Parse(args)

	if help {
		printRunUsage(stdErr, flags)
		return 0
	}

	if flags.NArg() < 1 {
		fmt.Fprintln(stdErr, "missing path to wasm file")
		printRunUsage(stdErr, flags)
		return 1
	}

	if memProfile != "" {
		defer writeHeapProfile(stdErr, memProfile)
	}

	if cpuProfile != "" {
		stopCPUProfile := startCPUProfile(stdErr, cpuProfile)
		defer stopCPUProfile()
	}

	wasmPath := flags.Arg(0)
	wasmArgs := flags.Args()[1:]
	if len(wasmArgs) > 1 {
		// Skip "--" if provided
		if wasmArgs[0] == "--" {
			wasmArgs = wasmArgs[1:]
		}
	}

	// Don't use map to preserve order
	var env []string
	if envInherit {
		envs = append(os.Environ(), envs...)
	}
	for _, e := range envs {
		fields := strings.SplitN(e, "=", 2)
		if len(fields) != 2 {
			fmt.Fprintf(stdErr, "invalid environment variable: %s\n", e)
			return 1
		}
		env = append(env, fields[0], fields[1])
	}

	rc, _, fsConfig := validateMounts(mounts, stdErr)
	if rc != 0 {
		return rc
	}

	wasm, err := os.ReadFile(wasmPath)
	if err != nil {
		fmt.Fprintf(stdErr, "error reading wasm binary: %v\n", err)
		return 1
	}

	wasmExe := filepath.Base(wasmPath)

	var rtc wazero.RuntimeConfig
	if useInterpreter {
		rtc = wazero.NewRuntimeConfigInterpreter()
	} else {
		rtc = wazero.NewRuntimeConfig()
	}

	ctx := maybeHostLogging(context.Background(), logging.LogScopes(hostlogging), stdErr)

	if rc, cache := maybeUseCacheDir(cacheDir, stdErr); rc != 0 {
		return rc
	} else if cache != nil {
		rtc = rtc.WithCompilationCache(cache)
	}

	if timeout > 0 {
		newCtx, cancel := context.WithTimeout(ctx, timeout)
		ctx = newCtx
		defer cancel()
		rtc = rtc.WithCloseOnContextDone(true)
	} else if timeout < 0 {
		fmt.Fprintf(stdErr, "timeout duration may not be negative, %v given\n", timeout)
		printRunUsage(stdErr, flags)
		return 1
	}

	if rc, sockCfg := validateListens(listens, stdErr); rc != 0 {
		return rc
	} else {
		ctx = sock.WithConfig(ctx, sockCfg)
	}

	rt := wazero.NewRuntimeWithConfig(ctx, rtc)
	defer rt.Close(ctx)

	// Because we are running a binary directly rather than embedding in an application,
	// we default to wiring up commonly used OS functionality.
	conf := wazero.NewModuleConfig().
		WithStdout(stdOut).
		WithStderr(stdErr).
		WithStdin(os.Stdin).
		WithRandSource(rand.Reader).
		WithFSConfig(fsConfig).
		WithSysNanosleep().
		WithSysNanotime().
		WithSysWalltime().
		WithArgs(append([]string{wasmExe}, wasmArgs...)...)
	for i := 0; i < len(env); i += 2 {
		conf = conf.WithEnv(env[i], env[i+1])
	}

	guest, err := rt.CompileModule(ctx, wasm)
	if err != nil {
		fmt.Fprintf(stdErr, "error compiling wasm binary: %v\n", err)
		return 1
	}

	switch detectImports(guest.ImportedFunctions()) {
	case modeWasi:
		wasi_snapshot_preview1.MustInstantiate(ctx, rt)
		_, err = rt.InstantiateModule(ctx, guest, conf)
	case modeWasiUnstable:
		// Instantiate the current WASI functions under the wasi_unstable
		// instead of wasi_snapshot_preview1.
		wasiBuilder := rt.NewHostModuleBuilder("wasi_unstable")
		wasi_snapshot_preview1.NewFunctionExporter().ExportFunctions(wasiBuilder)
		_, err = wasiBuilder.Instantiate(ctx)
		if err == nil {
			// Instantiate our binary, but using the old import names.
			_, err = rt.InstantiateModule(ctx, guest, conf)
		}
	case modeDefault:
		_, err = rt.InstantiateModule(ctx, guest, conf)
	}

	if err != nil {
		if exitErr, ok := err.(*sys.ExitError); ok {
			exitCode := exitErr.ExitCode()
			if exitCode == sys.ExitCodeDeadlineExceeded {
				fmt.Fprintf(stdErr, "error: %v (timeout %v)\n", exitErr, timeout)
			}
			return int(exitCode)
		}
		fmt.Fprintf(stdErr, "error instantiating wasm binary: %v\n", err)
		return 1
	}

	// We're done, _start was called as part of instantiating the module.
	return 0
}

func validateMounts(mounts sliceFlag, stdErr logging.Writer) (rc int, rootPath string, config wazero.FSConfig) {
	config = wazero.NewFSConfig()
	for _, mount := range mounts {
		if len(mount) == 0 {
			fmt.Fprintln(stdErr, "invalid mount: empty string")
			return 1, rootPath, config
		}

		readOnly := false
		if trimmed := strings.TrimSuffix(mount, ":ro"); trimmed != mount {
			mount = trimmed
			readOnly = true
		}

		// TODO: Support wasm paths with colon in them.
		var dir, guestPath string
		if clnIdx := strings.LastIndexByte(mount, ':'); clnIdx != -1 {
			dir, guestPath = mount[:clnIdx], mount[clnIdx+1:]
		} else {
			dir = mount
			guestPath = dir
		}

		// Eagerly validate the mounts as we know they should be on the host.
		if abs, err := filepath.Abs(dir); err != nil {
			fmt.Fprintf(stdErr, "invalid mount: path %q invalid: %v\n", dir, err)
			return 1, rootPath, config
		} else {
			dir = abs
		}

		if stat, err := os.Stat(dir); err != nil {
			fmt.Fprintf(stdErr, "invalid mount: path %q error: %v\n", dir, err)
			return 1, rootPath, config
		} else if !stat.IsDir() {
			fmt.Fprintf(stdErr, "invalid mount: path %q is not a directory\n", dir)
		}

		root := sysfs.DirFS(dir)
		if readOnly {
			root = &sysfs.ReadFS{FS: root}
		}

		config = config.(sysfs.FSConfig).WithSysFSMount(root, guestPath)

		if internalsys.StripPrefixesAndTrailingSlash(guestPath) == "" {
			rootPath = dir
		}
	}
	return 0, rootPath, config
}

// validateListens returns a non-nil net.Config, if there were any listen flags.
func validateListens(listens sliceFlag, stdErr logging.Writer) (rc int, config sock.Config) {
	for _, listen := range listens {
		idx := strings.LastIndexByte(listen, ':')
		if idx < 0 {
			fmt.Fprintln(stdErr, "invalid listen")
			return rc, config
		}
		port, err := strconv.Atoi(listen[idx+1:])
		if err != nil {
			fmt.Fprintln(stdErr, "invalid listen port:", err)
			return rc, config
		}
		if config == nil {
			config = sock.NewConfig()
		}
		config = config.WithTCPListener(listen[:idx], port)
	}
	return
}

const (
	modeDefault importMode = iota
	modeWasi
	modeWasiUnstable
)

type importMode uint

func detectImports(imports []api.FunctionDefinition) importMode {
	for _, f := range imports {
		moduleName, _, _ := f.Import()
		switch moduleName {
		case wasi_snapshot_preview1.ModuleName:
			return modeWasi
		case "wasi_unstable":
			return modeWasiUnstable
		}
	}
	return modeDefault
}

func maybeHostLogging(ctx context.Context, scopes logging.LogScopes, stdErr logging.Writer) context.Context {
	if scopes != 0 {
		return experimental.WithFunctionListenerFactory(ctx, logging.NewHostLoggingListenerFactory(stdErr, scopes))
	}
	return ctx
}

func cacheDirFlag(flags *flag.FlagSet) *string {
	return flags.String("cachedir", "", "Writeable directory for native code compiled from wasm. "+
		"Contents are re-used for the same version of wazero.")
}

func maybeUseCacheDir(cacheDir *string, stdErr io.Writer) (int, wazero.CompilationCache) {
	if dir := *cacheDir; dir != "" {
		if cache, err := wazero.NewCompilationCacheWithDir(dir); err != nil {
			fmt.Fprintf(stdErr, "invalid cachedir: %v\n", err)
			return 1, cache
		} else {
			return 0, cache
		}
	}
	return 0, nil
}

func printUsage(stdErr io.Writer) {
	fmt.Fprintln(stdErr, "wazero CLI")
	fmt.Fprintln(stdErr)
	fmt.Fprintln(stdErr, "Usage:\n  wazero <command>")
	fmt.Fprintln(stdErr)
	fmt.Fprintln(stdErr, "Commands:")
	fmt.Fprintln(stdErr, "  compile\tPre-compiles a WebAssembly binary")
	fmt.Fprintln(stdErr, "  run\t\tRuns a WebAssembly binary")
	fmt.Fprintln(stdErr, "  version\tDisplays the version of wazero CLI")
}

func printCompileUsage(stdErr io.Writer, flags *flag.FlagSet) {
	fmt.Fprintln(stdErr, "wazero CLI")
	fmt.Fprintln(stdErr)
	fmt.Fprintln(stdErr, "Usage:\n  wazero compile <options> <path to wasm file>")
	fmt.Fprintln(stdErr)
	fmt.Fprintln(stdErr, "Options:")
	flags.PrintDefaults()
}

func printRunUsage(stdErr io.Writer, flags *flag.FlagSet) {
	fmt.Fprintln(stdErr, "wazero CLI")
	fmt.Fprintln(stdErr)
	fmt.Fprintln(stdErr, "Usage:\n  wazero run <options> <path to wasm file> [--] <wasm args>")
	fmt.Fprintln(stdErr)
	fmt.Fprintln(stdErr, "Options:")
	flags.PrintDefaults()
}

func startCPUProfile(stdErr io.Writer, path string) (stopCPUProfile func()) {
	f, err := os.Create(path)
	if err != nil {
		fmt.Fprintf(stdErr, "error creating cpu profile output: %v\n", err)
		return func() {}
	}

	if err := pprof.StartCPUProfile(f); err != nil {
		f.Close()
		fmt.Fprintf(stdErr, "error starting cpu profile: %v\n", err)
		return func() {}
	}

	return func() {
		defer f.Close()
		pprof.StopCPUProfile()
	}
}

func writeHeapProfile(stdErr io.Writer, path string) {
	f, err := os.Create(path)
	if err != nil {
		fmt.Fprintf(stdErr, "error creating memory profile output: %v\n", err)
		return
	}
	defer f.Close()
	runtime.GC()
	if err := pprof.WriteHeapProfile(f); err != nil {
		fmt.Fprintf(stdErr, "error writing memory profile: %v\n", err)
	}
}

type sliceFlag []string

func (f *sliceFlag) String() string {
	return strings.Join(*f, ",")
}

func (f *sliceFlag) Set(s string) error {
	*f = append(*f, s)
	return nil
}

type logScopesFlag logging.LogScopes

func (f *logScopesFlag) String() string {
	return logging.LogScopes(*f).String()
}

func (f *logScopesFlag) Set(input string) error {
	for _, s := range strings.Split(input, ",") {
		switch s {
		case "":
			continue
		case "all":
			*f |= logScopesFlag(logging.LogScopeAll)
		case "clock":
			*f |= logScopesFlag(logging.LogScopeClock)
		case "filesystem":
			*f |= logScopesFlag(logging.LogScopeFilesystem)
		case "memory":
			*f |= logScopesFlag(logging.LogScopeMemory)
		case "proc":
			*f |= logScopesFlag(logging.LogScopeProc)
		case "poll":
			*f |= logScopesFlag(logging.LogScopePoll)
		case "random":
			*f |= logScopesFlag(logging.LogScopeRandom)
		case "sock":
			*f |= logScopesFlag(logging.LogScopeSock)
		default:
			return errors.New("not a log scope")
		}
	}
	return nil
}
