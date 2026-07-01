package main

import (
	"os"

	"github.com/slamb2k/azrl/cmd"
)

func main() {
	if err := cmd.ExecuteGhrl(); err != nil {
		os.Exit(1)
	}
}
