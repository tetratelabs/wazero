package main

func main() {
	a()
}

func a() {
	b()
}

func b() {
	c()
}

func c() {
	panic("NOOOOOOOOOOOOOOO")
}
