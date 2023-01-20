package cranelift

import (
	"log"
	"os"
	"runtime"
	"testing"
)

func TestMain(m *testing.M) {
	if runtime.GOARCH == "arm64" {
		os.Exit(m.Run())
	} else {
		log.Printf("Skipping unsuported %s/%s", runtime.GOARCH, runtime.GOOS)
	}
}
