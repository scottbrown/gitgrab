# gitgrab

A CLI utility to clone all GitHub repositories from an organization in one command.

## What it does

GitGrab fetches all repositories (both public and private) from a specified GitHub organization and clones them to a local directory. It handles authentication automatically and provides progress feedback during the cloning process.

## Usage

```bash
# Set your GitHub token
export GITHUB_TOKEN=your_github_token_here

# Clone all repositories from an organization (uses SSH for private repos by default)
gitgrab -o myorg ./repositories

# Use HTTP method for private repositories instead of SSH
gitgrab -o myorg -m http ./repositories

# Explicitly use SSH method for private repositories
gitgrab -o myorg -m ssh ./repositories
```

## Clone Methods

GitGrab supports two clone methods for private repositories:

- **SSH (default)**: Uses `git@github.com:org/repo.git` format
  - Requires SSH key setup with GitHub
  - More secure and doesn't expose tokens in process lists
  - Recommended for most use cases

- **HTTP**: Uses `https://token@github.com/org/repo.git` format
  - Uses your GitHub token for authentication
  - Fallback option if SSH keys aren't configured

Public repositories always use the standard HTTPS clone URL regardless of the method flag.

## Compile from source

```bash
# Build the application
task build

# The binary will be created at .build/gitgrab
```

## Requirements

- Go 1.24+
- Git installed and available in PATH
- GitHub personal access token with appropriate repository permissions

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
