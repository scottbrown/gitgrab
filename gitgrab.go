package gitgrab

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
)

type Repository struct {
	Name     string `json:"name"`
	CloneURL string `json:"clone_url"`
	SSHURL   string `json:"ssh_url"`
	Private  bool   `json:"private"`
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

func CloneRepo(repo Repository, targetDir, token, orgName, cloneMethod string) error {
	repoPath := filepath.Join(targetDir, repo.Name)
	
	// Check if directory already exists
	if _, err := os.Stat(repoPath); err == nil {
		fmt.Printf("  Directory %s already exists, skipping...\n", repo.Name)
		return nil
	}

	// Prepare clone URL based on repository privacy and clone method
	var cloneURL string
	if repo.Private {
		if cloneMethod == "ssh" {
			cloneURL = repo.SSHURL
		} else {
			cloneURL = fmt.Sprintf("https://%s@github.com/%s/%s.git", token, orgName, repo.Name)
		}
	} else {
		cloneURL = repo.CloneURL
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