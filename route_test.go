// ABOUTME: Tests for @mention routing
// ABOUTME: Covers self-tags, aliases, @all, emails, and mentions inside code

package main

import (
	"reflect"
	"strings"
	"testing"
)

func testConfig(t *testing.T) *Config {
	t.Helper()
	cfg := &Config{Agents: []Agent{
		{Name: "claude", Target: "w:claude", Aliases: []string{"Dembe"}},
		{Name: "codex", Target: "w:codex"},
		{Name: "gemini", Target: "w:gemini"},
	}}
	if err := cfg.normalize(); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestRecipients(t *testing.T) {
	cfg := testConfig(t)
	cases := []struct {
		name, author, content string
		want                  []string
	}{
		{"plain tag", "Claude@mcp", "@codex please review", []string{"codex"}},
		{"case insensitive", "mint@cli", "@CODEX and @Gemini go", []string{"codex", "gemini"}},
		{"self tag skipped", "codex@mcp", "@codex note to self, @claude your turn", []string{"claude"}},
		{"alias is self", "Dembe (Opus)@mcp", "@claude @codex", []string{"codex"}},
		{"all minus author", "Codex@mcp", "@all standup", []string{"claude", "gemini"}},
		{"human all", "mint@cli", "@all", []string{"claude", "codex", "gemini"}},
		{"no tag ends chain", "Claude@mcp", "done, shipped it", nil},
		{"email is not a tag", "mint@cli", "mail me at kay@codex.dev", nil},
		{"inline code ignored", "Claude@mcp", "the regex matches `@codex` fine", nil},
		{"fenced code ignored", "Claude@mcp", "```\n@codex\n```\nall good", nil},
		{"unknown name ignored", "Claude@mcp", "@cursor hi", nil},
		{"longer name not prefix", "Claude@mcp", "@codex-reviewer hi", nil},
		{"multiline", "Claude@mcp", "line one\n\n@codex line three", []string{"codex"}},
		{"punctuation", "Claude@mcp", "(cc @codex).", []string{"codex"}},
	}
	for _, c := range cases {
		got := recipients(Message{Author: c.author, Content: c.content}, cfg)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestConfigRejectsBadNames(t *testing.T) {
	for _, agents := range [][]Agent{
		{{Name: "all", Target: "x"}},
		{{Name: "has space", Target: "x"}},
		{{Name: "a", Target: ""}},
		{{Name: "a", Target: "x"}, {Name: "b", Target: "y", Aliases: []string{"a"}}},
		{{Name: "a", Target: "x", Aliases: []string{"dembe opus"}}},
		{{Name: "a", Target: "x", Aliases: []string{"all"}}},
	} {
		if err := (&Config{Agents: agents}).normalize(); err == nil {
			t.Errorf("expected error for %+v", agents)
		}
	}
}

func TestWakeTextIsSafeToType(t *testing.T) {
	m := Message{Author: "evil\x1b[2J@mcp", Subject: "fix\x1bthis\nnow\r\tplease", Thread: "t1"}
	got := wakeText(m)
	for _, r := range got {
		if r < 0x20 || r == 0x7f {
			t.Fatalf("control character %q in wake text: %q", r, got)
		}
	}
	if want := `"fixthis now please"`; !strings.Contains(got, want) {
		t.Errorf("subject not flattened, got %q", got)
	}
}
