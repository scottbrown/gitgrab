package gitgrab

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
)

// writeTestKey generates a 2048-bit RSA key, writes it as PKCS#1 PEM to a
// temp file, and returns the key and its path.
func writeTestKey(t *testing.T) (*rsa.PrivateKey, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate test RSA key: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	path := filepath.Join(t.TempDir(), "test.pem")
	if err := os.WriteFile(path, pemBytes, 0600); err != nil {
		t.Fatalf("failed to write test key file: %v", err)
	}
	return key, path
}

func TestLoadPrivateKey_PKCS1(t *testing.T) {
	_, path := writeTestKey(t)
	key, err := loadPrivateKey(path)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if key == nil {
		t.Fatal("expected non-nil key")
	}
}

func TestLoadPrivateKey_PKCS8(t *testing.T) {
	rawKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	pkcs8Bytes, err := x509.MarshalPKCS8PrivateKey(rawKey)
	if err != nil {
		t.Fatalf("failed to marshal PKCS8 key: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: pkcs8Bytes,
	})
	path := filepath.Join(t.TempDir(), "pkcs8.pem")
	if err := os.WriteFile(path, pemBytes, 0600); err != nil {
		t.Fatalf("failed to write key file: %v", err)
	}

	key, err := loadPrivateKey(path)
	if err != nil {
		t.Fatalf("expected no error for PKCS8 key, got %v", err)
	}
	if key == nil {
		t.Fatal("expected non-nil key")
	}
}

func TestLoadPrivateKey_FileNotFound(t *testing.T) {
	_, err := loadPrivateKey("/nonexistent/path/key.pem")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
	if !strings.Contains(err.Error(), "failed to read private key file") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestLoadPrivateKey_InvalidPEM(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.pem")
	if err := os.WriteFile(path, []byte("this is not pem"), 0600); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
	_, err := loadPrivateKey(path)
	if err == nil {
		t.Fatal("expected error for invalid PEM, got nil")
	}
	if !strings.Contains(err.Error(), "no PEM block found") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestBuildJWT_Structure(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	jwt, err := buildJWT("12345", key)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		t.Fatalf("expected 3 JWT parts, got %d", len(parts))
	}

	// Each part must be non-empty.
	for i, p := range parts {
		if p == "" {
			t.Errorf("JWT part %d is empty", i)
		}
	}
}

func TestGetInstallationToken_Success(t *testing.T) {
	_, keyPath := writeTestKey(t)

	mockClient := &mockHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			// Verify the request shape.
			if req.Method != "POST" {
				t.Errorf("expected POST, got %s", req.Method)
			}
			if !strings.HasSuffix(req.URL.Path, "/access_tokens") {
				t.Errorf("unexpected path: %s", req.URL.Path)
			}
			auth := req.Header.Get("Authorization")
			if !strings.HasPrefix(auth, "Bearer ") {
				t.Errorf("expected Bearer token in Authorization header, got %q", auth)
			}

			recorder := httptest.NewRecorder()
			recorder.WriteHeader(http.StatusCreated)
			json.NewEncoder(recorder).Encode(map[string]string{"token": "ghs_test_installation_token"})
			return recorder.Result(), nil
		},
	}

	creds := GitHubAppCredentials{
		AppID:          "12345",
		PrivateKeyPath: keyPath,
		InstallationID: "67890",
	}

	token, err := GetInstallationToken(creds, mockClient)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if token != GitHubToken("ghs_test_installation_token") {
		t.Errorf("expected token 'ghs_test_installation_token', got %q", token)
	}
}

func TestGetInstallationToken_APIError(t *testing.T) {
	_, keyPath := writeTestKey(t)

	mockClient := &mockHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			recorder := httptest.NewRecorder()
			recorder.WriteHeader(http.StatusUnauthorized)
			recorder.Write([]byte(`{"message":"Bad credentials"}`))
			return recorder.Result(), nil
		},
	}

	creds := GitHubAppCredentials{
		AppID:          "12345",
		PrivateKeyPath: keyPath,
		InstallationID: "67890",
	}

	_, err := GetInstallationToken(creds, mockClient)
	if err == nil {
		t.Fatal("expected error for API failure, got nil")
	}
	if !strings.Contains(err.Error(), "failed to get installation token") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestGetInstallationToken_EmptyToken(t *testing.T) {
	_, keyPath := writeTestKey(t)

	mockClient := &mockHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			recorder := httptest.NewRecorder()
			recorder.WriteHeader(http.StatusCreated)
			json.NewEncoder(recorder).Encode(map[string]string{"token": ""})
			return recorder.Result(), nil
		},
	}

	creds := GitHubAppCredentials{
		AppID:          "12345",
		PrivateKeyPath: keyPath,
		InstallationID: "67890",
	}

	_, err := GetInstallationToken(creds, mockClient)
	if err == nil {
		t.Fatal("expected error for empty token, got nil")
	}
	if !strings.Contains(err.Error(), "empty token") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestGetInstallationToken_BadKeyPath(t *testing.T) {
	mockClient := &mockHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			t.Error("HTTP client should not be called when key file is missing")
			return nil, nil
		},
	}

	creds := GitHubAppCredentials{
		AppID:          "12345",
		PrivateKeyPath: "/nonexistent/key.pem",
		InstallationID: "67890",
	}

	_, err := GetInstallationToken(creds, mockClient)
	if err == nil {
		t.Fatal("expected error for bad key path, got nil")
	}
}
