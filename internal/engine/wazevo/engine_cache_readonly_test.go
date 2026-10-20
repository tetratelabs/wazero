package wazevo

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"unsafe"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/wazevoapi"
	"github.com/tetratelabs/wazero/internal/filecache"
	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

func TestReadOnlyCacheStopsBeforeGuestCompilation(t *testing.T) {
	// This deliberately incomplete module panics if the guest compiler is entered.
	// No compiler machinery is initialized in the engine below.
	module := &wasm.Module{FunctionSection: []wasm.Index{0}, CodeSection: []wasm.Code{{Body: []byte{0xff}}}}
	t.Run("compiler poison is effective", func(t *testing.T) {
		defer func() { require.True(t, recover() != nil) }()
		_, _ = (&engine{}).compileModule(context.Background(), module, nil, false)
	})
	encoded, err := io.ReadAll(serializeCompiledModule(testVersion, &compiledModule{
		executables: &executables{executable: []byte{1, 2, 3, 4}}, functionOffsets: []int{0},
	}))
	require.NoError(t, err)
	badChecksum := bytes.Clone(encoded)
	badChecksum[len(encoded)-9] ^= 0xff
	for _, tc := range []struct {
		name string
		data []byte
		want error
	}{
		{"missing", nil, filecache.ErrMiss},
		{"stale", concat(magic, []byte{byte(len(testVersion))}, []byte("9.9.9"), make([]byte, 4)), filecache.ErrStale},
		{"corrupt", []byte("broken"), filecache.ErrCorrupt},
		{"stale after mmap", encoded[:len(encoded)-4], filecache.ErrStale},
		{"checksum after mmap", badChecksum, filecache.ErrCorrupt},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			rw := filecache.New(dir)
			if tc.data != nil {
				require.NoError(t, rw.Add(fileCacheKey(module), bytes.NewReader(tc.data)))
			}
			before := snapshotCacheFiles(t, dir)
			e := &engine{fileCache: filecache.NewReadOnly(dir), wazeroVersion: testVersion}
			err := e.CompileModule(context.Background(), module, nil, false)
			require.True(t, errors.Is(err, tc.want), err)
			require.Equal(t, before, snapshotCacheFiles(t, dir))
			require.Equal(t, 0, len(e.compiledModules))
		})
	}
}

func snapshotCacheFiles(t *testing.T, dir string) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	ret := map[string]string{}
	for _, entry := range entries {
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		require.NoError(t, err)
		info, err := entry.Info()
		require.NoError(t, err)
		ret[entry.Name()] = string(data) + info.ModTime().String() + info.Mode().String()
	}
	return ret
}

func TestDeserializeCompiledModuleReleasesMapping(t *testing.T) {
	executable := []byte{1, 2, 3, 4}
	cm := &compiledModule{
		executables:     &executables{executable: executable},
		functionOffsets: []int{0},
		sourceMap: sourceMap{
			executableOffsets: []uintptr{uintptr(unsafe.Pointer(&executable[0]))},
			wasmBinaryOffsets: []uint64{0},
		},
		tryTableInfo: []wazevoapi.TryTableInfo{{
			NumLocals:    1,
			ReuseLocals:  true,
			CatchClauses: []wazevoapi.CatchClauseInstance{{Kind: 0, TagIndex: 0}},
		}},
	}
	data, err := io.ReadAll(serializeCompiledModule(testVersion, cm))
	require.NoError(t, err)
	// Every truncation after the mapping is allocated must unmap exactly once,
	// including the stale old-format path with no try-table data.
	codeStart := len(magic) + 1 + len(testVersion) + 4 + 8 + 8
	for end := codeStart; end < len(data); end++ {
		t.Run(strconv.Itoa(end-codeStart), func(t *testing.T) {
			unmapped := 0
			got, stale, err := deserializeCompiledModuleWithUnmap(testVersion, io.NopCloser(bytes.NewReader(data[:end])), func(b []byte) error {
				unmapped++
				require.Equal(t, 4, len(b))
				return platform.MunmapCodeSegment(b)
			})
			require.Nil(t, got)
			require.True(t, err != nil || stale)
			require.Equal(t, 1, unmapped)
		})
	}
	t.Run("checksum mismatch", func(t *testing.T) {
		corrupt := bytes.Clone(data)
		corrupt[codeStart] ^= 0xff
		unmapped := 0
		_, _, err := deserializeCompiledModuleWithUnmap(testVersion, io.NopCloser(bytes.NewReader(corrupt)), func(b []byte) error {
			unmapped++
			return platform.MunmapCodeSegment(b)
		})
		require.Error(t, err)
		require.Equal(t, 1, unmapped)
	})
	t.Run("successful load transfers ownership", func(t *testing.T) {
		got, stale, err := deserializeCompiledModuleWithUnmap(testVersion, io.NopCloser(bytes.NewReader(data)), func([]byte) error {
			t.Fatal("unexpected unmap")
			return nil
		})
		require.NoError(t, err)
		require.False(t, stale)
		require.NoError(t, platform.MunmapCodeSegment(got.executable))
	})
}

func TestDeserializeCompiledModuleEmptyExecutable(t *testing.T) {
	cm := &compiledModule{executables: &executables{}}
	got, stale, err := deserializeCompiledModule(testVersion, io.NopCloser(serializeCompiledModule(testVersion, cm)))
	require.NoError(t, err)
	require.False(t, stale)
	require.Equal(t, 0, len(got.executable))
}

func TestReadWriteCacheStillDeletesStale(t *testing.T) {
	dir := t.TempDir()
	fc := filecache.New(dir)
	module := &wasm.Module{}
	data := concat(magic, []byte{byte(len(testVersion))}, []byte("9.9.9"), make([]byte, 4))
	require.NoError(t, fc.Add(fileCacheKey(module), bytes.NewReader(data)))
	e := &engine{fileCache: fc, wazeroVersion: testVersion}
	_, hit, err := e.getCompiledModuleFromCache(module)
	require.NoError(t, err)
	require.False(t, hit)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Equal(t, 0, len(entries))
}

func TestReadOnlyCacheEntryDirectoryIsIO(t *testing.T) {
	dir := t.TempDir()
	module := &wasm.Module{FunctionSection: []wasm.Index{0}, CodeSection: []wasm.Code{{Body: []byte{0xff}}}}
	require.NoError(t, filecache.New(dir).Add(fileCacheKey(module), bytes.NewReader(nil)))
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	entry := filepath.Join(dir, entries[0].Name())
	require.NoError(t, os.Remove(entry))
	require.NoError(t, os.Mkdir(entry, 0o700))
	e := &engine{fileCache: filecache.NewReadOnly(dir), wazeroVersion: testVersion}
	err = e.CompileModule(context.Background(), module, nil, false)
	require.True(t, errors.Is(err, filecache.ErrIO), err)
	require.False(t, errors.Is(err, filecache.ErrCorrupt))
	info, err := os.Stat(entry)
	require.NoError(t, err)
	require.True(t, info.IsDir(), "failed entry must not be deleted")
}

type cacheReadFunc func([]byte) (int, error)

func (f cacheReadFunc) Read(p []byte) (int, error) { return f(p) }

func TestDeserializeCompiledModuleIOFailures(t *testing.T) {
	data, err := io.ReadAll(serializeCompiledModule(testVersion, &compiledModule{
		executables: &executables{executable: []byte{1, 2, 3, 4}}, functionOffsets: []int{0},
	}))
	require.NoError(t, err)
	failure := errors.New("storage read failed")
	codeStart := len(magic) + 1 + len(testVersion) + 4 + 8 + 8
	for _, tc := range []struct {
		name  string
		end   int
		unmap int
	}{
		{"header", 0, 0},
		{"code after mmap", codeStart, 1},
		{"try table after mmap", len(data) - 4, 1},
		{"complete data with error", len(data), 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var reader io.Reader
			if tc.end == len(data) {
				input := bytes.NewReader(data)
				reader = cacheReadFunc(func(p []byte) (int, error) {
					n, err := input.Read(p)
					if input.Len() == 0 {
						return n, failure
					}
					return n, err
				})
			} else {
				reader = io.MultiReader(bytes.NewReader(data[:tc.end]), cacheReadFunc(func([]byte) (int, error) {
					return 0, failure
				}))
			}
			unmapped := 0
			cm, stale, err := deserializeCompiledModuleWithUnmap(testVersion, io.NopCloser(reader), func(b []byte) error {
				unmapped++
				return platform.MunmapCodeSegment(b)
			})
			require.Nil(t, cm)
			require.False(t, stale, "storage failure must not trigger stale-entry deletion")
			require.True(t, errors.Is(err, filecache.ErrIO), err)
			require.True(t, errors.Is(err, failure), err)
			require.Equal(t, tc.unmap, unmapped)
		})
	}
}
