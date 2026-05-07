package main

import (
	"os"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
