package main

import (
	"fmt"
	"os"

	"github.com/tetratelabs/wazero/internal/gojs/testdata/argsenv"
	"github.com/tetratelabs/wazero/internal/gojs/testdata/crypto"
	"github.com/tetratelabs/wazero/internal/gojs/testdata/fs"
	"github.com/tetratelabs/wazero/internal/gojs/testdata/goroutine"
	"github.com/tetratelabs/wazero/internal/gojs/testdata/http"
	"github.com/tetratelabs/wazero/internal/gojs/testdata/mem"
	"github.com/tetratelabs/wazero/internal/gojs/testdata/stdio"
	"github.com/tetratelabs/wazero/internal/gojs/testdata/syscall"
	"github.com/tetratelabs/wazero/internal/gojs/testdata/time"
)

// main includes a registry of all tests to reduce compilation time.
func main() {
	switch os.Args[1] {
	case "argsenv":
		argsenv.Main()
	case "crypto":
		crypto.Main()
	case "fs":
		fs.Main()
	case "goroutine":
		goroutine.Main()
	case "http":
		http.Main()
	case "mem":
		mem.Main()
	case "stdio":
		stdio.Main()
	case "syscall":
		syscall.Main()
	case "time":
		time.Main()
	default:
		panic(fmt.Errorf("unsupported arg: %s", os.Args[1]))
	}
}
