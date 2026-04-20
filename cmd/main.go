package main

import (
	"fmt"
	"os"
	"strings"

	_ "github.com/lib/pq"
)

func main() {
	// Rewrite -od[=value] → --output-dir[=value] before cobra/pflag sees it,
	// because pflag only supports single-character shorthands.
	for i, arg := range os.Args[1:] {
		idx := i + 1
		if arg == "-od" {
			os.Args[idx] = "--output-dir"
		} else if strings.HasPrefix(arg, "-od=") {
			os.Args[idx] = "--output-dir=" + arg[4:]
		} else if strings.HasPrefix(arg, "-od") {
			os.Args[idx] = "--output-dir=" + arg[3:]
		}
	}

	err := RootCmd.Execute()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
