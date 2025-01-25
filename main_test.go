package main

import (
	"flag"
	"os"
	"os/exec"
	"testing"
)

func TestParseFlags(t *testing.T) {
	// Save original arguments and flags
	originalArgs := os.Args
	defer func() { os.Args = originalArgs }()

	tests := []struct {
		name     string
		args     []string
		expected *AppConfig
	}{
		{
			name: "default values",
			args: []string{"gh-fork-sync"},
			expected: &AppConfig{
				UpstreamBranch: "main",
				OriginBranch:   "main",
				Rebase:         false,
				ForcePush:      false,
				DryRun:         false,
			},
		},
		{
			name: "custom branches",
			args: []string{"gh-fork-sync", "--upstream-branch=develop", "--origin-branch=feature"},
			expected: &AppConfig{
				UpstreamBranch: "develop",
				OriginBranch:   "feature",
				Rebase:         false,
				ForcePush:      false,
				DryRun:         false,
			},
		},
		{
			name: "all flags enabled",
			args: []string{"gh-fork-sync", "--rebase", "--force", "--dry-run"},
			expected: &AppConfig{
				UpstreamBranch: "main",
				OriginBranch:   "main",
				Rebase:         true,
				ForcePush:      true,
				DryRun:         true,
			},
		},
	}

	// Create new flag set for each test
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset flags before each test
			flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)

			os.Args = tt.args
			config := parseFlags()

			if config.UpstreamBranch != tt.expected.UpstreamBranch {
				t.Errorf("UpstreamBranch = %v, want %v", config.UpstreamBranch, tt.expected.UpstreamBranch)
			}
			if config.OriginBranch != tt.expected.OriginBranch {
				t.Errorf("OriginBranch = %v, want %v", config.OriginBranch, tt.expected.OriginBranch)
			}
			if config.Rebase != tt.expected.Rebase {
				t.Errorf("Rebase = %v, want %v", config.Rebase, tt.expected.Rebase)
			}
			if config.ForcePush != tt.expected.ForcePush {
				t.Errorf("ForcePush = %v, want %v", config.ForcePush, tt.expected.ForcePush)
			}
			if config.DryRun != tt.expected.DryRun {
				t.Errorf("DryRun = %v, want %v", config.DryRun, tt.expected.DryRun)
			}
		})
	}
}

func TestGetOriginRepo(t *testing.T) {
	// Create temporary directory for test git repository
	tmpDir, err := os.MkdirTemp("", "test-repo-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Change to temporary directory
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(originalDir)

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	// Initialize git repository
	if err := exec.Command("git", "init").Run(); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name          string
		remoteURL     string
		expectedOwner string
		expectedRepo  string
		wantErr       bool
	}{
		{
			name:          "HTTPS URL",
			remoteURL:     "https://github.com/owner/repo.git",
			expectedOwner: "owner",
			expectedRepo:  "repo",
			wantErr:       false,
		},
		{
			name:          "SSH URL",
			remoteURL:     "git@github.com:owner/repo.git",
			expectedOwner: "owner",
			expectedRepo:  "repo",
			wantErr:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set remote URL
			cmd := exec.Command("git", "remote", "remove", "origin")
			cmd.Run() // Ignore error if origin doesn't exist

			cmd = exec.Command("git", "remote", "add", "origin", tt.remoteURL)
			if err := cmd.Run(); err != nil {
				t.Fatal(err)
			}

			owner, repo, err := GetOriginRepo()
			if (err != nil) != tt.wantErr {
				t.Errorf("GetOriginRepo() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if owner != tt.expectedOwner {
				t.Errorf("GetOriginRepo() owner = %v, want %v", owner, tt.expectedOwner)
			}
			if repo != tt.expectedRepo {
				t.Errorf("GetOriginRepo() repo = %v, want %v", repo, tt.expectedRepo)
			}
		})
	}
}

func TestValidateFork(t *testing.T) {
	tests := []struct {
		name    string
		info    *RepoInfo
		wantErr bool
	}{
		{
			name: "valid fork",
			info: &RepoInfo{
				FullName: "user/repo",
				Fork:     true,
			},
			wantErr: false,
		},
		{
			name: "not a fork",
			info: &RepoInfo{
				FullName: "user/repo",
				Fork:     false,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFork(tt.info)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateFork() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGetSyncCommand(t *testing.T) {
	tests := []struct {
		name           string
		config         *AppConfig
		upstreamBranch string
		expectedArgs   []string
	}{
		{
			name: "merge without branch",
			config: &AppConfig{
				Rebase: false,
			},
			upstreamBranch: "",
			expectedArgs:   []string{"merge", "upstream"},
		},
		{
			name: "merge with branch",
			config: &AppConfig{
				Rebase: false,
			},
			upstreamBranch: "main",
			expectedArgs:   []string{"merge", "upstream", "upstream/main"},
		},
		{
			name: "rebase without branch",
			config: &AppConfig{
				Rebase: true,
			},
			upstreamBranch: "",
			expectedArgs:   []string{"rebase", "upstream"},
		},
		{
			name: "rebase with branch",
			config: &AppConfig{
				Rebase: true,
			},
			upstreamBranch: "main",
			expectedArgs:   []string{"rebase", "upstream", "upstream/main"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := getSyncCommand(tt.config, tt.upstreamBranch)

			if len(cmd.Args) != len(tt.expectedArgs) {
				t.Errorf("getSyncCommand() args length = %v, want %v", len(cmd.Args), len(tt.expectedArgs))
				return
			}

			for i, arg := range cmd.Args {
				if arg != tt.expectedArgs[i] {
					t.Errorf("getSyncCommand() arg[%d] = %v, want %v", i, arg, tt.expectedArgs[i])
				}
			}
		})
	}
}
