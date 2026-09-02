// Package config resolves foreman configuration in layers:
// defaults → .assembly/config.json → environment variables (env wins).
package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// Config is the resolved foreman configuration.
type Config struct {
	RepoDir      string `json:"repo_dir"`      // project repo the agents work on
	ProjectName  string `json:"project_name"`  // herdr workspace label (1 per project)
	WorktreeDir  string `json:"worktree_dir"`  // where task worktrees live
	BranchPrefix string `json:"branch_prefix"` // git branch prefix for tasks
	MaxWorkers   int    `json:"max_workers"`   // parallel pi agents
	PollInterval time.Duration `json:"poll_interval_sec"` // watcher poll interval (seconds in JSON)
	LinearAPIKey string `json:"linear_api_key"` // Linear API key
	LinearTeamID string `json:"linear_team_id"` // filter to this team, optional
	GitHubRepo   string `json:"github_repo"`    // "owner/name"
	Model        string `json:"model"`          // pi model spec, e.g. "anthropic/claude-sonnet-4-5:high"
}

// StateDir returns the runtime state dir (.assembly inside the repo).
func StateDir() string { return ".assembly" }

// ConfigPath returns the path of the config file.
func ConfigPath() string { return filepath.Join(StateDir(), "config.json") }

// Default builds a config from sensible defaults and the given repo dir.
func Default(repoDir string) *Config {
	name := filepath.Base(repoDir)
	wtDir := filepath.Join(StateDir(), "worktrees")
	if !filepath.IsAbs(wtDir) {
		wtDir = filepath.Join(repoDir, wtDir)
	}
	return &Config{
		RepoDir:      repoDir,
		ProjectName:  name,
		WorktreeDir:  wtDir,
		BranchPrefix: "foreman/",
		MaxWorkers:   3,
		PollInterval: 60 * time.Second,
	}
}

// fileConfig is the on-disk JSON shape. PollInterval is stored in seconds.
type fileConfig struct {
	RepoDir      string `json:"repo_dir"`
	ProjectName  string `json:"project_name"`
	WorktreeDir  string `json:"worktree_dir"`
	BranchPrefix string `json:"branch_prefix"`
	MaxWorkers   int    `json:"max_workers"`
	PollSec      int    `json:"poll_interval_sec"`
	LinearAPIKey string `json:"linear_api_key"`
	LinearTeamID string `json:"linear_team_id"`
	GitHubRepo   string `json:"github_repo"`
	Model        string `json:"model"`
}

// Load resolves config: defaults (no file required) → file → env.
// Unlike before, Load works without a config file; `foreman init` just
// pre-writes one for convenience.
func Load() *Config {
	cwd, _ := os.Getwd()
	cfg := Default(cwd)

	// Layer 2: config file, if present.
	if b, err := os.ReadFile(ConfigPath()); err == nil {
		var f fileConfig
		if json.Unmarshal(b, &f) == nil {
			if f.RepoDir != "" {
				cfg.RepoDir = f.RepoDir
			}
			if f.ProjectName != "" {
				cfg.ProjectName = f.ProjectName
			}
			if f.WorktreeDir != "" {
				cfg.WorktreeDir = f.WorktreeDir
			}
			if f.BranchPrefix != "" {
				cfg.BranchPrefix = f.BranchPrefix
			}
			if f.MaxWorkers > 0 {
				cfg.MaxWorkers = f.MaxWorkers
			}
			if f.PollSec > 0 {
				cfg.PollInterval = time.Duration(f.PollSec) * time.Second
			}
			cfg.LinearAPIKey = f.LinearAPIKey
			cfg.LinearTeamID = f.LinearTeamID
			cfg.GitHubRepo = f.GitHubRepo
			cfg.Model = f.Model
		}
	}

	// Layer 3: environment. FOREMAN_* always wins; LINEAR_API_KEY is a
	// convenience fallback.
	env := func(name, fallback string) string {
		if v := os.Getenv(name); v != "" {
			return v
		}
		return fallback
	}
	cfg.RepoDir = env("FOREMAN_REPO_DIR", cfg.RepoDir)
	cfg.ProjectName = env("FOREMAN_PROJECT_NAME", cfg.ProjectName)
	cfg.WorktreeDir = env("FOREMAN_WORKTREE_DIR", cfg.WorktreeDir)
	cfg.BranchPrefix = env("FOREMAN_BRANCH_PREFIX", cfg.BranchPrefix)
	cfg.LinearAPIKey = env("FOREMAN_LINEAR_API_KEY", env("LINEAR_API_KEY", cfg.LinearAPIKey))
	cfg.LinearTeamID = env("FOREMAN_LINEAR_TEAM_ID", cfg.LinearTeamID)
	cfg.GitHubRepo = env("FOREMAN_GITHUB_REPO", cfg.GitHubRepo)
	cfg.Model = env("FOREMAN_MODEL", cfg.Model)
	if v := os.Getenv("FOREMAN_MAX_WORKERS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.MaxWorkers = n
		}
	}
	if v := os.Getenv("FOREMAN_POLL_INTERVAL_SEC"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.PollInterval = time.Duration(n) * time.Second
		}
	}
	return cfg
}

// Save writes the config to .assembly/config.json.
func (c *Config) Save() error {
	if err := os.MkdirAll(StateDir(), 0o755); err != nil {
		return err
	}
	f := fileConfig{
		RepoDir:      c.RepoDir,
		ProjectName:  c.ProjectName,
		WorktreeDir:  c.WorktreeDir,
		BranchPrefix: c.BranchPrefix,
		MaxWorkers:   c.MaxWorkers,
		PollSec:      int(c.PollInterval.Seconds()),
		LinearAPIKey: c.LinearAPIKey,
		LinearTeamID: c.LinearTeamID,
		GitHubRepo:   c.GitHubRepo,
		Model:        c.Model,
	}
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(ConfigPath(), b, 0o644)
}

// ErrNoConfig is kept for compatibility; config files are now optional.
var ErrNoConfig = errors.New("no config found; run: foreman init")
