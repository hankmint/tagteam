// ABOUTME: tagteam CLI entry point
// ABOUTME: Lets coding agents from different vendors tag each other in on a shared bbs board

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

var version = "0.1.0"

const usage = `tagteam: tag-team your coding agents on a bbs board

Agents (Claude Code, Codex, Gemini CLI, anything in a tmux pane) hand work to
each other by @mentioning names in bbs posts. tagteam wakes whoever was tagged.

Usage:
  tagteam init      write an example config
  tagteam run       watch the board in the foreground
  tagteam start     watch the board in the background
  tagteam stop      stop the background watcher
  tagteam status    show whether it's running and what's in each agent's pane
  tagteam mcp       MCP server (stdio) with roster and tag tools
  tagteam version
`

func main() {
	if err := dispatch(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "tagteam:", err)
		os.Exit(1)
	}
}

func dispatch(args []string) error {
	if len(args) == 0 {
		fmt.Print(usage)
		return nil
	}
	switch args[0] {
	case "init":
		return initConfig()
	case "run":
		cfg, err := loadConfig()
		if err != nil {
			return err
		}
		return run(cfg)
	case "start":
		return start()
	case "stop":
		return stopDaemon()
	case "status":
		return status()
	case "mcp":
		cfg, err := loadConfig()
		if err != nil {
			return err
		}
		return serveMCP(context.Background(), cfg)
	case "version", "--version", "-v":
		fmt.Println("tagteam", version)
		return nil
	case "help", "--help", "-h":
		fmt.Print(usage)
		return nil
	}
	return fmt.Errorf("unknown command %q (try `tagteam help`)", args[0])
}

func initConfig() error {
	path := configPath()
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("config already exists at %s", path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(exampleConfig), 0o644); err != nil {
		return err
	}
	fmt.Printf("wrote %s\nedit the tmux targets, then run `tagteam status`\n", path)
	return nil
}
