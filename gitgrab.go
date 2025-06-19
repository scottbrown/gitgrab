package gitgrab

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Repository struct {
	Name          string `json:"name"`
	CloneURL      string `json:"clone_url"`
	SSHURL        string `json:"ssh_url"`
	Private       bool   `json:"private"`
	DefaultBranch string `json:"default_branch"`
}

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type GitHubClient struct {
	token  string
	client HTTPClient
}

func NewGitHubClient(token string) *GitHubClient {
	return &GitHubClient{
		token:  token,
		client: &http.Client{},
	}
}

func NewGitHubClientWithHTTPClient(token string, client HTTPClient) *GitHubClient {
	return &GitHubClient{
		token:  token,
		client: client,
	}
}

func (gc *GitHubClient) makeRequest(url string) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "token "+gc.token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "GitHub-Repo-Cloner")

	return gc.client.Do(req)
}

func (gc *GitHubClient) FetchAllRepos(orgName string) ([]Repository, error) {
	var allRepos []Repository
	page := 1
	perPage := 100

	for {
		url := fmt.Sprintf("https://api.github.com/orgs/%s/repos?page=%d&per_page=%d&type=all", orgName, page, perPage)
		
		resp, err := gc.makeRequest(url)
		if err != nil {
			return nil, fmt.Errorf("failed to make request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("API request failed: %s - %s", resp.Status, string(body))
		}

		var repos []Repository
		if err := json.NewDecoder(resp.Body).Decode(&repos); err != nil {
			return nil, fmt.Errorf("failed to decode response: %v", err)
		}

		if len(repos) == 0 {
			break
		}

		allRepos = append(allRepos, repos...)
		page++
	}

	return allRepos, nil
}

func getCurrentBranch(repoPath string) (string, error) {
	cmd := exec.Command("git", "-C", repoPath, "branch", "--show-current")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	
	return strings.TrimSpace(string(output)), nil
}

func CloneRepo(repo Repository, targetDir, token, orgName, cloneMethod string) error {
	repoPath := filepath.Join(targetDir, repo.Name)
	
	// Check if directory already exists
	if _, err := os.Stat(repoPath); err == nil {
		fmt.Printf("  Directory %s already exists, updating...\n", repo.Name)
		
		// Use default branch from the repository data (already fetched from API)
		defaultBranch := repo.DefaultBranch
		if defaultBranch == "" {
			fmt.Printf("  Warning: No default branch information for %s\n", repo.Name)
			fmt.Printf("  Performing git fetch instead...\n")
			
			// Fallback to git fetch
			cmd := exec.Command("git", "-C", repoPath, "fetch")
			cmd.Stdout = nil
			cmd.Stderr = nil
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("failed to fetch %s: %v", repo.Name, err)
			}
			fmt.Printf("  ✓ Fetched latest changes for %s\n", repo.Name)
			return nil
		}
		
		// Get the current branch
		currentBranch, err := getCurrentBranch(repoPath)
		if err != nil {
			fmt.Printf("  Warning: Could not determine current branch for %s: %v\n", repo.Name, err)
			fmt.Printf("  Performing git fetch instead...\n")
			
			// Fallback to git fetch
			cmd := exec.Command("git", "-C", repoPath, "fetch")
			cmd.Stdout = nil
			cmd.Stderr = nil
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("failed to fetch %s: %v", repo.Name, err)
			}
			fmt.Printf("  ✓ Fetched latest changes for %s\n", repo.Name)
			return nil
		}
		
		// Perform git pull if on default branch, git fetch otherwise
		if currentBranch == defaultBranch {
			fmt.Printf("  On default branch (%s), performing git pull...\n", defaultBranch)
			cmd := exec.Command("git", "-C", repoPath, "pull")
			cmd.Stdout = nil
			cmd.Stderr = nil
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("failed to pull %s: %v", repo.Name, err)
			}
			fmt.Printf("  ✓ Pulled latest changes for %s\n", repo.Name)
		} else {
			fmt.Printf("  On branch %s (not default), performing git fetch...\n", currentBranch)
			cmd := exec.Command("git", "-C", repoPath, "fetch")
			cmd.Stdout = nil
			cmd.Stderr = nil
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("failed to fetch %s: %v", repo.Name, err)
			}
			fmt.Printf("  ✓ Fetched latest changes for %s\n", repo.Name)
		}
		
		return nil
	}

	// Prepare clone URL based on clone method
	var cloneURL string
	if cloneMethod == "ssh" {
		cloneURL = repo.SSHURL
	} else {
		if repo.Private {
			cloneURL = fmt.Sprintf("https://%s@github.com/%s/%s.git", token, orgName, repo.Name)
		} else {
			cloneURL = repo.CloneURL
		}
	}

	// Execute git clone
	cmd := exec.Command("git", "clone", cloneURL, repoPath)
	cmd.Stdout = nil // Suppress output
	cmd.Stderr = nil // Suppress error output

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to clone %s: %v", repo.Name, err)
	}

	return nil
}