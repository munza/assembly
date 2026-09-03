package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/joho/godotenv"
)

type Project struct {
	Path        string `json:"path"`
	Repo        string `json:"repo"`
	IssuePrefix string `json:"issue_prefix,omitempty"`
}

type Linear struct {
	APIKey    string `json:"api_key"`
	Workspace string `json:"workspace"`
}

type Pi struct {
	Model    string `json:"model"`
	Thinking string `json:"thinking"`
}

type Settings struct {
	Linear   Linear              `json:"linear"`
	Pi       Pi                  `json:"pi"`
	Projects map[string]*Project `json:"projects"`
}

func Dir() string {
	if d := os.Getenv("FOREMAN_STATE_DIR"); d != "" {
		return d
	}
	return ".assembly"
}

func Path() string {
	return filepath.Join(Dir(), "settings.json")
}

func Load() (*Settings, error) {
	b, err := os.ReadFile(Path())
	if errors.Is(err, os.ErrNotExist) {
		return &Settings{Projects: map[string]*Project{}}, nil
	}
	if err != nil {
		return nil, err
	}
	st := &Settings{}
	if err := json.Unmarshal(b, st); err != nil {
		return nil, fmt.Errorf("parse %s: %w", Path(), err)
	}
	if st.Projects == nil {
		st.Projects = map[string]*Project{}
	}
	return st, nil
}

func Save(st *Settings) error {
	if err := os.MkdirAll(Dir(), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	tmp := Path() + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, Path())
}

var envPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// LoadDotEnv loads .assembly/.env into the process environment via godotenv.
// Existing environment variables win; a missing file is ignored.
func LoadDotEnv() {
	path := filepath.Join(Dir(), ".env")
	if _, err := os.Stat(path); err != nil {
		return
	}
	if err := godotenv.Load(path); err != nil {
		fmt.Fprintf(os.Stderr, "warning: %s: %v\n", path, err)
	}
}

// Expand resolves ${VAR} references from the environment (see LoadDotEnv).
func Expand(s string) string {
	return envPattern.ReplaceAllStringFunc(s, func(m string) string {
		return os.Getenv(envPattern.FindStringSubmatch(m)[1])
	})
}

func LinearAPIKey() string {
	st, err := Load()
	if err != nil || st == nil {
		return os.Getenv("LINEAR_API_KEY")
	}
	key := Expand(st.Linear.APIKey)
	if key == "" {
		key = os.Getenv("LINEAR_API_KEY")
	}
	return key
}
