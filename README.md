# gitgrab

A CLI utility to clone all GitHub repositories from an organization in one command.

## What it does

GitGrab fetches all repositories (both public and private) from a specified GitHub organization and clones them to a local directory. It handles authentication automatically and provides progress feedback during the cloning process.

## Usage

```bash
# Set your GitHub token
export GITHUB_TOKEN=your_github_token_here

# Clone all repositories from an organization
gitgrab -o myorg ./repositories
```

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
