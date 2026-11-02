// ABOUTME: Decides which agents a board message wakes
// ABOUTME: Pure functions: @mention parsing and author-to-agent matching

package main

import (
	"regexp"
	"strings"
)

// Message is one bbs post as tagteam sees it.
type Message struct {
	RowID     int64
	ID        string
	Thread    string
	Subject   string
	Author    string
	Content   string
	CreatedAt string
}

var (
	fencedCode = regexp.MustCompile("(?s)```.*?```")
	inlineCode = regexp.MustCompile("`[^`\n]*`")
	// A mention must not follow a word character, so emails like me@codex.com don't count.
	mentionRe = regexp.MustCompile(`(?i)(?:^|[^\w@])@([a-z0-9_-]+)`)
)

// mentions returns the configured agents tagged in content, in config order.
// Mentions inside code are ignored so quoting "@codex" in a snippet wakes nobody.
func mentions(content string, cfg *Config) []string {
	text := inlineCode.ReplaceAllString(fencedCode.ReplaceAllString(content, " "), " ")
	tagged := map[string]bool{}
	for _, m := range mentionRe.FindAllStringSubmatch(text, -1) {
		tagged[strings.ToLower(m[1])] = true
	}
	var out []string
	for _, a := range cfg.Agents {
		if tagged["all"] || tagged[a.Name] {
			out = append(out, a.Name)
		}
	}
	return out
}

var firstWord = regexp.MustCompile(`[a-z0-9_-]+`)

// authorAgent maps a bbs identity like "Codex@mcp" or "Dembe (Opus)@mcp" to a configured
// agent name by its first word, or "" for humans and unknown posters.
func authorAgent(author string, cfg *Config) string {
	user := firstWord.FindString(strings.ToLower(strings.SplitN(author, "@", 2)[0]))
	for _, a := range cfg.Agents {
		if user == a.Name {
			return a.Name
		}
		for _, alias := range a.Aliases {
			if user == alias {
				return a.Name
			}
		}
	}
	return ""
}

// recipients is who a message wakes: everyone tagged except the author.
func recipients(m Message, cfg *Config) []string {
	self := authorAgent(m.Author, cfg)
	var out []string
	for _, name := range mentions(m.Content, cfg) {
		if name != self {
			out = append(out, name)
		}
	}
	return out
}
