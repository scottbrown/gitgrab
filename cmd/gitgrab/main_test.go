package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scottbrown/gitgrab"
)

// testHTTPClient is a minimal HTTPClient for testing resolveToken.
type testHTTPClient struct {
	doFunc func(req *http.Request) (*http.Response, error)
}

func (c *testHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return c.doFunc(req)
}

// writeTestPEM generates an RSA key and writes it to a temp file, returning the path.
func writeTestPEM(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate test key: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	path := filepath.Join(t.TempDir(), "key.pem")
	if err := os.WriteFile(path, pemBytes, 0600); err != nil {
		t.Fatalf("failed to write PEM file: %v", err)
	}
	return path
}

// mockInstallationServer returns an HTTPClient that responds with a successful
// installation token response.
func mockInstallationServer(t *testing.T, token string) gitgrab.HTTPClient {
	t.Helper()
	return &testHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			recorder := httptest.NewRecorder()
			recorder.WriteHeader(http.StatusCreated)
			json.NewEncoder(recorder).Encode(map[string]string{"token": token})
			return recorder.Result(), nil
		},
	}
}

func TestResolveToken_PAT(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "ghp_test_pat")
	t.Setenv("GITHUB_APP_ID", "")
	t.Setenv("GITHUB_APP_PRIVATE_KEY", "")
	t.Setenv("GITHUB_APP_INSTALLATION_ID", "")

	token, err := resolveToken(&testHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			t.Error("HTTP client should not be called for PAT auth")
			return nil, nil
		},
	})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if token != gitgrab.GitHubToken("ghp_test_pat") {
		t.Errorf("expected PAT token, got %q", token)
	}
}

func TestResolveToken_GitHubApp(t *testing.T) {
	keyPath := writeTestPEM(t)

	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GITHUB_APP_ID", "12345")
	t.Setenv("GITHUB_APP_PRIVATE_KEY", keyPath)
	t.Setenv("GITHUB_APP_INSTALLATION_ID", "67890")

	token, err := resolveToken(mockInstallationServer(t, "ghs_app_token"))

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if token != gitgrab.GitHubToken("ghs_app_token") {
		t.Errorf("expected app token 'ghs_app_token', got %q", token)
	}
}

func TestResolveToken_AppTakesPrecedenceOverPAT(t *testing.T) {
	keyPath := writeTestPEM(t)

	t.Setenv("GITHUB_TOKEN", "ghp_should_not_be_used")
	t.Setenv("GITHUB_APP_ID", "12345")
	t.Setenv("GITHUB_APP_PRIVATE_KEY", keyPath)
	t.Setenv("GITHUB_APP_INSTALLATION_ID", "67890")

	token, err := resolveToken(mockInstallationServer(t, "ghs_app_wins"))

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if token != gitgrab.GitHubToken("ghs_app_wins") {
		t.Errorf("expected app token to take precedence, got %q", token)
	}
}

func TestResolveToken_NoCredentials(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GITHUB_APP_ID", "")
	t.Setenv("GITHUB_APP_PRIVATE_KEY", "")
	t.Setenv("GITHUB_APP_INSTALLATION_ID", "")

	_, err := resolveToken(&testHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			t.Error("HTTP client should not be called when no credentials are set")
			return nil, nil
		},
	})

	if err == nil {
		t.Fatal("expected error when no credentials configured, got nil")
	}
	if !strings.Contains(err.Error(), "no GitHub credentials configured") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestResolveToken_IncompleteAppCredentials(t *testing.T) {
	tests := []struct {
		name     string
		appID    string
		keyPath  string
		installID string
	}{
		{"missing installation ID", "12345", "/some/key.pem", ""},
		{"missing app ID", "", "/some/key.pem", "67890"},
		{"missing key path", "12345", "", "67890"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GITHUB_TOKEN", "")
			t.Setenv("GITHUB_APP_ID", tt.appID)
			t.Setenv("GITHUB_APP_PRIVATE_KEY", tt.keyPath)
			t.Setenv("GITHUB_APP_INSTALLATION_ID", tt.installID)

			_, err := resolveToken(&testHTTPClient{
				doFunc: func(req *http.Request) (*http.Response, error) {
					t.Error("HTTP client should not be called for incomplete credentials")
					return nil, nil
				},
			})

			if err == nil {
				t.Fatal("expected error for incomplete App credentials, got nil")
			}
			if !strings.Contains(err.Error(), "incomplete GitHub App credentials") {
				t.Errorf("unexpected error message: %v", err)
			}
		})
	}
}
