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
  - `Repository` struct: Represents GitHub repository metadata from API responses
  - `GitHubClient` struct: Handles GitHub API interactions with token-based authentication
  - `FetchAllRepos()`: Paginates through GitHub API to get all organization repositories
  - `CloneRepo()`: Performs git clone operations with proper authentication handling for both public and private repos
  - `rootCmd`: Cobra command that defines CLI interface with `-o/--org` flag and positional directory argument

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
GITHUB_TOKEN=<your_token> ./.build/gitgrab -o <organization> <target_directory>
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
- Handles both public and private repositories with token-based authentication
- Uses pagination to fetch all repositories (100 per page)
- Skips existing directories to avoid re-cloning
- Suppresses git clone output for cleaner console display
- Requires `GITHUB_TOKEN` environment variable and `git` binary in PATH
- Authentication URLs are dynamically constructed based on the specified organization