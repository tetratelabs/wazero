// Package testfs is an integration test of system calls mapped to the
// JavaScript object "fs". e.g. `go.syscall/js.valueCall(fs.openDir...`
package testfs

import (
	"log"
	"os"

	"github.com/tetratelabs/wazero/internal/fstest"
)

func Main() {
	if err := fstest.TestFS(os.DirFS("testfs")); err != nil {
		log.Panicln(err)
	}
}
