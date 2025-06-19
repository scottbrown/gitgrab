package main

import (
	"fmt"
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

		client := gitgrab.NewGitHubClient(token)
		repos, err := client.FetchAllRepos(orgName)
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
			
			if err := gitgrab.CloneRepo(repo, targetDir, token, orgName, cloneMethod); err != nil {
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
	rootCmd.Flags().StringVarP(&cloneMethod, "method", "m", "ssh", "Clone method for private repositories: 'ssh' or 'http' (default: ssh)")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}