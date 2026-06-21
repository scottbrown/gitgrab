package main

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/scottbrown/gitgrab"
	"github.com/spf13/cobra"
)

var (
	orgName     string
	cloneMethod string
)

var rootCmd = &cobra.Command{
	Use:   "gitgrab [target_directory]",
	Short: "Clone all repositories from a GitHub organization",
	Long:  "GitGrab is a CLI utility that clones all GitHub repositories from a specified organization to a local directory.",
	Args:  cobra.ExactArgs(1),
  Version: gitgrab.Version(),
	Run: func(cmd *cobra.Command, args []string) {
		targetDir := args[0]
		githubToken, err := resolveToken(&http.Client{})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
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

		// Parse clone method
		method, err := gitgrab.ParseCloneMethod(cloneMethod)
		if err != nil {
			fmt.Printf("Warning: %v\n", err)
		}

		// Create typed values
		organization := gitgrab.OrganizationName(orgName)

		client := gitgrab.NewGitHubClient(githubToken)
		repos, err := client.FetchAllRepos(organization)
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
			
			config := gitgrab.CloneConfig{
				Repository:   repo,
				TargetDir:    targetDir,
				Token:        githubToken,
				Organization: organization,
				Method:       method,
			}
			
			if err := gitgrab.CloneRepo(config); err != nil {
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

// resolveToken determines which authentication method to use and returns a
// GitHubToken. GitHub App credentials (GITHUB_APP_ID, GITHUB_APP_PRIVATE_KEY,
// GITHUB_APP_INSTALLATION_ID) take precedence over a PAT (GITHUB_TOKEN).
// If any App variable is set, all three must be present.
func resolveToken(httpClient gitgrab.HTTPClient) (gitgrab.GitHubToken, error) {
	appID := os.Getenv("GITHUB_APP_ID")
	keyPath := os.Getenv("GITHUB_APP_PRIVATE_KEY")
	installID := os.Getenv("GITHUB_APP_INSTALLATION_ID")

	appVarsSet := appID != "" || keyPath != "" || installID != ""
	if appVarsSet {
		if appID == "" || keyPath == "" || installID == "" {
			return "", errors.New(
				"incomplete GitHub App credentials: GITHUB_APP_ID, GITHUB_APP_PRIVATE_KEY, and GITHUB_APP_INSTALLATION_ID must all be set",
			)
		}
		creds := gitgrab.GitHubAppCredentials{
			AppID:          appID,
			PrivateKeyPath: keyPath,
			InstallationID: installID,
		}
		return gitgrab.GetInstallationToken(creds, httpClient)
	}

	pat := os.Getenv("GITHUB_TOKEN")
	if pat != "" {
		return gitgrab.GitHubToken(pat), nil
	}

	return "", errors.New(
		"no GitHub credentials configured: set GITHUB_TOKEN or all three GITHUB_APP_* variables (GITHUB_APP_ID, GITHUB_APP_PRIVATE_KEY, GITHUB_APP_INSTALLATION_ID)",
	)
}

func init() {
	rootCmd.Flags().StringVarP(&orgName, "org", "o", "", "GitHub organization name")
	rootCmd.MarkFlagRequired("org")
	rootCmd.Flags().StringVarP(&cloneMethod, "method", "m", "ssh", "Clone method for repositories: 'ssh' or 'http' (default: ssh)")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
