package main

import (
	"flag"
	"fmt"
	"os/exec"
	"strings"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/cli/safeexec"
)

// AppConfig holds the configuration for the fork-sync command
type AppConfig struct {
	UpstreamBranch string // Branch to sync from upstream (default: current branch)
	OriginBranch   string // Local branch to update (default: current branch)
	Rebase         bool   // Rebase instead of merge
	ForcePush      bool   // Force push to origin
	DryRun         bool   // Print commands without executing them
}

// RepoInfo holds information about a GitHub repository
type RepoInfo struct {
	FullName string `json:"full_name"`
	Fork     bool   `json:"fork"`
	Parent   struct {
		FullName string `json:"full_name"`
		CloneURL string `json:"clone_url"`
	} `json:"parent"`
}

// GitCommand represents a git command to be executed
type GitCommand struct {
	Args        []string
	Description string
}

// parseFlags parses command line flags and returns the configuration
func parseFlags() *AppConfig {
	config := &AppConfig{}

	flag.StringVar(&config.UpstreamBranch, "upstream-branch", "main", "Branch to sync from upstream (default: main)")
	flag.StringVar(&config.OriginBranch, "origin-branch", "main", "Local branch to update (default: main)")
	flag.BoolVar(&config.Rebase, "rebase", false, "Rebase instead of merge")
	flag.BoolVar(&config.ForcePush, "force", false, "Force push to origin")
	flag.BoolVar(&config.DryRun, "dry-run", false, "Print commands without executing them")

	// Add custom usage message
	flag.Usage = func() {
		fmt.Println("gh fork-sync - GitHub CLI extension to sync your fork with the upstream repository")
		fmt.Println("\nUsage:")
		fmt.Println("  gh fork-sync [flags]")
		fmt.Println("\nFlags:")
		flag.PrintDefaults()
		fmt.Println("\nExamples:")
		fmt.Println("  # Sync main branch with upstream")
		fmt.Println("  $ gh fork-sync")
		fmt.Println("\n  # Sync a specific branch")
		fmt.Println("  $ gh fork-sync --upstream-branch develop --origin-branch develop")
		fmt.Println("\n  # Rebase instead of merge")
		fmt.Println("  $ gh fork-sync --rebase")
		fmt.Println("\n  # Force push the changes")
		fmt.Println("  $ gh fork-sync --force")
		fmt.Println("\n  # Preview the commands without executing them")
		fmt.Println("  $ gh fork-sync --dry-run")
	}

	flag.Parse()
	return config
}

// GetOriginRepo returns the owner and repo name of the "origin" remote.
func GetOriginRepo() (owner, repo string, err error) {
	// Get the origin remote URL
	cmd := exec.Command("git", "remote", "get-url", "origin")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", "", fmt.Errorf("failed to get origin remote: %v", err)
	}

	url := strings.TrimSpace(string(output))

	// Parse SSH or HTTPS URL
	var parts []string
	if strings.HasPrefix(url, "git@github.com:") {
		// SSH format: git@github.com:owner/repo.git
		path := strings.TrimPrefix(url, "git@github.com:")
		parts = strings.SplitN(path, "/", 2)
	} else if strings.Contains(url, "github.com/") {
		// HTTPS format: https://github.com/owner/repo.git
		path := strings.SplitN(url, "github.com/", 2)[1]
		parts = strings.SplitN(path, "/", 2)
	} else {
		return "", "", fmt.Errorf("unsupported origin URL format: %s", url)
	}

	if len(parts) < 2 {
		return "", "", fmt.Errorf("failed to parse owner/repo from URL: %s", url)
	}

	owner = parts[0]
	repo = strings.TrimSuffix(parts[1], ".git") // Remove .git suffix if present
	return owner, repo, nil
}

// getRepoInfo fetches repository information from GitHub API
func getRepoInfo(client *api.RESTClient, owner, repoName string) (*RepoInfo, error) {
	info := &RepoInfo{}
	err := client.Get(fmt.Sprintf("repos/%s/%s", owner, repoName), info)
	if err != nil {
		return nil, fmt.Errorf("failed to get repo info: %v", err)
	}
	return info, nil
}

// validateFork checks if the repository is a fork
func validateFork(info *RepoInfo) error {
	if !info.Fork {
		return fmt.Errorf("repository %s isn't a fork", info.FullName)
	}
	return nil
}

// runGitCommand executes a git command and returns any error
func runGitCommand(gitBin string, cmd GitCommand) error {
	output, err := exec.Command(gitBin, cmd.Args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %v\nOutput: %s", cmd.Description, err, output)
	}
	return nil
}

// getSyncCommand returns the appropriate sync command (merge or rebase)
func getSyncCommand(config *AppConfig, upstreamBranch string) GitCommand {
	if config.Rebase {
		args := []string{"rebase", "upstream"}
		if upstreamBranch != "" {
			args = append(args, fmt.Sprintf("upstream/%s", upstreamBranch))
		}
		return GitCommand{
			Args:        args,
			Description: "rebasing onto upstream",
		}
	}
	args := []string{"merge", "upstream"}
	if upstreamBranch != "" {
		args = append(args, fmt.Sprintf("upstream/%s", upstreamBranch))
	}
	return GitCommand{
		Args:        args,
		Description: "merging upstream",
	}
}

// printDryRun prints commands that would be executed
func printDryRun(config *AppConfig, parentCloneURL string) {
	fmt.Println("Note: The following commands are examples. The actual upstream URL will be taken from your fork's parent repository.")
	fmt.Println("Dry run mode - commands that would be executed:")
	fmt.Printf("Would run: git remote add upstream %s\n", parentCloneURL)
	fmt.Printf("Would run: git fetch upstream\n")

	if config.Rebase {
		fmt.Printf("Would run: git rebase upstream/%s\n", config.UpstreamBranch)
	} else {
		fmt.Printf("Would run: git merge upstream/%s\n", config.UpstreamBranch)
	}

	pushCmd := "push"
	if config.ForcePush {
		pushCmd += " -f"
	}
	fmt.Printf("Would run: git %s origin HEAD:%s\n", pushCmd, config.OriginBranch)
}

func main() {
	config := parseFlags()

	// Handle dry run first, before any API calls or git operations
	if config.DryRun {
		// Use placeholder URL for dry run
		printDryRun(config, "https://github.com/upstream/repo.git")
		return
	}

	// Initialize the GitHub API client
	client, err := api.DefaultRESTClient()
	if err != nil {
		fmt.Printf("✗ Error: %v\n", err)
		return
	}

	// Get repository information
	owner, repoName, err := GetOriginRepo()
	if err != nil {
		fmt.Printf("✗ Error: %v\n", err)
		return
	}

	// Get and validate repository information
	repoInfo, err := getRepoInfo(client, owner, repoName)
	if err != nil {
		fmt.Printf("✗ Error: %v\n", err)
		return
	}

	if err := validateFork(repoInfo); err != nil {
		fmt.Printf("✗ %v\n", err)
		return
	}

	fmt.Printf("✓ Detected fork: %s (parent: %s)\n", repoInfo.FullName, repoInfo.Parent.FullName)

	// Find git executable
	gitBin, err := safeexec.LookPath("git")
	if err != nil {
		fmt.Printf("✗ Error while looking for git: %v\n", err)
		return
	}

	// Add upstream remote
	cmd := GitCommand{
		Args:        []string{"remote", "add", "upstream", repoInfo.Parent.CloneURL},
		Description: "adding upstream remote",
	}
	if err := runGitCommand(gitBin, cmd); err != nil {
		if !strings.Contains(err.Error(), "remote upstream already exists") {
			fmt.Printf("✗ Error while %v\n", err)
			return
		}
	}

	// Fetch upstream
	cmd = GitCommand{
		Args:        []string{"fetch", "upstream"},
		Description: "fetching upstream",
	}
	if err := runGitCommand(gitBin, cmd); err != nil {
		fmt.Printf("✗ Error while %v\n", err)
		return
	}
	fmt.Println("✓ Fetched upstream")

	// Sync with upstream
	cmd = getSyncCommand(config, config.UpstreamBranch)
	if err := runGitCommand(gitBin, cmd); err != nil {
		fmt.Printf("✗ Error while %v\n", err)
		if config.Rebase {
			fmt.Println("To abort the rebase, run: git rebase --abort")
		} else {
			fmt.Println("To abort the merge, run: git merge --abort")
		}
		return
	}
	if config.Rebase {
		fmt.Printf("✓ Rebased onto upstream/%s\n", config.UpstreamBranch)
	} else {
		fmt.Printf("✓ Merged upstream/%s\n", config.UpstreamBranch)
	}

	// Push changes
	pushArgs := []string{"push"}
	if config.ForcePush {
		pushArgs = append(pushArgs, "-f")
	}
	pushArgs = append(pushArgs, "origin", fmt.Sprintf("HEAD:%s", config.OriginBranch))

	cmd = GitCommand{
		Args:        pushArgs,
		Description: fmt.Sprintf("pushing to origin/%s", config.OriginBranch),
	}
	if err := runGitCommand(gitBin, cmd); err != nil {
		fmt.Printf("✗ Error while %v\n", err)
		return
	}
	fmt.Printf("✓ Pushed to origin/%s\n", config.OriginBranch)
}

// For more examples of using go-gh, see:
// https://github.com/cli/go-gh/blob/trunk/example_gh_test.go
