package main

import (
	"fmt"
	"os"

	_ "github.com/lib/pq"
)

func main() {
	err := RootCmd.Execute()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
