// Package main is a tiny example for catt's Go syntax highlighting.
package main

import (
	"fmt"
	"strings"
)

// greeting returns a cheerful message for the given name.
func greeting(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "world"
	}
	return fmt.Sprintf("Hello, %s!", name)
}

func main() {
	for _, name := range []string{"catt", "gopher", ""} {
		fmt.Println(greeting(name))
	}
}
