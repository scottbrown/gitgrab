# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is `gitgrab`, a CLI utility written in Go that clones all GitHub repositories from an organization. The tool uses the GitHub API to fetch repository metadata and git commands to clone repositories locally.

## Architecture

- **Single binary**: The entire application is contained in `main.go`
- **Core components**:
  - `Repository` struct: Represents GitHub repository metadata
  - `GitHubClient` struct: Handles GitHub API interactions with authentication
  - `fetchAllRepos()`: Paginates through GitHub API to get all org repositories
  - `cloneRepo()`: Performs git clone operations with authentication handling

## Development Commands

**Build the application:**
```bash
go build -o gitgrab main.go
```

**Run the application:**
```bash
GITHUB_TOKEN=<your_token> ./gitgrab <target_directory>
```

**Run tests (if any exist):**
```bash
go test ./...
```

**Format code:**
```bash
go fmt ./...
```

**Check for issues:**
```bash
go vet ./...
```

## Key Implementation Details

- The application is hardcoded to clone from "kohofinancial" organization
- Handles both public and private repositories with token-based authentication
- Uses pagination to fetch all repositories (100 per page)
- Skips existing directories to avoid re-cloning
- Suppresses git clone output for cleaner console display
- Requires `GITHUB_TOKEN` environment variable and `git` binary in PATH