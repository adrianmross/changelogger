package main

import (
	"fmt"
	"os"

	"github.com/adrianmross/changelogger/internal/changelogger"
)

func main() {
	if err := changelogger.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
