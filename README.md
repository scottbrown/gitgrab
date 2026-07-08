# gitgrab

A CLI utility to clone all GitHub repositories from an organization in one command.

## What it does

GitGrab fetches all repositories (both public and private) from a specified GitHub organization and clones them to a local directory. For repositories that already exist locally, it automatically updates them:

- **New repositories**: Clones the repository using the specified method (SSH or HTTP)
- **Existing repositories**: 
  - If on the default branch (main, master, etc.): Performs `git pull` to get latest changes
  - If on any other branch: Performs `git fetch` to update remote tracking branches
  - Fallback: If branch detection fails, performs `git fetch`

It handles authentication automatically and provides progress feedback during the cloning and updating process.

## Usage

```bash
# Set your GitHub token
export GITHUB_TOKEN=your_github_token_here

# Clone all repositories from an organization (uses SSH by default)
gitgrab -o myorg ./repositories

# Use HTTP method for all repositories instead of SSH
gitgrab -o myorg -m http ./repositories

# Explicitly use SSH method for all repositories
gitgrab -o myorg -m ssh ./repositories
```

## Clone Methods

GitGrab supports two clone methods for all repositories:

- **SSH (default)**: Uses `git@github.com:org/repo.git` format
  - Requires SSH key setup with GitHub
  - More secure and doesn't expose tokens in process lists
  - Recommended for most use cases
  - Works for both public and private repositories

- **HTTP**: Uses different URLs based on repository privacy:
  - **Private repositories**: `https://token@github.com/org/repo.git` (uses your GitHub token)
  - **Public repositories**: `https://github.com/org/repo.git` (standard HTTPS clone URL)
  - Fallback option if SSH keys aren't configured

## Compile from source

```bash
# Build the application
task build

# The binary will be created at .build/gitgrab
```

## Security fuzz testing

GitGrab is fuzz-tested at its untrusted-input boundaries (the clone-method
flag, GitHub-supplied repository/organization names and URLs, path resolution,
clone-URL construction, and API JSON decoding) to guard against path traversal
and command/argument injection. The fuzz targets live in `fuzz_test.go` and
come in two tiers:

```bash
# Lightweight, time-bounded burst — runs in CI on every push/PR
task fuzz-ci

# Heavyweight ad hoc run on a developer machine (defaults to 5m per target)
task fuzz

# Override the per-target duration
task fuzz FUZZTIME=30m
```

The seed corpora also execute as ordinary unit tests during `task test`, so the
malicious inputs are checked on every test run even without a fuzzing pass.

## Requirements

- Go 1.24+
- Git installed and available in PATH
- GitHub personal access token with appropriate repository permissions

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
