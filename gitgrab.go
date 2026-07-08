package gitgrab

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"
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
	return strings.HasPrefix(string(u), "https://") && isSafeURL(string(u))
}

func (u SSHURL) String() string {
	return string(u)
}

func (u SSHURL) IsValid() bool {
	return strings.HasPrefix(string(u), "git@") && isSafeURL(string(u))
}

// isSafeURL reports whether a URL is safe to hand to `git clone` as a
// positional argument. It rejects the empty string, any leading '-' (which git
// would interpret as an option), and any embedded whitespace or control
// characters that could split or corrupt the command. This does not attempt
// full URL validation — it is a defensive guard against argument injection via
// hostile GitHub API responses.
func isSafeURL(s string) bool {
	if s == "" || strings.HasPrefix(s, "-") {
		return false
	}
	for _, r := range s {
		if r == unicode.ReplacementChar || unicode.IsSpace(r) || unicode.IsControl(r) {
			return false
		}
	}
	return true
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

// IsValid reports whether the organization name is safe to interpolate into a
// URL path. GitHub logins are limited to alphanumerics and single hyphens, and
// may not begin or end with a hyphen. Anything else (path separators, spaces,
// URL/shell metacharacters, control characters) is rejected so a hostile API
// response or flag value cannot smuggle in traversal or injection payloads.
func (o OrganizationName) IsValid() bool {
	return isSafeName(string(o), false)
}

// RepositoryName represents a repository name
type RepositoryName string

func (r RepositoryName) String() string {
	return string(r)
}

// IsValid reports whether the repository name is safe to use as a single path
// component and to interpolate into a clone URL. GitHub repository names allow
// alphanumerics plus '-', '_', and '.', but the names "." and ".." are
// rejected because they would escape the target directory when joined into a
// path. Path separators, spaces, and other metacharacters are also rejected.
func (r RepositoryName) IsValid() bool {
	s := string(r)
	if s == "." || s == ".." {
		return false
	}
	return isSafeName(s, true)
}

// isSafeName is the shared allowlist check for GitHub-style identifiers used in
// paths and URLs. When allowExtra is true the characters '_' and '.' are also
// permitted (repository names), otherwise only alphanumerics and '-' are
// allowed (organization logins). A leading or trailing '-' is always rejected,
// matching GitHub's own rules and avoiding names that could be parsed as CLI
// flags. The empty string is never valid.
func isSafeName(s string, allowExtra bool) bool {
	if s == "" {
		return false
	}
	if strings.HasPrefix(s, "-") || strings.HasSuffix(s, "-") {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-':
		case allowExtra && (r == '_' || r == '.'):
		default:
			return false
		}
	}
	return true
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

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read response: %v", err)
		}

		repos, err := parseRepos(body)
		if err != nil {
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

// parseRepos decodes a single page of the GitHub "list org repos" response.
// It is separated from FetchAllRepos so the JSON decoding path can be
// fuzzed against hostile or malformed API responses without any network I/O.
// Decoding is strict: unexpected trailing data after the JSON array is
// rejected rather than silently ignored.
func parseRepos(data []byte) ([]Repository, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	var repos []Repository
	if err := dec.Decode(&repos); err != nil {
		return nil, err
	}
	if dec.More() {
		return nil, fmt.Errorf("unexpected trailing data after JSON array")
	}
	return repos, nil
}

// resolveRepoPath joins a repository name onto the target directory, returning
// an error if the repository name is not a safe single path component or if the
// result would escape targetDir (path traversal). It is the single choke point
// for turning an untrusted repository name into a filesystem path, and is pure
// so it can be fuzzed directly.
func resolveRepoPath(targetDir string, name RepositoryName) (string, error) {
	if !name.IsValid() {
		return "", fmt.Errorf("invalid repository name: %q", name.String())
	}

	repoPath := filepath.Join(targetDir, name.String())

	// Defense in depth: even though IsValid already forbids separators and
	// "..", confirm the joined path stays directly within targetDir.
	cleanTarget := filepath.Clean(targetDir)
	if parent := filepath.Dir(repoPath); parent != cleanTarget {
		return "", fmt.Errorf("repository path escapes target directory: %q", repoPath)
	}

	return repoPath, nil
}

// buildCloneURL determines the git clone URL for a repository based on the
// chosen clone method, validating every component that originates from
// untrusted input (the GitHub API response and CLI flags). It never embeds a
// value that would be interpreted by git as an option or that contains
// command-splitting characters. It is pure so it can be fuzzed directly.
func buildCloneURL(config CloneConfig) (string, error) {
	repo := config.Repository

	if config.Method == CloneMethodSSH {
		if !repo.SSHURL.IsValid() {
			return "", fmt.Errorf("invalid ssh url for %q", repo.Name.String())
		}
		return repo.SSHURL.String(), nil
	}

	// HTTP method.
	if !repo.Private {
		if !repo.CloneURL.IsValid() {
			return "", fmt.Errorf("invalid clone url for %q", repo.Name.String())
		}
		return repo.CloneURL.String(), nil
	}

	// Private repo over HTTP: build a token-authenticated URL from validated
	// components so a hostile name/org cannot inject into the URL or command.
	if !config.Organization.IsValid() {
		return "", fmt.Errorf("invalid organization name: %q", config.Organization.String())
	}
	if !repo.Name.IsValid() {
		return "", fmt.Errorf("invalid repository name: %q", repo.Name.String())
	}
	if config.Token.IsEmpty() {
		return "", fmt.Errorf("token required for private repository over http")
	}
	if !isSafeURL(config.Token.String()) {
		return "", fmt.Errorf("token contains invalid characters")
	}

	url := fmt.Sprintf("https://%s@github.com/%s/%s.git",
		config.Token, config.Organization, repo.Name)
	if !isSafeURL(url) {
		return "", fmt.Errorf("constructed clone url is invalid")
	}
	return url, nil
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
	// Validate the repository name and resolve it to a contained path before
	// it is ever used as a filesystem path or passed to git.
	repoPath, err := resolveRepoPath(config.TargetDir, config.Repository.Name)
	if err != nil {
		return fmt.Errorf("failed to clone %s: %v", config.Repository.Name, err)
	}

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

	// Prepare and validate the clone URL based on clone method.
	cloneURL, err := buildCloneURL(config)
	if err != nil {
		return fmt.Errorf("failed to clone %s: %v", config.Repository.Name, err)
	}

	// Execute git clone. The "--" separator prevents a URL or path that begins
	// with "-" from being interpreted by git as an option (argument injection).
	cmd := exec.Command("git", "clone", "--", cloneURL, repoPath)
	cmd.Stdout = nil // Suppress output
	cmd.Stderr = nil // Suppress error output

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to clone %s: %v", config.Repository.Name, err)
	}

	return nil
}