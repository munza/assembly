package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

type Project struct {
	Path string `json:"path"`
	Repo string `json:"repo"`
}

type Linear struct {
	APIKey    string `json:"api_key"`
	Workspace string `json:"workspace"`
}

type Settings struct {
	Linear   Linear              `json:"linear"`
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

func Expand(s string) string {
	return envPattern.ReplaceAllStringFunc(s, func(m string) string {
		return os.Getenv(envPattern.FindStringSubmatch(m)[1])
	})
}
