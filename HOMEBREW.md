# Homebrew Setup for TCS

This document explains how to set up Homebrew distribution for TCS (Tmux Claude Scheduler).

## Automated Setup (via GoReleaser)

The Homebrew formula is automatically generated and published when creating a new release via GitHub Actions. The process:

1. **Create a release**: Tag and push a new version (e.g., `v1.0.0`)
2. **GitHub Actions triggers**: The release workflow runs GoReleaser  
3. **GoReleaser publishes**: Creates binaries and updates the Homebrew tap
4. **Users can install**: `brew install derekxwang/tcs/tcs`

## Manual Homebrew Tap Setup

If you need to set up the Homebrew tap manually:

### 1. Create Homebrew Tap Repository

```bash
# Create a new repository named 'homebrew-tcs'
gh repo create homebrew-tcs --public --description "Homebrew tap for TCS (Tmux Claude Scheduler)"
```

### 2. Clone and Setup

```bash
git clone https://github.com/derekxwang/homebrew-tcs.git
cd homebrew-tcs
mkdir -p Formula
```

### 3. GitHub Token Setup

For GoReleaser to publish to the Homebrew tap, you need to set up a GitHub token:

1. Create a Personal Access Token with `public_repo` scope
2. Add it as a repository secret named `HOMEBREW_TAP_GITHUB_TOKEN`

## Installation for Users

Once the tap is set up, users can install TCS via Homebrew:

```bash
# Add the tap (only needed once)
brew tap derekxwang/tcs

# Install TCS
brew install tcs

# Or install directly
brew install derekxwang/tcs/tcs
```

## Usage After Installation

```bash
# Verify installation
tcs --help

# Scan for Claude windows
tcs windows scan

# Schedule your first message
tcs message add project:0 "Hello Claude!" --when now

# Launch TUI
tcs tui
```

## Updating

Users can update to the latest version:

```bash
brew update
brew upgrade tcs
```

## Uninstalling

```bash
brew uninstall tcs
brew untap derekxwang/tcs  # Optional: remove the tap
```

## Formula Details

The Homebrew formula includes:

- **Binary**: `tcs` installed to `/usr/local/bin/` (Intel) or `/opt/homebrew/bin/` (Apple Silicon)
- **Config**: Example config at `/usr/local/etc/tcs/config.yaml.example`
- **Dependencies**: Runtime dependency on `tmux`
- **Post-install**: Helpful setup instructions

## Troubleshooting

### Formula Not Found
```bash
brew update
brew tap derekxwang/tcs
```

### Permission Issues
```bash
# Fix Homebrew permissions
sudo chown -R $(whoami) /usr/local/var/homebrew
```

### tmux Not Found
```bash
# Install tmux dependency
brew install tmux
```

## Development

To test formula changes locally:

```bash
# Install from local formula
brew install --build-from-source ./Formula/tcs.rb

# Test installation
brew test tcs

# Audit formula
brew audit --strict tcs
```