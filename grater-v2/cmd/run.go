/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>

*/
package cmd

import (
	"fmt"

	"grater-v2/internal"
	"github.com/spf13/cobra"
)

var (
	repo string
	base string
	head string
)

// runCmd represents the run command
var runCmd = &cobra.Command{
	Use:   "run",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("run called")

	},
}

func callRunOnRepo() {
	internal.RunTests(repo, base, head)
}

func init() {
	rootCmd.AddCommand(runCmd)
	runCmd.Flags().StringVarP(&repo, "repo", "r", "", "Repository to run the command against")
	runCmd.Flags().StringVarP(&base, "base", "b", "", "Base branch name")
	runCmd.Flags().StringVarP(&head, "head", "h", "", "Head branch name")
}
