package wasi

import "github.com/tetratelabs/wazero"

var r = wazero.NewRuntime()

// Example_InstantiateSnapshotPreview1 shows how to instantiate ModuleSnapshotPreview1, which allows other modules to
// import functions such as "wasi_snapshot_preview1" "fd_write".
func Example_instantiateSnapshotPreview1() {
	wm, _ := InstantiateSnapshotPreview1(r)
	defer wm.Close()

	// Output:
}
