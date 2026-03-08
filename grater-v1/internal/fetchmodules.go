package internal

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
)

type moduleInfo struct {
	Path  string  `json:"path"`
	Score float64 `json:"score"`
}

// Create a custom transport with proper settings for concurrent requests
var httpClient = &http.Client{
	Timeout: 10 * time.Second,
	Transport: &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
	},
}

func LoadModules(limit int, repo string) ([]string, error) {
	if repo == "" {
		out, err := exec.Command("git", "remote", "get-url", "origin").Output()
		if err != nil {
			return nil, fmt.Errorf("repo not provided: %w", err)
		}
		repo = strings.TrimSpace(string(out))
	}
	repo = cleanRepoURL(repo)
	return FetchAndRankWithScorecard(limit, repo)
}

func FetchAndRankWithScorecard(limit int, module string) ([]string, error) {
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

	cache := loadCache()
	var wg sync.WaitGroup
	var mu sync.Mutex
	scored := make([]moduleInfo, 0, len(roots))
	
	// Use a worker pool pattern instead of unbounded goroutines
	numWorkers := 5
	workChan := make(chan string, len(roots))
	
	// Start workers
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go worker(w, workChan, &wg, &mu, &scored, cache)
	}
	
	// Send work
	for _, p := range roots {
		workChan <- p
	}
	close(workChan)
	
	wg.Wait()
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

func worker(id int, jobs <-chan string, wg *sync.WaitGroup, mu *sync.Mutex, scored *[]moduleInfo, cache map[string]float64) {
	defer wg.Done()
	
	// Create a separate HTTP client for each worker to avoid connection contention
	workerClient := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			MaxIdleConnsPerHost: 2,
			DisableKeepAlives:   true, // Disable keep-alives to avoid connection sharing issues
		},
	}
	
	for path := range jobs {
		score, exists := cache[path]
		if !exists || score == 0.0 {
			score = fetchScorecardScoreWithClient(workerClient, path)
			if score > 0.0 {
				mu.Lock()
				cache[path] = score
				mu.Unlock()
			}
		}
		
		mu.Lock()
		*scored = append(*scored, moduleInfo{Path: path, Score: score})
		fmt.Printf("  [%d] [%3.2f] %s\n", len(*scored), score, path)
		mu.Unlock()
		
		// Small delay to prevent overwhelming the API
		time.Sleep(50 * time.Millisecond)
	}
}

func fetchScorecardScoreWithClient(client *http.Client, path string) float64 {
	parts := strings.Split(path, "/")
	if len(parts) < 3 || parts[0] != "github.com" {
		return 0.0
	}

	owner := parts[1]
	repo := parts[2]

	apiURL := fmt.Sprintf("https://api.securityscorecards.dev/projects/github.com/%s/%s", owner, repo)

	resp, err := client.Get(apiURL)
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

// Keep the original function for backward compatibility
func fetchScorecardScore(path string) float64 {
	return fetchScorecardScoreWithClient(httpClient, path)
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