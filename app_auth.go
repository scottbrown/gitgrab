package gitgrab

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// GitHubAppCredentials holds the credentials needed to authenticate as a GitHub App.
type GitHubAppCredentials struct {
	AppID          string
	PrivateKeyPath string
	InstallationID string
}

// loadPrivateKey reads and parses an RSA private key from a PEM-encoded file.
// Both PKCS#1 and PKCS#8 formats are supported.
func loadPrivateKey(path string) (*rsa.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read private key file %q: %w", path, err)
	}

	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found in %q", path)
	}

	// Try PKCS#1 (traditional RSA private key) first.
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}

	// Fall back to PKCS#8 (used by some key generation tools).
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key from %q: %w", path, err)
	}

	rsaKey, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("private key in %q is not an RSA key", path)
	}

	return rsaKey, nil
}

// buildJWT creates a signed RS256 JWT suitable for GitHub App authentication.
// The token is valid for 10 minutes with a 60-second back-dated iat to tolerate
// minor clock skew between the client and GitHub's servers.
func buildJWT(appID string, privateKey *rsa.PrivateKey) (string, error) {
	now := time.Now()

	headerJSON, err := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT"})
	if err != nil {
		return "", fmt.Errorf("failed to marshal JWT header: %w", err)
	}

	payloadJSON, err := json.Marshal(map[string]interface{}{
		"iat": now.Add(-60 * time.Second).Unix(),
		"exp": now.Add(10 * time.Minute).Unix(),
		"iss": appID,
	})
	if err != nil {
		return "", fmt.Errorf("failed to marshal JWT payload: %w", err)
	}

	header := base64.RawURLEncoding.EncodeToString(headerJSON)
	payload := base64.RawURLEncoding.EncodeToString(payloadJSON)
	signingInput := header + "." + payload

	h := sha256.New()
	h.Write([]byte(signingInput))
	digest := h.Sum(nil)

	sig, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, digest)
	if err != nil {
		return "", fmt.Errorf("failed to sign JWT: %w", err)
	}

	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

// GetInstallationToken exchanges GitHub App credentials for a short-lived
// installation access token. The returned GitHubToken can be used directly
// in place of a PAT — it uses the same Authorization header format.
func GetInstallationToken(creds GitHubAppCredentials, client HTTPClient) (GitHubToken, error) {
	privateKey, err := loadPrivateKey(creds.PrivateKeyPath)
	if err != nil {
		return "", err
	}

	jwt, err := buildJWT(creds.AppID, privateKey)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("https://api.github.com/app/installations/%s/access_tokens", creds.InstallationID)
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create installation token request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+jwt)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "GitHub-Repo-Cloner")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to request installation token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to get installation token: %s - %s", resp.Status, string(body))
	}

	var result struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode installation token response: %w", err)
	}

	if result.Token == "" {
		return "", fmt.Errorf("received empty token from GitHub API")
	}

	return GitHubToken(result.Token), nil
}
