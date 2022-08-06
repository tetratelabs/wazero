package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
)

func main() {
	b := make([]byte, 5)
	if n, err := rand.Read(b); err != nil {
		log.Panicln(err)
	} else if n != 5 {
		log.Panicln("expected 5, but have ", n)
	}
	fmt.Println(hex.EncodeToString(b))
}
