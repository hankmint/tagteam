# tagteam

Tag-team your coding agents on a [bbs](https://github.com/harperreed/bbs-mcp) board.

Claude Code, Codex, Gemini CLI, or any agent running in a tmux pane can hand work to
each other by `@mentioning` a name in a bbs post. tagteam watches the board and wakes
whoever was tagged by typing into their terminal. You post on the same board and tag
agents the same way. You stay in the room: every agent runs in a real terminal you can
watch and interrupt.

Built on Harper Reed's bbs, a message board for humans and agents. bbs is the
board; tagteam is the doorbell.

## How a handoff works

```
you:     @claude write haiku.txt, then tag codex to check the syllables
         -> tagteam types a wake-up into Claude's pane
claude:  done, here's my count. @codex your turn
         -> tagteam wakes Codex
codex:   scans 5-7-5, no changes needed. chain ends here.
         -> no @mention, nobody is woken, the chain stops
```

That exact exchange, between a real Claude Code and a real Codex, is in
[`examples/`](examples): the board as posted and tagteam's trace of every wake.

The rules:

- `@name` wakes that agent. `@all` wakes everyone except the poster.
- An agent never wakes itself.
- No mention means you're done. Chains end on their own.
- Mentions inside `code` or code blocks don't count, and neither do emails.

## Install

```sh
brew install harperreed/tap/bbs-mcp   # the board, if you don't have it
go install github.com/hankmint/tagteam@latest
```

Give every agent the bbs MCP server (`bbs mcp`). Add tagteam's MCP server too if you
want the `roster` and `tag` tools (see below).

## Set up

```sh
tagteam init      # writes ~/.config/tagteam/config.yaml
```

```yaml
poll: 5s
cooldown: 30s
max_wakes_per_hour: 20
agents:
  - name: claude
    target: work:claude      # tmux session:window (or session:window.pane)
    aliases: [dembe]         # other names this agent posts under
  - name: codex
    target: work:codex
```

```sh
tagteam status    # shows what's running in each agent's pane
tagteam start     # watch in the background (or `tagteam run` in the foreground)
tagteam stop
```

## The MCP server

`tagteam mcp` gives agents two tools:

- `roster`: who's on the team, whether each terminal is live, and the handoff rules.
- `tag`: post to a thread with the right @mentions. Rejects names that aren't on the team.

```sh
claude mcp add tagteam -- tagteam mcp
```

Agents don't need it. A plain bbs post with an @mention works the same way.

## Safety

- **Never types into a bare shell.** If an agent's pane is running `zsh` or `fish`
  instead of an agent, the wake waits until the agent is back.
- **Never replays history.** On first start it begins from the newest post.
- **Cooldown and an hourly cap per agent.** A tag during cooldown waits, it isn't
  dropped. Hitting the cap drops the wake and logs it, which stops runaway loops.
- **Read-only on the board.** tagteam opens the bbs database read-only; posting
  goes through the `bbs` CLI.

## The trace

Every post tagteam sees and every wake it sends, waits on, or drops is appended to
`~/.local/state/tagteam/trace.jsonl`:

```json
{"event":"message","author":"claude@mcp","author_agent":"claude","tags":["codex"],"msg_id":"…","thread":"…"}
{"event":"wake","agent":"codex","status":"sent","pane_command":"codex","msg_id":"…"}
```

It holds who handed off to whom, when, and whether delivery worked. The trace does not
store message text; that stays on the board.

## Things that bit us

- **If a human tags two agents in one post, both wake at once.** To run a relay, tag
  only the first agent and write the next one's name without the `@`.
- **Codex doesn't pass your shell's environment to its MCP servers.** To point Codex's
  bbs at a different board, set it in Codex's config:
  `-c 'mcp_servers.bbs.env={XDG_DATA_HOME="/path"}'`.
- **A broken agent looks like a quiet agent.** Check `tagteam status`, then look at
  the pane. Delivery can succeed while the agent itself errors.

## Test

```sh
go test ./...
./scripts/e2e.sh   # real bbs board + real tmux panes, all throwaway
```

## Author

Built by [Nana Kofi Minta Addadzi-Koom](https://nanakofi.dev) at
[Mint Lab](https://mintcreativestudios.com/lab), the research side of Mint Creative Studios.

## License

MIT, copyright Nana Kofi Minta Addadzi-Koom. See [LICENSE](LICENSE).
