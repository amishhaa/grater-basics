package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"grater-basics/internal"
)

var limit int
// DO NOT redeclare repo if it's already in run.go
// var repo string   <-- remove this

var findCmd = &cobra.Command{
	Use:   "find",
	Short: "Find modules in the workspace",
	Long:  "Search for modules in the .grater workspace",
	RunE: func(cmd *cobra.Command, args []string) error {
		wsDir := ".grater"
		modulesPath := filepath.Join(wsDir, "modules.txt")

		if err := os.MkdirAll(wsDir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", wsDir, err)
		}

		modules, err := internal.LoadModules(limit, repo)
		if err != nil {
			return err
		}

		content := strings.Join(modules, "\n")
		if len(modules) > 0 {
			content += "\n"
		}

		if err := os.WriteFile(modulesPath, []byte(content), 0644); err != nil {
			return fmt.Errorf("failed to write to %s: %w", modulesPath, err)
		}

		fmt.Printf("Successfully saved %d modules to %s\n", len(modules), modulesPath)
		return nil
	},
}

func init() {
	findCmd.Flags().IntVarP(&limit, "limit", "l", 0, "Limit the number of modules found")
	findCmd.Flags().StringVarP(&repo, "repo", "r", "", "Specify a repository to search for modules")
	rootCmd.AddCommand(findCmd)
}
