package main

func main() {
	a()
}

//export a
func a() {
	b()
}

//export b
func b() {
	c()
}

//export c
func c() {
	panic("NOOOOOOOOOOOOOOO")
}
