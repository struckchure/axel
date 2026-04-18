package main

import (
	"fmt"
	"os"

	_ "github.com/lib/pq"
	"github.com/struckchure/axel/cmd"
)

func main() {
	err := cmd.RootCmd.Execute()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
