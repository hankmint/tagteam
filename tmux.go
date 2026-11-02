// ABOUTME: Delivers wake-ups by typing into an agent's tmux pane
// ABOUTME: Refuses to type into a bare shell or a pane that doesn't exist

package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

var shells = map[string]bool{
	"sh": true, "bash": true, "zsh": true, "fish": true, "dash": true,
	"ksh": true, "tcsh": true, "csh": true, "nu": true, "login": true,
}

// paneState reports what is running in a tmux target: "agent", "shell", or "missing".
func paneState(target string) (state, command string) {
	out, err := exec.Command("tmux", "display-message", "-p", "-t", target, "#{pane_current_command}").Output()
	if err != nil {
		return "missing", ""
	}
	command = strings.TrimSpace(string(out))
	if command == "" {
		return "missing", ""
	}
	if shells[strings.TrimPrefix(filepath.Base(command), "-")] {
		return "shell", command
	}
	return "agent", command
}

// typeInto types text into the pane and presses Enter.
func typeInto(target, text string) error {
	if out, err := exec.Command("tmux", "send-keys", "-t", target, "-l", text).CombinedOutput(); err != nil {
		return fmt.Errorf("send-keys: %v: %s", err, strings.TrimSpace(string(out)))
	}
	// TUIs like Claude Code treat fast input as a paste; give it a beat before Enter.
	time.Sleep(400 * time.Millisecond)
	if out, err := exec.Command("tmux", "send-keys", "-t", target, "Enter").CombinedOutput(); err != nil {
		return fmt.Errorf("send-keys Enter: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// oneLine flattens board text for typing into a terminal. Control characters are dropped:
// an Escape inside a thread subject would interrupt the agent mid-turn.
func oneLine(s string, max int) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > max {
		return string(r[:max-1]) + "…"
	}
	return s
}

func wakeText(m Message) string {
	return fmt.Sprintf(
		"tagteam: %s tagged you in bbs thread %q (id %s). Read the thread, do the work, then reply there and @mention whoever should go next. Mention nobody if you are done.",
		oneLine(m.Author, 40), oneLine(m.Subject, 60), m.Thread)
}
