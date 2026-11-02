// ABOUTME: Loads and validates tagteam's YAML config
// ABOUTME: Resolves default paths for the config, the bbs database, and tagteam's state

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Agent is one interactive coding agent living in a tmux pane.
type Agent struct {
	Name    string   `yaml:"name" json:"name"`
	Target  string   `yaml:"target" json:"target"`
	Aliases []string `yaml:"aliases" json:"aliases,omitempty"`
}

// Config is the whole tagteam setup.
type Config struct {
	DB         string        `yaml:"db"`
	Bbs        string        `yaml:"bbs"`
	Poll       time.Duration `yaml:"poll"`
	Cooldown   time.Duration `yaml:"cooldown"`
	MaxPerHour int           `yaml:"max_wakes_per_hour"`
	Agents     []Agent       `yaml:"agents"`
}

var namePattern = regexp.MustCompile(`^[a-z0-9_-]+$`)

const exampleConfig = `# tagteam config. Agents wake each other by @mentioning names in bbs posts.

# Path to the bbs database. Defaults to bbs's own default location.
# db: ~/.local/share/bbs/bbs.db

poll: 5s                  # how often to check the board
cooldown: 30s             # minimum gap between two wakes of the same agent
max_wakes_per_hour: 20    # per agent; stops runaway loops

agents:
  - name: claude
    target: workspace:claude   # tmux target (session:window or session:window.pane)
    aliases: [dembe]           # other names this agent posts under
  - name: codex
    target: workspace:codex
`

func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, strings.TrimPrefix(p, "~"))
	}
	return p
}

func xdg(env, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(env)); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, fallback)
}

func configPath() string {
	if v := strings.TrimSpace(os.Getenv("TAGTEAM_CONFIG")); v != "" {
		return expandHome(v)
	}
	return filepath.Join(xdg("XDG_CONFIG_HOME", ".config"), "tagteam", "config.yaml")
}

func stateDir() string {
	return filepath.Join(xdg("XDG_STATE_HOME", ".local/state"), "tagteam")
}

func loadConfig() (*Config, error) {
	path := configPath()
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no config at %s (run `tagteam init`)", path)
		}
		return nil, err
	}
	cfg := &Config{}
	if err := yaml.Unmarshal(raw, cfg); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if err := cfg.normalize(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, nil
}

func (c *Config) normalize() error {
	if c.DB == "" {
		c.DB = filepath.Join(xdg("XDG_DATA_HOME", ".local/share"), "bbs", "bbs.db")
	}
	c.DB = expandHome(strings.TrimSpace(c.DB))
	if c.Bbs == "" {
		c.Bbs = "bbs"
	}
	if c.Poll <= 0 {
		c.Poll = 5 * time.Second
	}
	if c.Cooldown < 0 {
		c.Cooldown = 0
	}
	if c.MaxPerHour <= 0 {
		c.MaxPerHour = 20
	}
	if len(c.Agents) == 0 {
		return fmt.Errorf("no agents configured")
	}
	seen := map[string]bool{}
	for i := range c.Agents {
		a := &c.Agents[i]
		a.Name = strings.ToLower(strings.TrimSpace(a.Name))
		a.Target = strings.TrimSpace(a.Target)
		if !namePattern.MatchString(a.Name) || a.Name == "all" {
			return fmt.Errorf("agent name %q must be lowercase letters, digits, - or _, and not \"all\"", a.Name)
		}
		if a.Target == "" {
			return fmt.Errorf("agent %q has no tmux target", a.Name)
		}
		for j, alias := range a.Aliases {
			a.Aliases[j] = strings.ToLower(strings.TrimSpace(alias))
			if !namePattern.MatchString(a.Aliases[j]) || a.Aliases[j] == "all" {
				return fmt.Errorf("alias %q of %q must be one word: lowercase letters, digits, - or _", alias, a.Name)
			}
		}
		for _, n := range append([]string{a.Name}, a.Aliases...) {
			if seen[n] {
				return fmt.Errorf("name %q is used twice", n)
			}
			seen[n] = true
		}
	}
	return nil
}

func (c *Config) agent(name string) *Agent {
	for i := range c.Agents {
		if c.Agents[i].Name == name {
			return &c.Agents[i]
		}
	}
	return nil
}
