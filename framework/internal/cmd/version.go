package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the Stridal version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("stridal v%s\n", Version)
	},
}
