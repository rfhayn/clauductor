package main

import (
	"os"

	"github.com/rfhayn/stridal/internal/cmd"
)

func main() {
	os.Exit(cmd.Main(os.Stderr))
}
