package internal

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

type moduleInfo struct {
	Path  string  `json:"path"`
	Score float64 `json:"score"`
}

var httpClient = &http.Client{Timeout: 10 * time.Second}

func LoadModules(limit int, repo string) ([]string, error) {
	if repo == "" {
		out, err := exec.Command("git", "remote", "get-url", "origin").Output()
		if err != nil {
			return nil, fmt.Errorf("repo not provided: %w", err)
		}
		repo = strings.TrimSpace(string(out))
	}
	repo = cleanRepoURL(repo)
	
	importers, err := findImporters(repo)
	if err != nil {
		return nil, err
	}
	
	if len(importers) == 0 {
		fmt.Println("⚠️ No importers found for this module")
		return []string{}, nil
	}
	
	return rankProjectsSequential(importers, limit)
}

func ScoreProjects(projects []string, limit int) ([]string, error) {
	if len(projects) == 0 {
		return []string{}, nil
	}
	return rankProjectsSequential(projects, limit)
}

func findImporters(module string) ([]string, error) {
	fmt.Printf("🔍 Fetching importers for: %s\n", module)
	url := fmt.Sprintf("https://pkg.go.dev/%s?tab=importedby", module)

	resp, err := httpClient.Get(url)
	if err != nil || resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to fetch from pkg.go.dev")
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	unique := make(map[string]bool)
	var roots []string

	doc.Find(".ImportedBy-details a").Each(func(i int, s *goquery.Selection) {
		path := strings.TrimSpace(s.Text())
		if strings.HasPrefix(strings.ToLower(path), "github.com/") {
			root := strings.ToLower(getRootModule(path))
			if !unique[root] {
				unique[root] = true
				roots = append(roots, root)
			}
		}
	})

	fmt.Printf("📡 Found %d unique projects.\n", len(roots))
	return roots, nil
}

func rankProjectsSequential(projects []string, limit int) ([]string, error) {
	cache := loadCache()
	scored := make([]moduleInfo, 0, len(projects))

	for i, path := range projects {
		score, exists := cache[path]
		if !exists || score == 0.0 {
			score = fetchScorecardScore(path)
			if score > 0.0 {
				cache[path] = score
			}
		}

		fmt.Printf("  [%d/%d] [%3.2f] %s\n", i+1, len(projects), score, path)
		
		scored = append(scored, moduleInfo{Path: path, Score: score})
		
		time.Sleep(100 * time.Millisecond)
	}
	
	saveCache(cache)

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].Score > scored[j].Score
	})

	results := []string{}
	for i := 0; i < len(scored) && (limit <= 0 || i < limit); i++ {
		results = append(results, scored[i].Path)
	}

	return results, nil
}

func fetchScorecardScore(path string) float64 {
	parts := strings.Split(path, "/")
	if len(parts) < 3 || parts[0] != "github.com" {
		return 0.0
	}

	owner := parts[1]
	repo := parts[2]

	apiURL := fmt.Sprintf("https://api.securityscorecards.dev/projects/github.com/%s/%s", owner, repo)

	resp, err := httpClient.Get(apiURL)
	if err != nil {
		return 0.0
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0.0
	}

	var result struct {
		Score float64 `json:"score"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0.0
	}

	return result.Score
}

func getRootModule(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) > 3 && parts[0] == "github.com" {
		return strings.Join(parts[:3], "/")
	}
	return path
}

func loadCache() map[string]float64 {
	cache := make(map[string]float64)
	data, err := os.ReadFile(".grater/cache.json")
	if err == nil {
		json.Unmarshal(data, &cache)
	}
	return cache
}

func saveCache(cache map[string]float64) {
	os.MkdirAll(".grater", 0755)
	data, _ := json.MarshalIndent(cache, "", "  ")
	os.WriteFile(".grater/cache.json", data, 0644)
}

func cleanRepoURL(url string) string {
	url = strings.TrimSuffix(url, ".git")
	url = strings.TrimPrefix(url, "https://")
	url = strings.TrimPrefix(url, "http://")
	if strings.HasPrefix(url, "git@") {
		url = strings.Replace(url, ":", "/", 1)
		url = strings.TrimPrefix(url, "git@")
	}
	return url
}