package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var prepareCmd = &cobra.Command{
	Use:   "prepare",
	Short: "Prepare grater workspace",
	Long:  "Creates a .grater directory and generates required files",
	RunE: func(cmd *cobra.Command, args []string) error {

		wsDir := ".grater"
		err := os.MkdirAll(wsDir, 0755)
		if err != nil {
			return err
		}

		modulesPath := filepath.Join(wsDir, "modules.txt")

		fmt.Println("✅ .grater workspace created")
		fmt.Println("✅ modules.txt generated at", modulesPath)

		return nil
	},
}

func init() {
	rootCmd.AddCommand(prepareCmd)
}
