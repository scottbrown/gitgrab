package main

import (
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "os"
    "os/exec"
    "path/filepath"
    "strings"

    "github.com/spf13/cobra"
)

type Repository struct {
    Name     string `json:"name"`
    CloneURL string `json:"clone_url"`
    Private  bool   `json:"private"`
}

type GitHubClient struct {
    token  string
    client *http.Client
}

func NewGitHubClient(token string) *GitHubClient {
    return &GitHubClient{
        token:  token,
        client: &http.Client{},
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

func (gc *GitHubClient) fetchAllRepos(orgName string) ([]Repository, error) {
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

func cloneRepo(repo Repository, targetDir, token, orgName string) error {
    repoPath := filepath.Join(targetDir, repo.Name)
    
    // Check if directory already exists
    if _, err := os.Stat(repoPath); err == nil {
        fmt.Printf("  Directory %s already exists, skipping...\n", repo.Name)
        return nil
    }

    // Prepare clone URL with authentication
    var cloneURL string
    if repo.Private {
        cloneURL = fmt.Sprintf("https://%s@github.com/%s/%s.git", token, orgName, repo.Name)
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

var (
    orgName string
)

var rootCmd = &cobra.Command{
    Use:   "gitgrab [target_directory]",
    Short: "Clone all repositories from a GitHub organization",
    Long:  "GitGrab is a CLI utility that clones all GitHub repositories from a specified organization to a local directory.",
    Args:  cobra.ExactArgs(1),
    Run: func(cmd *cobra.Command, args []string) {
        targetDir := args[0]
        token := os.Getenv("GITHUB_TOKEN")
        
        if token == "" {
            fmt.Fprintf(os.Stderr, "Error: GITHUB_TOKEN environment variable is required\n")
            os.Exit(1)
        }

        // Check if git is available
        if _, err := exec.LookPath("git"); err != nil {
            fmt.Fprintf(os.Stderr, "Error: git is not installed or not in PATH\n")
            os.Exit(1)
        }

        // Create target directory if it doesn't exist
        if err := os.MkdirAll(targetDir, 0755); err != nil {
            fmt.Fprintf(os.Stderr, "Error creating directory %s: %v\n", targetDir, err)
            os.Exit(1)
        }

        fmt.Printf("Fetching repositories for %s organization...\n", orgName)
        fmt.Printf("Target directory: %s\n", targetDir)
        fmt.Println(strings.Repeat("-", 50))

        client := NewGitHubClient(token)
        repos, err := client.fetchAllRepos(orgName)
        if err != nil {
            fmt.Fprintf(os.Stderr, "Error fetching repositories: %v\n", err)
            os.Exit(1)
        }

        if len(repos) == 0 {
            fmt.Printf("No repositories found for %s organization\n", orgName)
            return
        }

        fmt.Printf("Found %d repositories\n\n", len(repos))

        successCount := 0
        failureCount := 0

        for i, repo := range repos {
            fmt.Printf("[%d/%d] Cloning %s...\n", i+1, len(repos), repo.Name)
            
            if err := cloneRepo(repo, targetDir, token, orgName); err != nil {
                fmt.Printf("  ✗ %v\n", err)
                failureCount++
            } else {
                fmt.Printf("  ✓ Successfully cloned %s\n", repo.Name)
                successCount++
            }
        }

        fmt.Println(strings.Repeat("-", 50))
        fmt.Printf("Completed! Success: %d, Failed: %d\n", successCount, failureCount)
    },
}

func init() {
    rootCmd.Flags().StringVarP(&orgName, "org", "o", "", "GitHub organization name")
    rootCmd.MarkFlagRequired("org")
}

func main() {
    if err := rootCmd.Execute(); err != nil {
        fmt.Fprintf(os.Stderr, "Error: %v\n", err)
        os.Exit(1)
    }
}
