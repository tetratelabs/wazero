package stdio

import (
	"bufio"
	"os"
)

func Main() {
	reader := bufio.NewReader(os.Stdin)
	input, _, err := reader.ReadLine()
	if err != nil {
		panic(err)
	}
	println("println", string(input))
	os.Stdout.Write([]byte("Stdout.Write"))
	os.Stderr.Write([]byte("Stderr.Write"))
}
