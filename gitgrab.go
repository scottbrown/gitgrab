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

// CloneMethod represents the method used to clone repositories
type CloneMethod int

const (
	CloneMethodSSH CloneMethod = iota
	CloneMethodHTTP
)

func (c CloneMethod) String() string {
	switch c {
	case CloneMethodSSH:
		return "ssh"
	case CloneMethodHTTP:
		return "http"
	default:
		return "unknown"
	}
}

func ParseCloneMethod(s string) (CloneMethod, error) {
	switch strings.ToLower(s) {
	case "ssh":
		return CloneMethodSSH, nil
	case "http":
		return CloneMethodHTTP, nil
	default:
		return CloneMethodSSH, fmt.Errorf("invalid clone method: %s, defaulting to ssh", s)
	}
}

// URL types for type safety
type GitURL string
type HTTPURL string
type SSHURL string

func (u GitURL) String() string {
	return string(u)
}

func (u GitURL) IsValid() bool {
	s := string(u)
	return strings.HasPrefix(s, "git@") || strings.HasPrefix(s, "https://")
}

func (u HTTPURL) String() string {
	return string(u)
}

func (u HTTPURL) IsValid() bool {
	return strings.HasPrefix(string(u), "https://")
}

func (u SSHURL) String() string {
	return string(u)
}

func (u SSHURL) IsValid() bool {
	return strings.HasPrefix(string(u), "git@")
}

// GitHubToken represents a GitHub authentication token
type GitHubToken string

func (t GitHubToken) String() string {
	return string(t)
}

func (t GitHubToken) IsEmpty() bool {
	return string(t) == ""
}

func (t GitHubToken) AuthHeader() string {
	return "token " + string(t)
}

// OrganizationName represents a GitHub organization name
type OrganizationName string

func (o OrganizationName) String() string {
	return string(o)
}

func (o OrganizationName) IsValid() bool {
	s := string(o)
	return len(s) > 0 && !strings.ContainsAny(s, " /\\")
}

// RepositoryName represents a repository name
type RepositoryName string

func (r RepositoryName) String() string {
	return string(r)
}

func (r RepositoryName) IsValid() bool {
	s := string(r)
	return len(s) > 0 && !strings.ContainsAny(s, " /\\")
}

// BranchName represents a git branch name
type BranchName string

func (b BranchName) String() string {
	return string(b)
}

func (b BranchName) IsDefault() bool {
	s := string(b)
	return s == "main" || s == "master"
}

// CloneConfig groups all parameters needed for cloning
type CloneConfig struct {
	Repository   Repository
	TargetDir    string
	Token        GitHubToken
	Organization OrganizationName
	Method       CloneMethod
}

type Repository struct {
	Name          RepositoryName `json:"name"`
	CloneURL      HTTPURL        `json:"clone_url"`
	SSHURL        SSHURL         `json:"ssh_url"`
	Private       bool           `json:"private"`
	DefaultBranch BranchName     `json:"default_branch"`
}

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type GitHubClient struct {
	token  GitHubToken
	client HTTPClient
}

func NewGitHubClient(token GitHubToken) *GitHubClient {
	return &GitHubClient{
		token:  token,
		client: &http.Client{},
	}
}

func NewGitHubClientWithHTTPClient(token GitHubToken, client HTTPClient) *GitHubClient {
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

	req.Header.Set("Authorization", gc.token.AuthHeader())
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "GitHub-Repo-Cloner")

	return gc.client.Do(req)
}

func (gc *GitHubClient) FetchAllRepos(orgName OrganizationName) ([]Repository, error) {
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

func CloneRepo(config CloneConfig) error {
	repoPath := filepath.Join(config.TargetDir, config.Repository.Name.String())
	
	// Check if directory already exists
	if _, err := os.Stat(repoPath); err == nil {
		fmt.Printf("  Directory %s already exists, updating...\n", config.Repository.Name)
		
		// Use default branch from the repository data (already fetched from API)
		defaultBranch := config.Repository.DefaultBranch
		if defaultBranch.String() == "" {
			fmt.Printf("  Warning: No default branch information for %s\n", config.Repository.Name)
			fmt.Printf("  Performing git fetch instead...\n")
			
			// Fallback to git fetch
			cmd := exec.Command("git", "-C", repoPath, "fetch")
			cmd.Stdout = nil
			cmd.Stderr = nil
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("failed to fetch %s: %v", config.Repository.Name, err)
			}
			fmt.Printf("  ✓ Fetched latest changes for %s\n", config.Repository.Name)
			return nil
		}
		
		// Get the current branch
		currentBranch, err := getCurrentBranch(repoPath)
		if err != nil {
			fmt.Printf("  Warning: Could not determine current branch for %s: %v\n", config.Repository.Name, err)
			fmt.Printf("  Performing git fetch instead...\n")
			
			// Fallback to git fetch
			cmd := exec.Command("git", "-C", repoPath, "fetch")
			cmd.Stdout = nil
			cmd.Stderr = nil
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("failed to fetch %s: %v", config.Repository.Name, err)
			}
			fmt.Printf("  ✓ Fetched latest changes for %s\n", config.Repository.Name)
			return nil
		}
		
		// Perform git pull if on default branch, git fetch otherwise
		if BranchName(currentBranch) == defaultBranch {
			fmt.Printf("  On default branch (%s), performing git pull...\n", defaultBranch)
			cmd := exec.Command("git", "-C", repoPath, "pull")
			cmd.Stdout = nil
			cmd.Stderr = nil
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("failed to pull %s: %v", config.Repository.Name, err)
			}
			fmt.Printf("  ✓ Pulled latest changes for %s\n", config.Repository.Name)
		} else {
			fmt.Printf("  On branch %s (not default), performing git fetch...\n", currentBranch)
			cmd := exec.Command("git", "-C", repoPath, "fetch")
			cmd.Stdout = nil
			cmd.Stderr = nil
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("failed to fetch %s: %v", config.Repository.Name, err)
			}
			fmt.Printf("  ✓ Fetched latest changes for %s\n", config.Repository.Name)
		}
		
		return nil
	}

	// Prepare clone URL based on clone method
	var cloneURL string
	if config.Method == CloneMethodSSH {
		cloneURL = config.Repository.SSHURL.String()
	} else {
		if config.Repository.Private {
			cloneURL = fmt.Sprintf("https://%s@github.com/%s/%s.git", config.Token, config.Organization, config.Repository.Name)
		} else {
			cloneURL = config.Repository.CloneURL.String()
		}
	}

	// Execute git clone
	cmd := exec.Command("git", "clone", cloneURL, repoPath)
	cmd.Stdout = nil // Suppress output
	cmd.Stderr = nil // Suppress error output

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to clone %s: %v", config.Repository.Name, err)
	}

	return nil
}