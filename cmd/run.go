package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// Updated result struct to include both base and head
type DualResult struct {
	Module string `json:"module"`
	Base   bool   `json:"base"`
	Head   bool   `json:"head"`
}

var (
	repo  string
	base  string
	head  string
	image string
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run downstream tests on base and head and detect regressions",
	RunE: func(cmd *cobra.Command, args []string) error {

		projectRoot, err := os.Getwd()
		if err != nil {
			return err
		}

		graterDir := filepath.Join(projectRoot, ".grater")
		modulesFile := filepath.Join(graterDir, "modules.txt")
		resultsFile := filepath.Join(graterDir, "results.json")

		dockerfilePath := filepath.Join(projectRoot, "docker", "dockerfile")
		dockerContext := filepath.Join(projectRoot, "docker")

		// Read modules.txt
		data, err := os.ReadFile(modulesFile)
		if err != nil {
			return fmt.Errorf("run prepare first: %w", err)
		}
		modules := strings.Split(strings.TrimSpace(string(data)), "\n")

		// Build docker image
		fmt.Println("Building docker image...")
		build := exec.Command(
			"docker", "build",
			"-t", image,
			"-f", dockerfilePath,
			dockerContext,
		)
		build.Stdout = os.Stdout
		build.Stderr = os.Stderr
		if err := build.Run(); err != nil {
			return fmt.Errorf("docker build failed: %w", err)
		}

		var allResults []map[string]string

		for _, m := range modules {
			fmt.Println("\n========================================")
			fmt.Println("Testing module:", m)
			fmt.Println("========================================")

			// Run ONE container per module that tests both refs
			dualResult, err := runDualContainer(image, m, repo, base, head)
			if err != nil {
				fmt.Printf("❌ Test failed: %v\n", err)
				allResults = append(allResults, map[string]string{
					"module": m,
					"status": "BROKEN",
				})
				continue
			}

			// Determine status based on base and head results
			status := "PASS"
			if dualResult.Base && !dualResult.Head {
				status = "REGRESSION"
			} else if !dualResult.Base {
				status = "BROKEN"
			}
			// else both true -> PASS, both false -> BROKEN (already covered)

			fmt.Printf("   Base (%s): %v\n", base, dualResult.Base)
			fmt.Printf("   Head (%s): %v\n", head, dualResult.Head)
			fmt.Printf("   Status: %s\n", status)

			allResults = append(allResults, map[string]string{
				"module": m,
				"status": status,
			})
		}

		out, _ := json.MarshalIndent(allResults, "", "  ")
		return os.WriteFile(resultsFile, out, 0644)
	},
}

// runDualContainer runs ONE container that tests both base and head refs
func runDualContainer(image, module, repo, baseRef, headRef string) (DualResult, error) {
	cmd := exec.Command(
		"docker", "run", "--rm",
		"-e", "MODULE="+module,
		"-e", "REPO="+repo,
		"-e", "BASE_REF="+baseRef,
		"-e", "HEAD_REF="+headRef,
		image,
	)

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		return DualResult{}, fmt.Errorf("container failed: %s", out.String())
	}

	var r DualResult
	if err := json.Unmarshal(out.Bytes(), &r); err != nil {
		return DualResult{}, fmt.Errorf("invalid JSON from container: %s", out.String())
	}

	return r, nil
}

func init() {
	rootCmd.AddCommand(runCmd)

	runCmd.Flags().StringVar(&repo, "repo", "", "Repo under test")
	runCmd.Flags().StringVar(&base, "base", "main", "Base git ref")
	runCmd.Flags().StringVar(&head, "head", "HEAD", "Head git ref")
	runCmd.Flags().StringVar(&image, "image", "grater-runner", "Docker image name")

	runCmd.MarkFlagRequired("repo")
}