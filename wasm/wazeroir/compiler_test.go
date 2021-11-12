package wazeroir

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/wasm"
	"github.com/tetratelabs/wazero/wasm/spectests"
)

// Ensure Compile function works against all the well-formed binaries in spectests.
func TestCompile(t *testing.T) {
	const spectestsCases = "../spectests/cases"
	files, err := os.ReadDir(spectestsCases)
	require.NoError(t, err)

	for _, f := range files {
		if filepath.Ext(f.Name()) != ".json" {
			continue
		}
		jsonPath := filepath.Join(spectestsCases, f.Name())
		raw, err := os.ReadFile(jsonPath)
		require.NoError(t, err)

		var base spectests.Testbase
		require.NoError(t, json.Unmarshal(raw, &base))
		wastName := filepath.Base(base.SourceFile)
		t.Run(wastName, func(t *testing.T) {
			store := wasm.NewStore(&wasm.NopEngine{})
			require.NoError(t, spectests.AddSpectestModule(store))
			var lastInstanceName string

			for _, c := range base.Commands {
				if c.CommandType == "register" {
					name := lastInstanceName
					if c.Name != "" {
						name = c.Name
					}
					store.ModuleInstances[c.As] = store.ModuleInstances[name]
				} else if c.CommandType == "module" {
					msg := fmt.Sprintf("%s:%d", wastName, c.Line)
					buf, err := os.ReadFile(filepath.Join(spectestsCases, c.Filename))
					require.NoError(t, err, msg)

					mod, err := wasm.DecodeModule(buf)
					require.NoError(t, err, msg)

					lastInstanceName = c.Name
					if lastInstanceName == "" {
						lastInstanceName = c.Filename
					}
					err = store.Instantiate(mod, lastInstanceName)
					require.NoError(t, err, msg)

					for i, f := range store.ModuleInstances[lastInstanceName].Functions {
						if f.HostFunction != nil {
							// Host function doesn't need to be compiled to wazeroir.
							continue
						}
						res, err := Compile(f)
						require.NoError(t, err, msg)
						t.Logf("%s %d/%d:\n%s", msg,
							i, len(store.ModuleInstances[lastInstanceName].Functions)-1,
							Disassemble(res))
					}
				}
			}
		})
	}
}
