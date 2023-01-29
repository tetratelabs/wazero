package main

import (
	"bytes"
	_ "embed"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental/logging"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/version"
)

//go:embed testdata/wasi_arg.wasm
var wasmWasiArg []byte

//go:embed testdata/wasi_env.wasm
var wasmWasiEnv []byte

//go:embed testdata/wasi_fd.wasm
var wasmWasiFd []byte

//go:embed testdata/wasi_random_get.wasm
var wasmWasiRandomGet []byte

// wasmCatGo is compiled on demand with `GOARCH=wasm GOOS=js`
var wasmCatGo []byte

//go:embed testdata/cat/cat-tinygo.wasm
var wasmCatTinygo []byte

func TestMain(m *testing.M) {
	// For some reason, riscv64 fails to see directory listings.
	if a := runtime.GOARCH; a == "riscv64" {
		log.Println("main: skipping due to not yet supported GOARCH:", a)
		os.Exit(0)
	}

	// Notably our scratch containers don't have go, so don't fail tests.
	if err := compileGoJS(); err != nil {
		log.Println("main: Skipping GOARCH=wasm GOOS=js tests due to:", err)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestCompile(t *testing.T) {
	tmpDir, oldwd := requireChdirToTemp(t)
	defer os.Chdir(oldwd) //nolint

	wasmPath := filepath.Join(tmpDir, "test.wasm")
	require.NoError(t, os.WriteFile(wasmPath, wasmWasiArg, 0o600))

	existingDir1 := filepath.Join(tmpDir, "existing1")
	require.NoError(t, os.Mkdir(existingDir1, 0o700))
	existingDir2 := filepath.Join(tmpDir, "existing2")
	require.NoError(t, os.Mkdir(existingDir2, 0o700))

	tests := []struct {
		name       string
		wazeroOpts []string
		test       func(t *testing.T)
	}{
		{
			name: "no opts",
		},
		{
			name:       "cachedir existing absolute",
			wazeroOpts: []string{"--cachedir=" + existingDir1},
			test: func(t *testing.T) {
				entries, err := os.ReadDir(existingDir1)
				require.NoError(t, err)
				require.True(t, len(entries) > 0)
			},
		},
		{
			name:       "cachedir existing relative",
			wazeroOpts: []string{"--cachedir=existing2"},
			test: func(t *testing.T) {
				entries, err := os.ReadDir(existingDir2)
				require.NoError(t, err)
				require.True(t, len(entries) > 0)
			},
		},
		{
			name:       "cachedir new absolute",
			wazeroOpts: []string{"--cachedir=" + path.Join(tmpDir, "new1")},
			test: func(t *testing.T) {
				entries, err := os.ReadDir("new1")
				require.NoError(t, err)
				require.True(t, len(entries) > 0)
			},
		},
		{
			name:       "cachedir new relative",
			wazeroOpts: []string{"--cachedir=new2"},
			test: func(t *testing.T) {
				entries, err := os.ReadDir("new2")
				require.NoError(t, err)
				require.True(t, len(entries) > 0)
			},
		},
	}

	for _, tc := range tests {
		tt := tc
		t.Run(tt.name, func(t *testing.T) {
			args := append([]string{"compile"}, tt.wazeroOpts...)
			args = append(args, wasmPath)
			exitCode, stdOut, stdErr := runMain(t, args)
			require.Zero(t, stdErr)
			require.Equal(t, 0, exitCode, stdErr)
			require.Zero(t, stdOut)
			if test := tt.test; test != nil {
				test(t)
			}
		})
	}
}

func requireChdirToTemp(t *testing.T) (string, string) {
	tmpDir := t.TempDir()
	oldwd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmpDir))
	return tmpDir, oldwd
}

func TestCompile_Errors(t *testing.T) {
	tmpDir := t.TempDir()

	wasmPath := filepath.Join(tmpDir, "test.wasm")
	require.NoError(t, os.WriteFile(wasmPath, wasmWasiArg, 0o600))

	notWasmPath := filepath.Join(tmpDir, "bears.wasm")
	require.NoError(t, os.WriteFile(notWasmPath, []byte("pooh"), 0o600))

	tests := []struct {
		message string
		args    []string
	}{
		{
			message: "missing path to wasm file",
			args:    []string{},
		},
		{
			message: "error reading wasm binary",
			args:    []string{"non-existent.wasm"},
		},
		{
			message: "error compiling wasm binary",
			args:    []string{notWasmPath},
		},
		{
			message: "invalid cachedir",
			args:    []string{"--cachedir", notWasmPath, wasmPath},
		},
	}

	for _, tc := range tests {
		tt := tc
		t.Run(tt.message, func(t *testing.T) {
			exitCode, _, stdErr := runMain(t, append([]string{"compile"}, tt.args...))

			require.Equal(t, 1, exitCode)
			require.Contains(t, stdErr, tt.message)
		})
	}
}

func TestRun(t *testing.T) {
	tmpDir, oldwd := requireChdirToTemp(t)
	defer os.Chdir(oldwd) //nolint

	// We can't rely on the mtime from git because in CI, only the last commit
	// is checked out. Instead, grab the effective mtime.
	bearDir := filepath.Join(oldwd, "testdata", "fs")
	bearPath := filepath.Join(bearDir, "bear.txt")
	bearStat, err := os.Stat(bearPath)
	require.NoError(t, err)
	bearMtime := bearStat.ModTime().UnixMilli()
	// The file is world read, but windows cannot see that and reports world
	// write. Hence, we save off the current interpretation of mode for
	// comparison.
	bearMode := bearStat.Mode()

	existingDir1 := filepath.Join(tmpDir, "existing1")
	require.NoError(t, os.Mkdir(existingDir1, 0o700))
	existingDir2 := filepath.Join(tmpDir, "existing2")
	require.NoError(t, os.Mkdir(existingDir2, 0o700))

	type test struct {
		name             string
		wazeroOpts       []string
		wasm             []byte
		wasmArgs         []string
		expectedStdout   string
		expectedStderr   string
		expectedExitCode int
		test             func(t *testing.T)
	}

	tests := []test{
		{
			name:     "args",
			wasm:     wasmWasiArg,
			wasmArgs: []string{"hello world"},
			// Executable name is first arg so is printed.
			expectedStdout: "test.wasm\x00hello world\x00",
		},
		{
			name:     "-- args",
			wasm:     wasmWasiArg,
			wasmArgs: []string{"--", "hello world"},
			// Executable name is first arg so is printed.
			expectedStdout: "test.wasm\x00hello world\x00",
		},
		{
			name:           "env",
			wasm:           wasmWasiEnv,
			wazeroOpts:     []string{"--env=ANIMAL=bear", "--env=FOOD=sushi"},
			expectedStdout: "ANIMAL=bear\x00FOOD=sushi\x00",
		},
		{
			name:           "wasi",
			wasm:           wasmWasiFd,
			wazeroOpts:     []string{fmt.Sprintf("--mount=%s:/", bearDir)},
			expectedStdout: "pooh\n",
		},
		{
			name:           "wasi readonly",
			wasm:           wasmWasiFd,
			wazeroOpts:     []string{fmt.Sprintf("--mount=%s:/:ro", bearDir)},
			expectedStdout: "pooh\n",
		},
		{
			name:           "wasi non root",
			wasm:           wasmCatTinygo,
			wazeroOpts:     []string{fmt.Sprintf("--mount=%s:/animals:ro", bearDir)},
			wasmArgs:       []string{"/animals/bear.txt"},
			expectedStdout: "pooh\n",
		},
		{
			name:       "wasi hostlogging=random",
			wasm:       wasmWasiRandomGet,
			wazeroOpts: []string{"--hostlogging=random"},
			expectedStderr: `==> wasi_snapshot_preview1.random_get(buf=0,buf_len=1000)
<== errno=ESUCCESS
`,
		},
		{
			name:           "wasi hostlogging=filesystem",
			wasm:           wasmCatTinygo,
			wazeroOpts:     []string{"--hostlogging=filesystem", fmt.Sprintf("--mount=%s:/animals:ro", bearDir)},
			wasmArgs:       []string{"/animals/bear.txt"},
			expectedStdout: "pooh\n",
			expectedStderr: `==> wasi_snapshot_preview1.fd_prestat_get(fd=3)
<== (prestat={pr_name_len=1},errno=ESUCCESS)
==> wasi_snapshot_preview1.fd_prestat_dir_name(fd=3)
<== (path=/,errno=ESUCCESS)
==> wasi_snapshot_preview1.fd_prestat_get(fd=4)
<== (prestat=,errno=EBADF)
==> wasi_snapshot_preview1.fd_fdstat_get(fd=3)
<== (stat={filetype=DIRECTORY,fdflags=,fs_rights_base=,fs_rights_inheriting=},errno=ESUCCESS)
==> wasi_snapshot_preview1.path_open(fd=3,dirflags=SYMLINK_FOLLOW,path=animals/bear.txt,oflags=,fs_rights_base=,fs_rights_inheriting=,fdflags=)
<== (opened_fd=4,errno=ESUCCESS)
==> wasi_snapshot_preview1.fd_filestat_get(fd=4)
<== (filestat={filetype=REGULAR_FILE,size=5,mtim=1672360090212022269},errno=ESUCCESS)
==> wasi_snapshot_preview1.fd_read(fd=4,iovs=64776,iovs_len=1)
<== (nread=5,errno=ESUCCESS)
==> wasi_snapshot_preview1.fd_read(fd=4,iovs=64776,iovs_len=1)
<== (nread=0,errno=ESUCCESS)
==> wasi_snapshot_preview1.fd_close(fd=4)
<== errno=ESUCCESS
`,
		},
		{
			name:           "GOARCH=wasm GOOS=js",
			wasm:           wasmCatGo,
			wazeroOpts:     []string{fmt.Sprintf("--mount=%s:/", bearDir)},
			wasmArgs:       []string{"/bear.txt"},
			expectedStdout: "pooh\n",
		},
		{
			name:           "GOARCH=wasm GOOS=js readonly",
			wasm:           wasmCatGo,
			wazeroOpts:     []string{fmt.Sprintf("--mount=%s:/:ro", bearDir)},
			wasmArgs:       []string{"/bear.txt"},
			expectedStdout: "pooh\n",
		},
		{
			name:           "GOARCH=wasm GOOS=js hostlogging=filesystem",
			wasm:           wasmCatGo,
			wazeroOpts:     []string{"--hostlogging=filesystem", fmt.Sprintf("--mount=%s:/", bearDir)},
			wasmArgs:       []string{"/bear.txt"},
			expectedStdout: "pooh\n",
			expectedStderr: fmt.Sprintf(`==> go.syscall/js.valueCall(fs.open(path=/bear.txt,flags=,perm=----------))
<== (err=<nil>,fd=4)
==> go.syscall/js.valueCall(fs.fstat(fd=4))
<== (err=<nil>,stat={isDir=false,mode=%[1]s,size=5,mtimeMs=%[2]d})
==> go.syscall/js.valueCall(fs.fstat(fd=4))
<== (err=<nil>,stat={isDir=false,mode=%[1]s,size=5,mtimeMs=%[2]d})
==> go.syscall/js.valueCall(fs.read(fd=4,offset=0,byteCount=512,fOffset=<nil>))
<== (err=<nil>,n=5)
==> go.syscall/js.valueCall(fs.read(fd=4,offset=0,byteCount=507,fOffset=<nil>))
<== (err=<nil>,n=0)
==> go.syscall/js.valueCall(fs.close(fd=4))
<== (err=<nil>,ok=true)
`, bearMode, bearMtime),
		},
		{
			name:       "cachedir existing absolute",
			wazeroOpts: []string{"--cachedir=" + existingDir1},
			wasm:       wasmWasiArg,
			wasmArgs:   []string{"hello world"},
			// Executable name is first arg so is printed.
			expectedStdout: "test.wasm\x00hello world\x00",
			test: func(t *testing.T) {
				entries, err := os.ReadDir(existingDir1)
				require.NoError(t, err)
				require.True(t, len(entries) > 0)
			},
		},
		{
			name:       "cachedir existing relative",
			wazeroOpts: []string{"--cachedir=existing2"},
			wasm:       wasmWasiArg,
			wasmArgs:   []string{"hello world"},
			// Executable name is first arg so is printed.
			expectedStdout: "test.wasm\x00hello world\x00",
			test: func(t *testing.T) {
				entries, err := os.ReadDir(existingDir2)
				require.NoError(t, err)
				require.True(t, len(entries) > 0)
			},
		},
		{
			name:       "cachedir new absolute",
			wazeroOpts: []string{"--cachedir=" + path.Join(tmpDir, "new1")},
			wasm:       wasmWasiArg,
			wasmArgs:   []string{"hello world"},
			// Executable name is first arg so is printed.
			expectedStdout: "test.wasm\x00hello world\x00",
			test: func(t *testing.T) {
				entries, err := os.ReadDir("new1")
				require.NoError(t, err)
				require.True(t, len(entries) > 0)
			},
		},
		{
			name:       "cachedir new relative",
			wazeroOpts: []string{"--cachedir=new2"},
			wasm:       wasmWasiArg,
			wasmArgs:   []string{"hello world"},
			// Executable name is first arg so is printed.
			expectedStdout: "test.wasm\x00hello world\x00",
			test: func(t *testing.T) {
				entries, err := os.ReadDir("new2")
				require.NoError(t, err)
				require.True(t, len(entries) > 0)
			},
		},
	}

	cryptoTest := test{
		name:       "GOARCH=wasm GOOS=js hostlogging=filesystem,random",
		wasm:       wasmCatGo,
		wazeroOpts: []string{"--hostlogging=filesystem,random"},
		wasmArgs:   []string{"/bear.txt"},
		expectedStderr: `==> go.runtime.getRandomData(r_len=32)
<==
==> go.runtime.getRandomData(r_len=8)
<==
==> go.syscall/js.valueCall(fs.open(path=/bear.txt,flags=,perm=----------))
<== (err=function not implemented,fd=0)
`, // Test only shows logging happens in two scopes; it is ok to fail.
		expectedExitCode: 1,
	}

	for _, tt := range append(tests, cryptoTest) {
		tc := tt

		if tc.wasm == nil {
			// We should only skip when the runtime is a scratch image.
			require.False(t, platform.CompilerSupported())
			continue
		}
		t.Run(tc.name, func(t *testing.T) {
			wasmPath := filepath.Join(tmpDir, "test.wasm")
			require.NoError(t, os.WriteFile(wasmPath, tc.wasm, 0o700))

			args := append([]string{"run"}, tc.wazeroOpts...)
			args = append(args, wasmPath)
			args = append(args, tc.wasmArgs...)
			exitCode, stdOut, stdErr := runMain(t, args)

			// TODO: Go 1.17 initializes randoms in a different order than Go 1.18,19
			// When we move to 1.20, remove the workaround.
			if tc.name == cryptoTest.name {
				if tc.expectedStderr != stdErr {
					require.Equal(t, `==> go.runtime.getRandomData(r_len=8)
<==
==> go.runtime.getRandomData(r_len=32)
<==
==> go.syscall/js.valueCall(fs.open(path=/bear.txt,flags=,perm=----------))
<== (err=function not implemented,fd=0)
`, stdErr)
				}
			} else {
				require.Equal(t, tc.expectedStderr, stdErr)
			}

			require.Equal(t, tc.expectedExitCode, exitCode, stdErr)
			require.Equal(t, tc.expectedStdout, stdOut)
			if test := tc.test; test != nil {
				test(t)
			}
		})
	}
}

func TestVersion(t *testing.T) {
	exitCode, stdOut, stdErr := runMain(t, []string{"version"})
	require.Equal(t, 0, exitCode)
	require.Equal(t, version.GetWazeroVersion()+"\n", stdOut)
	require.Equal(t, "", stdErr)
}

func TestRun_Errors(t *testing.T) {
	wasmPath := filepath.Join(t.TempDir(), "test.wasm")
	require.NoError(t, os.WriteFile(wasmPath, wasmWasiArg, 0o700))

	notWasmPath := filepath.Join(t.TempDir(), "bears.wasm")
	require.NoError(t, os.WriteFile(notWasmPath, []byte("pooh"), 0o700))

	tests := []struct {
		message string
		args    []string
	}{
		{
			message: "missing path to wasm file",
			args:    []string{},
		},
		{
			message: "error reading wasm binary",
			args:    []string{"non-existent.wasm"},
		},
		{
			message: "error compiling wasm binary",
			args:    []string{notWasmPath},
		},
		{
			message: "invalid environment variable",
			args:    []string{"--env=ANIMAL", "testdata/wasi_env.wasm"},
		},
		{
			message: "invalid mount", // not found
			args:    []string{"--mount=te", "testdata/wasi_env.wasm"},
		},
		{
			message: "invalid cachedir",
			args:    []string{"--cachedir", notWasmPath, wasmPath},
		},
	}

	for _, tc := range tests {
		tt := tc
		t.Run(tt.message, func(t *testing.T) {
			exitCode, _, stdErr := runMain(t, append([]string{"run"}, tt.args...))

			require.Equal(t, 1, exitCode)
			require.Contains(t, stdErr, tt.message)
		})
	}
}

var _ api.FunctionDefinition = importer{}

type importer struct {
	moduleName, name string
}

func (i importer) ModuleName() string { return "" }
func (i importer) Index() uint32      { return 0 }
func (i importer) Import() (moduleName, name string, isImport bool) {
	return i.moduleName, i.name, true
}
func (i importer) ExportNames() []string        { return nil }
func (i importer) Name() string                 { return "" }
func (i importer) DebugName() string            { return "" }
func (i importer) GoFunction() interface{}      { return nil }
func (i importer) ParamTypes() []api.ValueType  { return nil }
func (i importer) ParamNames() []string         { return nil }
func (i importer) ResultTypes() []api.ValueType { return nil }
func (i importer) ResultNames() []string        { return nil }

func Test_detectImports(t *testing.T) {
	tests := []struct {
		message                        string
		imports                        []api.FunctionDefinition
		expectNeedsWASI, expectNeedsGo bool
	}{
		{
			message: "no imports",
		},
		{
			message: "other imports",
			imports: []api.FunctionDefinition{
				importer{"env", "emscripten_notify_memory_growth"},
			},
		},
		{
			message: "wasi",
			imports: []api.FunctionDefinition{
				importer{wasi_snapshot_preview1.ModuleName, "fd_read"},
			},
			expectNeedsWASI: true,
		},
		{
			message: "GOARCH=wasm GOOS=js",
			imports: []api.FunctionDefinition{
				importer{"go", "syscall/js.valueCall"},
			},
			expectNeedsGo: true,
		},
	}

	for _, tc := range tests {
		tt := tc
		t.Run(tt.message, func(t *testing.T) {
			needsWASI, needsGo := detectImports(tc.imports)
			require.Equal(t, tc.expectNeedsWASI, needsWASI)
			require.Equal(t, tc.expectNeedsGo, needsGo)
		})
	}
}

func Test_logScopesFlag(t *testing.T) {
	tests := []struct {
		name     string
		values   []string
		expected logging.LogScopes
	}{
		{
			name:     "defaults to none",
			expected: logging.LogScopeNone,
		},
		{
			name:     "ignores empty",
			values:   []string{""},
			expected: logging.LogScopeNone,
		},
		{
			name:     "clock",
			values:   []string{"clock"},
			expected: logging.LogScopeClock,
		},
		{
			name:     "filesystem",
			values:   []string{"filesystem"},
			expected: logging.LogScopeFilesystem,
		},
		{
			name:     "poll",
			values:   []string{"poll"},
			expected: logging.LogScopePoll,
		},
		{
			name:     "random",
			values:   []string{"random"},
			expected: logging.LogScopeRandom,
		},
		{
			name:     "clock filesystem poll random",
			values:   []string{"clock", "filesystem", "poll", "random"},
			expected: logging.LogScopeClock | logging.LogScopeFilesystem | logging.LogScopePoll | logging.LogScopeRandom,
		},
		{
			name:     "clock,filesystem poll,random",
			values:   []string{"clock,filesystem", "poll,random"},
			expected: logging.LogScopeClock | logging.LogScopeFilesystem | logging.LogScopePoll | logging.LogScopeRandom,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			f := logScopesFlag(0)
			for _, v := range tc.values {
				require.NoError(t, f.Set(v))
			}
			require.Equal(t, tc.expected, logging.LogScopes(f))
		})
	}
}

func TestHelp(t *testing.T) {
	exitCode, _, stdErr := runMain(t, []string{"-h"})
	require.Equal(t, 0, exitCode)
	fmt.Println(stdErr)
	require.Equal(t, `wazero CLI

Usage:
  wazero <command>

Commands:
  compile	Pre-compiles a WebAssembly binary
  run		Runs a WebAssembly binary
  version	Displays the version of wazero CLI
`, stdErr)
}

func runMain(t *testing.T, args []string) (int, string, string) {
	t.Helper()
	oldArgs := os.Args
	t.Cleanup(func() {
		os.Args = oldArgs
	})
	os.Args = append([]string{"wazero"}, args...)

	var exitCode int
	stdOut := &bytes.Buffer{}
	stdErr := &bytes.Buffer{}
	var exited bool
	func() {
		defer func() {
			if r := recover(); r != nil {
				exited = true
			}
		}()
		flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
		doMain(stdOut, stdErr, func(code int) {
			exitCode = code
			panic(code)
		})
	}()

	require.True(t, exited)

	return exitCode, stdOut.String(), stdErr.String()
}

// compileGoJS compiles "testdata/cat/cat.go" on demand as the binary generated
// is too big (1.6MB) to check into the source tree.
func compileGoJS() (err error) {
	dir, err := os.Getwd()
	if err != nil {
		return err
	}

	srcDir := path.Join(dir, "testdata", "cat")
	outPath := path.Join(srcDir, "cat-go.wasm")

	// This doesn't add "-ldflags=-s -w", as the binary size only changes 28KB.
	cmd := exec.Command("go", "build", "-o", outPath, ".")
	cmd.Dir = srcDir
	cmd.Env = append(os.Environ(), "GOARCH=wasm", "GOOS=js", "GOWASM=satconv,signext")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("go build: %v\n%s", err, out)
	}

	wasmCatGo, err = os.ReadFile(outPath)
	return
}
