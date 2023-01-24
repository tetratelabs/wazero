package writefs_test

import (
	_ "embed"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/experimental/writefs"
)

var config wazero.ModuleConfig //nolint

// This shows how to use writefs.NewDirFS to map paths relative to "/work/appA",
// as "/". Unlike os.DirFS, these paths will be writable.
func Example_dirFS() {
	fs := writefs.NewDirFS("/work/appA")
	config = wazero.NewModuleConfig().WithFS(fs)
}
