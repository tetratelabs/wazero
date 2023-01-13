package writefs_test

import (
	_ "embed"
	"log"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/experimental/writefs"
)

var config wazero.ModuleConfig //nolint

// This shows how to use writefs.NewDirFS to map paths relative to "/work/appA",
// as "/". Unlike os.DirFS, these paths will be writable.
func Example_dirFS() {
	fs, err := writefs.NewDirFS("/work/appA")
	if err != nil {
		log.Panicln(err)
	}
	config = wazero.NewModuleConfig().WithFS(fs)
}
