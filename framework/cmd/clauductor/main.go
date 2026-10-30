// Command clauductor is the old name's shim (OPS-104): a machine or a script set up before
// the rename still runs `clauductor`, so it runs the same CLI as cmd/stridal until it migrates.
package main

import (
	"os"

	"github.com/rfhayn/stridal/internal/cmd"
)

func main() {
	os.Exit(cmd.Main(os.Stderr))
}
