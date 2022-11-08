package gc

import (
	"fmt"
	"runtime"
)

func Main() {
	fmt.Println("before gc")
	runtime.GC()
	fmt.Println("after gc")
}
