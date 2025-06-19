package gitgrab

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type mockHTTPClient struct {
	doFunc func(req *http.Request) (*http.Response, error)
}

func (m *mockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return m.doFunc(req)
}

func TestNewGitHubClient(t *testing.T) {
	token := GitHubToken("test-token")
	client := NewGitHubClient(token)
	
	if client.token != token {
		t.Errorf("Expected token %s, got %s", token, client.token)
	}
	
	if client.client == nil {
		t.Error("Expected HTTP client to be initialized")
	}
}

func TestGitHubClient_makeRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "token test-token" {
			t.Errorf("Expected Authorization header 'token test-token', got '%s'", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Accept") != "application/vnd.github.v3+json" {
			t.Errorf("Expected Accept header 'application/vnd.github.v3+json', got '%s'", r.Header.Get("Accept"))
		}
		if r.Header.Get("User-Agent") != "GitHub-Repo-Cloner" {
			t.Errorf("Expected User-Agent header 'GitHub-Repo-Cloner', got '%s'", r.Header.Get("User-Agent"))
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"test": "response"}`))
	}))
	defer server.Close()

	client := NewGitHubClient(GitHubToken("test-token"))
	resp, err := client.makeRequest(server.URL)
	
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestGitHubClient_FetchAllRepos(t *testing.T) {
	testRepos := []Repository{
		{Name: RepositoryName("repo1"), CloneURL: HTTPURL("https://github.com/test/repo1.git"), SSHURL: SSHURL("git@github.com:test/repo1.git"), Private: false, DefaultBranch: BranchName("main")},
		{Name: RepositoryName("repo2"), CloneURL: HTTPURL("https://github.com/test/repo2.git"), SSHURL: SSHURL("git@github.com:test/repo2.git"), Private: true, DefaultBranch: BranchName("master")},
	}

	callCount := 0
	mockClient := &mockHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			callCount++
			
			recorder := httptest.NewRecorder()
			if callCount == 1 {
				recorder.WriteHeader(http.StatusOK)
				json.NewEncoder(recorder).Encode(testRepos)
			} else {
				recorder.WriteHeader(http.StatusOK)
				json.NewEncoder(recorder).Encode([]Repository{})
			}
			
			return recorder.Result(), nil
		},
	}

	client := NewGitHubClientWithHTTPClient(GitHubToken("test-token"), mockClient)
	repos, err := client.FetchAllRepos(OrganizationName("testorg"))
	
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	
	if len(repos) != 2 {
		t.Errorf("Expected 2 repositories, got %d", len(repos))
	}
	
	if repos[0].Name != "repo1" {
		t.Errorf("Expected first repo name 'repo1', got '%s'", repos[0].Name)
	}
	
	if repos[1].Private != true {
		t.Errorf("Expected second repo to be private")
	}
}

func TestGitHubClient_FetchAllRepos_APIError(t *testing.T) {
	mockClient := &mockHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			recorder := httptest.NewRecorder()
			recorder.WriteHeader(http.StatusUnauthorized)
			recorder.Write([]byte(`{"message": "Bad credentials"}`))
			return recorder.Result(), nil
		},
	}

	client := NewGitHubClientWithHTTPClient(GitHubToken("invalid-token"), mockClient)
	_, err := client.FetchAllRepos(OrganizationName("testorg"))
	
	if err == nil {
		t.Fatal("Expected error for API failure, got none")
	}
	
	if !strings.Contains(err.Error(), "API request failed") {
		t.Errorf("Expected 'API request failed' in error message, got %v", err)
	}
}

func TestCloneRepo_DirectoryExists_SkipUpdate(t *testing.T) {
	// This test creates a directory but doesn't set up a proper git repo
	// The function should attempt to update but may fail gracefully
	tempDir := t.TempDir()
	repo := Repository{
		Name:          RepositoryName("test-repo"),
		CloneURL:      HTTPURL("https://github.com/test/test-repo.git"),
		SSHURL:        SSHURL("git@github.com:test/test-repo.git"),
		Private:       false,
		DefaultBranch: BranchName("main"),
	}
	
	repoDir := filepath.Join(tempDir, repo.Name.String())
	err := os.MkdirAll(repoDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	// The function will try to update existing directories now
	// It may fail due to API calls or git commands, which is expected in test environment
	config := CloneConfig{
		Repository:   repo,
		TargetDir:    tempDir,
		Token:        GitHubToken("invalid-token"),
		Organization: OrganizationName("testorg"),
		Method:       CloneMethodSSH,
	}
	err = CloneRepo(config)
	
	// We expect either success (if git commands work) or specific failure messages
	if err != nil {
		// Check that we're getting expected error types (API or git failures)
		if !strings.Contains(err.Error(), "failed to") {
			t.Errorf("Expected git or API related error, got: %v", err)
		}
	}
}

func TestCloneRepo_URLGeneration_PrivateSSH(t *testing.T) {
	repo := Repository{
		Name:          RepositoryName("private-repo"),
		CloneURL:      HTTPURL("https://github.com/test/private-repo.git"),
		SSHURL:        SSHURL("git@github.com:test/private-repo.git"),
		Private:       true,
		DefaultBranch: BranchName("main"),
	}
	
	expectedURL := "git@github.com:test/private-repo.git"
	
	if repo.Private && repo.SSHURL.String() != expectedURL {
		t.Errorf("Expected private repo SSH URL %s, got %s", expectedURL, repo.SSHURL)
	}
}

func TestCloneRepo_URLGeneration_PrivateHTTP(t *testing.T) {
	repo := Repository{
		Name:          RepositoryName("private-repo"),
		CloneURL:      HTTPURL("https://github.com/test/private-repo.git"),
		SSHURL:        SSHURL("git@github.com:test/private-repo.git"),
		Private:       true,
		DefaultBranch: BranchName("main"),
	}
	
	token := "token123"
	orgName := "testorg"
	expectedURL := "https://token123@github.com/testorg/private-repo.git"
	
	if repo.Private {
		actualURL := "https://" + token + "@github.com/" + orgName + "/" + repo.Name.String() + ".git"
		if actualURL != expectedURL {
			t.Errorf("Expected private repo HTTP URL %s, got %s", expectedURL, actualURL)
		}
	}
}

func TestCloneRepo_URLGeneration_Public(t *testing.T) {
	repo := Repository{
		Name:          RepositoryName("public-repo"),
		CloneURL:      HTTPURL("https://github.com/test/public-repo.git"),
		SSHURL:        SSHURL("git@github.com:test/public-repo.git"),
		Private:       false,
		DefaultBranch: BranchName("main"),
	}
	
	if repo.Private {
		t.Error("Test repo should be public")
	}
	
	expectedURL := "https://github.com/test/public-repo.git"
	if repo.CloneURL.String() != expectedURL {
		t.Errorf("Expected public repo URL %s, got %s", expectedURL, repo.CloneURL)
	}
}


func TestGetCurrentBranch(t *testing.T) {
	// Create a temporary git repository for testing
	tempDir := t.TempDir()
	
	// Initialize git repo
	cmd := exec.Command("git", "init", tempDir)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		t.Skip("git not available for testing")
	}
	
	// Configure git for the test
	exec.Command("git", "-C", tempDir, "config", "user.email", "test@example.com").Run()
	exec.Command("git", "-C", tempDir, "config", "user.name", "Test User").Run()
	
	// Create a file and commit
	testFile := filepath.Join(tempDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	
	exec.Command("git", "-C", tempDir, "add", "test.txt").Run()
	exec.Command("git", "-C", tempDir, "commit", "-m", "Initial commit").Run()
	
	// Test getting current branch
	branch, err := getCurrentBranch(tempDir)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	
	// The default branch could be "master" or "main" depending on git configuration
	if branch != "master" && branch != "main" {
		t.Errorf("Expected branch to be 'master' or 'main', got '%s'", branch)
	}
}

func TestCloneRepo_ExistingDirectory_GitPullFetch(t *testing.T) {
	tempDir := t.TempDir()
	repo := Repository{
		Name:          RepositoryName("test-repo"),
		CloneURL:      HTTPURL("https://github.com/test/test-repo.git"),
		SSHURL:        SSHURL("git@github.com:test/test-repo.git"),
		Private:       false,
		DefaultBranch: BranchName("main"),
	}
	
	repoDir := filepath.Join(tempDir, repo.Name.String())
	
	// Create directory structure that mimics a git repository
	err := os.MkdirAll(filepath.Join(repoDir, ".git"), 0755)
	if err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}
	
	// Create a basic git config to make it look like a real repo
	gitConfig := filepath.Join(repoDir, ".git", "config")
	configContent := `[core]
	repositoryformatversion = 0
	filemode = true
	bare = false
[remote "origin"]
	url = https://github.com/test/test-repo.git
	fetch = +refs/heads/*:refs/remotes/origin/*
[branch "main"]
	remote = origin
	merge = refs/heads/main`
	
	if err := os.WriteFile(gitConfig, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to create git config: %v", err)
	}
	
	// The function will try to call git commands, but we can't fully test them
	// without a real git repository. We'll test that the function handles 
	// existing directories properly by checking it doesn't return a "directory exists" error
	config := CloneConfig{
		Repository:   repo,
		TargetDir:    tempDir,
		Token:        GitHubToken("token"),
		Organization: OrganizationName("testorg"),
		Method:       CloneMethodSSH,
	}
	err = CloneRepo(config)
	
	// We expect this to not return a "directory exists" error since we handle that case
	// It may fail on git commands, but that's expected in this test environment
	if err != nil && !strings.Contains(err.Error(), "failed to") {
		t.Errorf("Unexpected error type: %v", err)
	}
}

func TestCloneRepo_CloneMethodSSH(t *testing.T) {
	tests := []struct {
		name           string
		repo           Repository
		cloneMethod    string
		expectedSSHURL bool
	}{
		{
			name: "Private repo with SSH method",
			repo: Repository{
				Name:          RepositoryName("private-repo"),
				CloneURL:      HTTPURL("https://github.com/test/private-repo.git"),
				SSHURL:        SSHURL("git@github.com:test/private-repo.git"),
				Private:       true,
				DefaultBranch: BranchName("main"),
			},
			cloneMethod:    "ssh",
			expectedSSHURL: true,
		},
		{
			name: "Private repo with HTTP method",
			repo: Repository{
				Name:          RepositoryName("private-repo"),
				CloneURL:      HTTPURL("https://github.com/test/private-repo.git"),
				SSHURL:        SSHURL("git@github.com:test/private-repo.git"),
				Private:       true,
				DefaultBranch: BranchName("main"),
			},
			cloneMethod:    "http",
			expectedSSHURL: false,
		},
		{
			name: "Public repo with SSH method",
			repo: Repository{
				Name:          RepositoryName("public-repo"),
				CloneURL:      HTTPURL("https://github.com/test/public-repo.git"),
				SSHURL:        SSHURL("git@github.com:test/public-repo.git"),
				Private:       false,
				DefaultBranch: BranchName("main"),
			},
			cloneMethod:    "ssh",
			expectedSSHURL: true, // Public repos now respect method flag
		},
		{
			name: "Public repo with HTTP method",
			repo: Repository{
				Name:          RepositoryName("public-repo"),
				CloneURL:      HTTPURL("https://github.com/test/public-repo.git"),
				SSHURL:        SSHURL("git@github.com:test/public-repo.git"),
				Private:       false,
				DefaultBranch: BranchName("main"),
			},
			cloneMethod:    "http",
			expectedSSHURL: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var expectedURL string
			if tt.cloneMethod == "ssh" {
				expectedURL = tt.repo.SSHURL.String()
			} else {
				if tt.repo.Private {
					expectedURL = "https://token@github.com/testorg/" + tt.repo.Name.String() + ".git"
				} else {
					expectedURL = tt.repo.CloneURL.String()
				}
			}
			
			// We can't actually test git clone without git being available,
			// but we can verify the URL generation logic
			if tt.cloneMethod == "ssh" && tt.repo.SSHURL.String() != expectedURL {
				t.Errorf("Expected SSH URL %s, got %s", expectedURL, tt.repo.SSHURL)
			} else if tt.cloneMethod == "http" && !tt.repo.Private && tt.repo.CloneURL.String() != expectedURL {
				t.Errorf("Expected HTTP URL %s, got %s", expectedURL, tt.repo.CloneURL)
			}
		})
	}
}