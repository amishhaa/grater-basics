package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

// prepareCmd represents the prepare command
var prepareCmd = &cobra.Command{
	Use:   "prepare",
	Short: "Prepare grater workspace",
	Long:  "Creates a .grater directory and generates required files",
	RunE: func(cmd *cobra.Command, args []string) error {

		// 1. Create .grater directory
		wsDir := ".grater"
		err := os.MkdirAll(wsDir, 0755)
		if err != nil {
			return err
		}

		// 2. Create modules.txt inside .grater
		modulesPath := filepath.Join(wsDir, "modules.txt")

		content := []byte("moduleA\nmoduleB\nmoduleC\n")

		err = os.WriteFile(modulesPath, content, 0644)
		if err != nil {
			return err
		}

		fmt.Println("✅ .grater workspace created")
		fmt.Println("✅ modules.txt generated at", modulesPath)

		return nil
	},
}

func init() {
	rootCmd.AddCommand(prepareCmd)
}
