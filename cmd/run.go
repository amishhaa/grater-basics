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

// DualResult matches the detailed structure from runner.sh
type DualResult struct {
	Module string `json:"module"`
	Base   struct {
		Ref     string `json:"ref"`
		Passed  bool   `json:"passed"`
		Error   string `json:"error"`
		Skipped bool   `json:"skipped"`
	} `json:"base"`
	Head struct {
		Ref     string `json:"ref"`
		Passed  bool   `json:"passed"`
		Error   string `json:"error"`
		Skipped bool   `json:"skipped"`
	} `json:"head"`
}

// Simplified status for results.json
type ModuleStatus struct {
	Module string `json:"module"`
	Status string `json:"status"` // PASS, BROKEN, REGRESSION, FIXED, SKIPPED, ERROR
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

		// Create grater directory if it doesn't exist
		if err := os.MkdirAll(graterDir, 0755); err != nil {
			return fmt.Errorf("failed to create .grater directory: %w", err)
		}

		dockerfilePath := filepath.Join(projectRoot, "docker", "dockerfile")
		dockerContext := filepath.Join(projectRoot, "docker")
		data, err := os.ReadFile(modulesFile)
		if err != nil {
			return fmt.Errorf("run prepare first: %w", err)
		}
		modules := strings.Split(strings.TrimSpace(string(data)), "\n")

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

		var allResults []ModuleStatus
		var detailedResults []DualResult

		for i, m := range modules {
			fmt.Println("\n========================================")
			fmt.Printf("Testing module [%d/%d]: %s\n", i+1, len(modules), m)
			fmt.Println("========================================")

			dualResult, err := runDualContainer(image, m, repo, base, head)
			if err != nil {
				fmt.Printf("❌ Test failed: %v\n", err)
				allResults = append(allResults, ModuleStatus{
					Module: m,
					Status: "ERROR",
				})
				continue
			}

			// Determine status based on detailed results
			status := "PASS"
			if dualResult.Base.Skipped || dualResult.Head.Skipped {
				status = "SKIPPED"
			} else if dualResult.Base.Passed && !dualResult.Head.Passed {
				status = "REGRESSION"
			} else if !dualResult.Base.Passed && dualResult.Head.Passed {
				status = "FIXED"
			} else if !dualResult.Base.Passed && !dualResult.Head.Passed {
				status = "BROKEN"
			}

			// Print detailed results
			fmt.Printf("\n📊 Results for %s:\n", m)
			fmt.Printf("   Base (%s): ", dualResult.Base.Ref)
			if dualResult.Base.Skipped {
				fmt.Printf("⏰ SKIPPED - %s\n", dualResult.Base.Error)
			} else if dualResult.Base.Passed {
				fmt.Printf("✅ PASS\n")
			} else {
				fmt.Printf("❌ FAIL - %s\n", dualResult.Base.Error)
			}

			fmt.Printf("   Head (%s): ", dualResult.Head.Ref)
			if dualResult.Head.Skipped {
				fmt.Printf("⏰ SKIPPED - %s\n", dualResult.Head.Error)
			} else if dualResult.Head.Passed {
				fmt.Printf("✅ PASS\n")
			} else {
				fmt.Printf("❌ FAIL - %s\n", dualResult.Head.Error)
			}
			fmt.Printf("   Status: %s\n", status)

			allResults = append(allResults, ModuleStatus{
				Module: m,
				Status: status,
			})
			detailedResults = append(detailedResults, dualResult)
		}

		// Save simplified status results (for quick overview)
		simpleOut, _ := json.MarshalIndent(allResults, "", "  ")
		if err := os.WriteFile(resultsFile, simpleOut, 0644); err != nil {
			return fmt.Errorf("failed to write results: %w", err)
		}

		// Save detailed results (for report command)
		detailedFile := filepath.Join(graterDir, "detailed_results.json")
		detailedOut, _ := json.MarshalIndent(detailedResults, "", "  ")
		if err := os.WriteFile(detailedFile, detailedOut, 0644); err != nil {
			return fmt.Errorf("failed to write detailed results: %w", err)
		}

		fmt.Printf("\n✅ Results saved to %s and %s\n", resultsFile, detailedFile)
		return nil
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

	// Ensure refs are set (in case container didn't populate them)
	if r.Base.Ref == "" {
		r.Base.Ref = baseRef
	}
	if r.Head.Ref == "" {
		r.Head.Ref = headRef
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