package writefs_test

import (
	_ "embed"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/experimental/writefs"
)

var config wazero.ModuleConfig //nolint

// This shows how to use writefs.DirFS to map paths relative to "/work/appA",
// as "/". Unlike os.DirFS, these paths will be writable.
func Example_dirFS() {
	config = wazero.NewModuleConfig().WithFS(writefs.DirFS("/work/appA"))
}
