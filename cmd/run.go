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

type Result struct {
	Module string `json:"module"`
	Passed bool   `json:"passed"`
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

		// ✅ Use current working directory (project root)
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
			fmt.Println("Testing:", m)

			baseRes, err := runContainer(image, m, repo, base)
			if err != nil {
				fmt.Println("Base failed:", err)
				allResults = append(allResults, map[string]string{
					"module": m,
					"status": "BROKEN",
				})
				continue
			}

			headRes, err := runContainer(image, m, repo, head)
			if err != nil {
				fmt.Println("Head failed:", err)
				allResults = append(allResults, map[string]string{
					"module": m,
					"status": "BROKEN",
				})
				continue
			}

			status := "PASS"
			if baseRes.Passed && !headRes.Passed {
				status = "REGRESSION"
			} else if !baseRes.Passed {
				status = "BROKEN"
			}

			fmt.Printf("%s => %s\n", m, status)

			allResults = append(allResults, map[string]string{
				"module": m,
				"status": status,
			})
		}

		out, _ := json.MarshalIndent(allResults, "", "  ")
		return os.WriteFile(resultsFile, out, 0644)
	},
}

func runContainer(image, module, repo, ref string) (Result, error) {
	cmd := exec.Command(
		"docker", "run", "--rm",
		"-e", "MODULE="+module,
		"-e", "REPO="+repo,
		"-e", "REF="+ref,
		image,
	)

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		return Result{}, fmt.Errorf("container failed: %s", out.String())
	}

	var r Result
	if err := json.Unmarshal(out.Bytes(), &r); err != nil {
		return Result{}, fmt.Errorf("invalid JSON from container: %s", out.String())
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
