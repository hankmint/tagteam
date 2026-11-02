// ABOUTME: MCP server so agents can see the team and tag each other in
// ABOUTME: Posting goes through the bbs CLI, so tagteam never writes to bbs's database

package main

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type rosterIn struct{}

type rosterAgent struct {
	Name    string   `json:"name"`
	Aliases []string `json:"aliases,omitempty"`
	Pane    string   `json:"pane" jsonschema:"agent, shell, or missing"`
}

type rosterOut struct {
	Agents []rosterAgent `json:"agents"`
	Rules  string        `json:"rules"`
}

type tagIn struct {
	Thread    string   `json:"thread" jsonschema:"bbs thread id or subject to post in"`
	To        []string `json:"to" jsonschema:"agent names to tag in, or [\"all\"]"`
	Message   string   `json:"message" jsonschema:"what you did and what you need from them"`
	AgentName string   `json:"agent_name" jsonschema:"your own name, as listed by roster"`
}

type tagOut struct {
	Posted string `json:"posted"`
}

const rules = "Tag an agent in by @mentioning its name in a bbs post (or use the tag tool). " +
	"tagteam types a wake-up into that agent's terminal. Tag exactly who should go next. " +
	"Mention nobody when you are done; the handoff chain ends there."

func serveMCP(ctx context.Context, cfg *Config) error {
	server := mcp.NewServer(&mcp.Implementation{Name: "tagteam", Version: version}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "roster",
		Description: "List the agents on this tagteam, whether each one's terminal is live, and the handoff rules.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, _ rosterIn) (*mcp.CallToolResult, rosterOut, error) {
		out := rosterOut{Rules: rules}
		for _, a := range cfg.Agents {
			s, _ := paneState(a.Target)
			out.Agents = append(out.Agents, rosterAgent{Name: a.Name, Aliases: a.Aliases, Pane: s})
		}
		return nil, out, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "tag",
		Description: "Hand off to other agents: posts your message to a bbs thread with @mentions, which wakes them.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in tagIn) (*mcp.CallToolResult, tagOut, error) {
		if strings.TrimSpace(in.Thread) == "" || strings.TrimSpace(in.Message) == "" {
			return nil, tagOut{}, fmt.Errorf("thread and message are required")
		}
		if strings.TrimSpace(in.AgentName) == "" {
			return nil, tagOut{}, fmt.Errorf("agent_name is required so the board shows who handed off")
		}
		if len(in.To) == 0 {
			return nil, tagOut{}, fmt.Errorf("to is required; to finish without handing off, post to bbs without a mention")
		}
		var tags []string
		for _, name := range in.To {
			name = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(name), "@"))
			if name != "all" && cfg.agent(name) == nil {
				return nil, tagOut{}, fmt.Errorf("no agent named %q; call roster to see the team", name)
			}
			tags = append(tags, "@"+name)
		}
		content := strings.Join(tags, " ") + " " + strings.TrimSpace(in.Message)
		// "--" keeps a thread or name that starts with "-" from being read as a bbs flag.
		cmd := exec.CommandContext(ctx, cfg.Bbs, "post", "--as="+in.AgentName, "--", in.Thread, content)
		if out, err := cmd.CombinedOutput(); err != nil {
			return nil, tagOut{}, fmt.Errorf("bbs post failed: %v: %s", err, strings.TrimSpace(string(out)))
		}
		return nil, tagOut{Posted: content}, nil
	})

	return server.Run(ctx, &mcp.StdioTransport{})
}
