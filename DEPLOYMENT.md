# TCS Deployment Guide

This guide explains how to deploy TCS (Tmux Claude Scheduler) for distribution via Homebrew and other channels.

## Quick Setup for Homebrew Distribution

### 1. Prerequisites

- Create a GitHub repository named `homebrew-tcs` for your Homebrew tap
- Set up a GitHub Personal Access Token with `public_repo` permissions
- Add the token as a repository secret named `HOMEBREW_TAP_GITHUB_TOKEN`

### 2. Create a Release

```bash
# Tag a new version
git tag v1.0.0
git push origin v1.0.0
```

This will trigger the GitHub Actions workflow that:
- Runs tests
- Builds binaries for multiple platforms
- Creates a GitHub release
- Updates the Homebrew tap automatically

### 3. Users Can Install

Once released, users can install via Homebrew:

```bash
# Add tap and install
brew tap derekxwang/tcs
brew install tcs

# Or install directly
brew install derekxwang/tcs/tcs
```

## Manual Testing

To test the GoReleaser configuration locally:

```bash
# Install goreleaser
go install github.com/goreleaser/goreleaser@latest

# Test the configuration (dry run)
goreleaser check

# Build snapshot without releasing
goreleaser build --snapshot --clean
```

## Release Process

### Automated (Recommended)

1. **Commit changes**: Ensure all changes are committed and pushed
2. **Create tag**: `git tag v1.0.0 && git push origin v1.0.0`
3. **GitHub Actions**: Automatically builds and releases
4. **Homebrew**: Formula is automatically updated

### Manual Release

If you need to release manually:

```bash
# Set required environment variables
export GITHUB_TOKEN="your_github_token"
export HOMEBREW_TAP_GITHUB_TOKEN="your_homebrew_token"

# Create release
goreleaser release --clean
```

## Distribution Channels

### 1. Homebrew (Primary)
- **Command**: `brew install derekxwang/tcs/tcs`
- **Platforms**: macOS, Linux
- **Updates**: Automatic via `brew upgrade`

### 2. Direct Download
- **Source**: GitHub Releases
- **Platforms**: Linux (amd64, arm64), macOS (amd64, arm64), Windows (amd64, arm64)
- **Formats**: tar.gz (Unix), zip (Windows)

### 3. Go Install
```bash
go install github.com/derekxwang/tcs@latest
```

## Release Checklist

- [ ] All tests pass locally: `make test`
- [ ] Build works on multiple platforms: `make build`
- [ ] Version number follows semantic versioning
- [ ] CHANGELOG.md is updated
- [ ] GitHub token has correct permissions
- [ ] Homebrew tap repository exists and is accessible

## Homebrew Formula Details

The generated formula includes:

```ruby
class Tcs < Formula
  desc "TCS (Tmux Claude Scheduler) - Maximize Claude subscription usage with smart scheduling"
  homepage "https://github.com/derekxwang/tcs"
  license "MIT"
  
  depends_on "tmux"
  
  def install
    bin.install "tcs"
    etc.install "config.yaml" => "tcs/config.yaml.example"
    (etc/"tcs").mkpath
  end
  
  def caveats
    <<~EOS
      TCS (Tmux Claude Scheduler) has been installed!
      
      To get started:
      1. Ensure tmux is running: tmux new-session -d
      2. Scan for Claude windows: tcs windows scan
      3. Schedule your first message: tcs message add project:0 "Hello Claude!" --when now
      4. Launch the TUI: tcs tui
      
      Configuration file: ~/.config/tcs/config.yaml
      Example config: #{etc}/tcs/config.yaml.example
    EOS
  end
end
```

## Troubleshooting

### Release Fails
- Check GitHub token permissions
- Verify repository access
- Ensure tag follows semantic versioning (v1.0.0)

### Homebrew Formula Issues
- Verify homebrew-tcs repository exists
- Check HOMEBREW_TAP_GITHUB_TOKEN permissions
- Ensure formula syntax is correct

### Build Issues
- Run `make test` to verify all tests pass
- Check Go version compatibility (1.22+)
- Verify all dependencies are available

## Support

For issues with:
- **Installation**: Check [HOMEBREW.md](HOMEBREW.md)
- **Usage**: See [README.md](README.md)
- **Development**: Refer to the main documentation
- **Bugs**: Open an issue on GitHub