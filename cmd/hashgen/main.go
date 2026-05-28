package main

import (
	"fmt"
	"os"

	"golang.org/x/crypto/bcrypt"
)

func main() {
	pw := "test123"
	if len(os.Args) > 1 {
		pw = os.Args[1]
	}
	h, err := bcrypt.GenerateFromPassword([]byte(pw), 10)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(string(h))
}
