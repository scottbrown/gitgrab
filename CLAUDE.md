# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is `gitgrab`, a CLI utility written in Go that clones all GitHub repositories from an organization. The tool uses the GitHub API to fetch repository metadata and git commands to clone repositories locally.

## Architecture

- **Library package**: Core business logic is in the root `gitgrab` package, making it importable as a library
- **CLI entry point**: Located in `cmd/gitgrab/main.go` following standard Go project layout conventions
- **CLI framework**: Uses spf13/cobra for command-line argument parsing and flag handling
- **Dependency injection**: Uses `HTTPClient` interface to enable testing without external API calls
- **Core components**:
  - `Repository` struct: Represents GitHub repository metadata from API responses (includes `CloneURL`, `SSHURL`, and `Private` fields)
  - `GitHubClient` struct: Handles GitHub API interactions with token-based authentication
  - `FetchAllRepos()`: Paginates through GitHub API to get all organization repositories
  - `CloneRepo()`: Performs git clone operations with configurable clone method (SSH/HTTP) for private repos
  - `rootCmd`: Cobra command that defines CLI interface with `-o/--org` and `-m/--method` flags and positional directory argument

## Testing Strategy

- Unit tests use idiomatic Go interfaces for mocking (no third-party mocking libraries)
- `HTTPClient` interface enables testing GitHub API interactions without external calls
- Tests use `httptest.NewRecorder()` for mock HTTP responses
- Current test coverage: ~70%

## Development Commands

**Build the application (preferred method):**
```bash
task build
```

**Alternative build:**
```bash
go build -o .build/gitgrab ./cmd/gitgrab
```

**Run the application:**
```bash
# Default (SSH for private repos)
GITHUB_TOKEN=<your_token> ./.build/gitgrab -o <organization> <target_directory>

# Force HTTP method for private repos
GITHUB_TOKEN=<your_token> ./.build/gitgrab -o <organization> -m http <target_directory>
```

**Format code:**
```bash
task fmt
```

**Run tests:**
```bash
task test
```

**Run test coverage:**
```bash
task coverage
```

**Run specific test:**
```bash
go test . -run TestName
```

**Clean build artifacts:**
```bash
task clean
```

## Key Implementation Details

- Organization name is required via `-o/--org` flag
- Clone method can be specified via `-m/--method` flag (`ssh` or `http`, defaults to `ssh`)
- Handles both public and private repositories with configurable authentication:
  - **SSH method (default)**: Uses `ssh_url` from GitHub API (`git@github.com:org/repo.git`) for all repositories
  - **HTTP method**: 
    - Private repos: Uses token-based authentication (`https://token@github.com/org/repo.git`)
    - Public repos: Uses `clone_url` from GitHub API (`https://github.com/org/repo.git`)
- Uses pagination to fetch all repositories (100 per page)
- Skips existing directories to avoid re-cloning
- Suppresses git clone output for cleaner console display
- Requires `GITHUB_TOKEN` environment variable and `git` binary in PATH
- For SSH method, requires SSH key setup with GitHub for repository access