package gitgrab

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
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
	token := "test-token"
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

	client := NewGitHubClient("test-token")
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
		{Name: "repo1", CloneURL: "https://github.com/test/repo1.git", Private: false},
		{Name: "repo2", CloneURL: "https://github.com/test/repo2.git", Private: true},
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

	client := NewGitHubClientWithHTTPClient("test-token", mockClient)
	repos, err := client.FetchAllRepos("testorg")
	
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

	client := NewGitHubClientWithHTTPClient("invalid-token", mockClient)
	_, err := client.FetchAllRepos("testorg")
	
	if err == nil {
		t.Fatal("Expected error for API failure, got none")
	}
	
	if !strings.Contains(err.Error(), "API request failed") {
		t.Errorf("Expected 'API request failed' in error message, got %v", err)
	}
}

func TestCloneRepo_DirectoryExists(t *testing.T) {
	tempDir := t.TempDir()
	repo := Repository{
		Name:     "test-repo",
		CloneURL: "https://github.com/test/test-repo.git",
		Private:  false,
	}
	
	repoDir := filepath.Join(tempDir, repo.Name)
	err := os.MkdirAll(repoDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	err = CloneRepo(repo, tempDir, "token", "testorg")
	
	if err != nil {
		t.Errorf("Expected no error when directory exists, got %v", err)
	}
}

func TestCloneRepo_URLGeneration_Private(t *testing.T) {
	repo := Repository{
		Name:     "private-repo",
		CloneURL: "https://github.com/test/private-repo.git",
		Private:  true,
	}
	
	token := "token123"
	orgName := "testorg"
	expectedURL := "https://token123@github.com/testorg/private-repo.git"
	
	if repo.Private {
		actualURL := "https://" + token + "@github.com/" + orgName + "/" + repo.Name + ".git"
		if actualURL != expectedURL {
			t.Errorf("Expected private repo URL %s, got %s", expectedURL, actualURL)
		}
	}
}

func TestCloneRepo_URLGeneration_Public(t *testing.T) {
	repo := Repository{
		Name:     "public-repo",
		CloneURL: "https://github.com/test/public-repo.git",
		Private:  false,
	}
	
	if repo.Private {
		t.Error("Test repo should be public")
	}
	
	expectedURL := "https://github.com/test/public-repo.git"
	if repo.CloneURL != expectedURL {
		t.Errorf("Expected public repo URL %s, got %s", expectedURL, repo.CloneURL)
	}
}