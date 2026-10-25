package main

import (
	"os"

	"github.com/clauductor/clauductor/internal/cmd"
)

func main() {
	os.Exit(cmd.Main(os.Stderr))
}
