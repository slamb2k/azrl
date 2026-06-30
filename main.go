package main

import (
	"os"

	"github.com/slamb2k/azrl/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
