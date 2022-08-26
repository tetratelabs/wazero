package argsenv

import (
	_ "flag" // to ensure flags parse
	"fmt"
	"os"
)

func Main() {
	fmt.Println()
	for i, a := range os.Args {
		fmt.Println("args", i, "=", a)
	}
	for i, e := range os.Environ() {
		fmt.Println("environ", i, "=", e)
	}
}
