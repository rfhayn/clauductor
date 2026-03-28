package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the Clauductor version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("clauductor v%s\n", Version)
	},
}
