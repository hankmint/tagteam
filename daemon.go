// ABOUTME: The watch loop: polls the board, queues wake-ups, delivers them safely
// ABOUTME: Also start/stop/status for running it in the background, and the JSONL trace

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type state struct {
	LastRowID int64 `json:"last_rowid"`
}

func statePath() string { return filepath.Join(stateDir(), "state.json") }
func pidPath() string   { return filepath.Join(stateDir(), "tagteam.pid") }
func logPath() string   { return filepath.Join(stateDir(), "tagteam.log") }
func tracePath() string { return filepath.Join(stateDir(), "trace.jsonl") }

func loadState() (*state, bool) {
	raw, err := os.ReadFile(statePath())
	if err != nil {
		return &state{}, false
	}
	st := &state{}
	if json.Unmarshal(raw, st) != nil {
		return &state{}, false
	}
	return st, true
}

func (s *state) save() error {
	raw, _ := json.Marshal(s)
	tmp := statePath() + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, statePath())
}

// trace appends one event to trace.jsonl: the research log of every handoff.
type tracer struct{ f *os.File }

func (t *tracer) event(kind string, fields map[string]any) {
	fields["ts"] = time.Now().UTC().Format(time.RFC3339Nano)
	fields["event"] = kind
	raw, _ := json.Marshal(fields)
	t.f.Write(append(raw, '\n'))
}

type pending struct {
	msg    Message
	logged string // last reason we logged for not delivering, to avoid spamming the trace
}

type watcher struct {
	cfg     *Config
	board   *board
	st      *state
	trace   *tracer
	queue   map[string]*pending
	history map[string][]time.Time
}

func run(cfg *Config) error {
	if err := os.MkdirAll(stateDir(), 0o755); err != nil {
		return err
	}
	b, err := openBoard(cfg.DB)
	if err != nil {
		return err
	}
	defer b.Close()
	tf, err := os.OpenFile(tracePath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer tf.Close()

	st, ok := loadState()
	if !ok {
		// First run: start from now, never replay the board's history into live agents.
		if st.LastRowID, err = b.latestRowID(); err != nil {
			return err
		}
		if err := st.save(); err != nil {
			return err
		}
	}

	w := &watcher{cfg: cfg, board: b, st: st, trace: &tracer{tf},
		queue: map[string]*pending{}, history: map[string][]time.Time{}}

	fmt.Printf("tagteam watching %s every %s\n", cfg.DB, cfg.Poll)
	for _, a := range cfg.Agents {
		s, cmd := paneState(a.Target)
		fmt.Printf("  @%-10s -> %-24s %s %s\n", a.Name, a.Target, s, cmd)
	}
	w.trace.event("start", map[string]any{"from_rowid": st.LastRowID, "agents": cfg.Agents})

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	tick := time.NewTicker(cfg.Poll)
	defer tick.Stop()
	for {
		if err := w.poll(); err != nil {
			fmt.Fprintf(os.Stderr, "%s poll error: %v\n", time.Now().Format("15:04:05"), err)
		}
		w.deliver()
		select {
		case <-stop:
			w.trace.event("stop", map[string]any{})
			return nil
		case <-tick.C:
		}
	}
}

func (w *watcher) poll() error {
	msgs, err := w.board.since(w.st.LastRowID)
	if err != nil || len(msgs) == 0 {
		return err
	}
	for _, m := range msgs {
		to := recipients(m, w.cfg)
		w.trace.event("message", map[string]any{
			"msg_id": m.ID, "thread": m.Thread, "author": m.Author,
			"author_agent": authorAgent(m.Author, w.cfg), "tags": to, "chars": len(m.Content),
		})
		for _, name := range to {
			// Several tags for one agent before it wakes collapse into one wake for the latest.
			w.queue[name] = &pending{msg: m}
			fmt.Printf("%s %s tagged @%s in %q\n", time.Now().Format("15:04:05"), m.Author, name, oneLine(m.Subject, 40))
		}
		w.st.LastRowID = m.RowID
	}
	return w.st.save()
}

// deliver wakes queued agents whose pane is ready and who are within their limits.
func (w *watcher) deliver() {
	now := time.Now()
	for name, p := range w.queue {
		a := w.cfg.agent(name)
		recent := w.history[name][:0]
		for _, t := range w.history[name] {
			if now.Sub(t) < time.Hour {
				recent = append(recent, t)
			}
		}
		w.history[name] = recent

		reason := ""
		paneCmd := ""
		switch {
		case len(recent) >= w.cfg.MaxPerHour:
			reason = "hourly-cap"
		case len(recent) > 0 && now.Sub(recent[len(recent)-1]) < w.cfg.Cooldown:
			reason = "cooldown"
		default:
			var s string
			s, paneCmd = paneState(a.Target)
			if s != "agent" {
				reason = "pane-" + s
			}
		}

		fields := map[string]any{"agent": name, "target": a.Target, "msg_id": p.msg.ID, "thread": p.msg.Thread}
		if reason == "hourly-cap" {
			// A runaway loop: drop the wake rather than queue it forever.
			fields["status"] = "dropped"
			fields["reason"] = reason
			w.trace.event("wake", fields)
			fmt.Printf("%s @%s hit %d wakes/hour, dropping\n", now.Format("15:04:05"), name, w.cfg.MaxPerHour)
			delete(w.queue, name)
			continue
		}
		if reason != "" {
			if p.logged != reason {
				fields["status"] = "waiting"
				fields["reason"] = reason
				fields["pane_command"] = paneCmd
				w.trace.event("wake", fields)
				if reason != "cooldown" {
					fmt.Printf("%s @%s waiting: %s %s\n", now.Format("15:04:05"), name, reason, paneCmd)
				}
				p.logged = reason
			}
			continue
		}
		if err := typeInto(a.Target, wakeText(p.msg)); err != nil {
			if p.logged != "error" {
				fields["status"] = "error"
				fields["error"] = err.Error()
				w.trace.event("wake", fields)
				fmt.Printf("%s @%s delivery failed, will retry: %v\n", now.Format("15:04:05"), name, err)
				p.logged = "error"
			}
			continue
		}
		fields["status"] = "sent"
		fields["pane_command"] = paneCmd
		w.trace.event("wake", fields)
		fmt.Printf("%s woke @%s (%s)\n", now.Format("15:04:05"), name, a.Target)
		w.history[name] = append(w.history[name], now)
		delete(w.queue, name)
	}
}

func runningPID() int {
	raw, err := os.ReadFile(pidPath())
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil || pid <= 0 {
		return 0
	}
	// After a reboot the pid can belong to some other program; only trust it if it's us.
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "command=").Output()
	args := strings.Fields(string(out))
	if err != nil || len(args) < 2 || filepath.Base(args[0]) != "tagteam" || args[1] != "run" {
		os.Remove(pidPath())
		return 0
	}
	return pid
}

func start() error {
	if pid := runningPID(); pid != 0 {
		return fmt.Errorf("already running (pid %d)", pid)
	}
	if _, err := loadConfig(); err != nil {
		return err
	}
	if err := os.MkdirAll(stateDir(), 0o755); err != nil {
		return err
	}
	logf, err := os.OpenFile(logPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer logf.Close()
	self, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(self, "run")
	cmd.Stdout, cmd.Stderr = logf, logf
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	if err := os.WriteFile(pidPath(), []byte(strconv.Itoa(cmd.Process.Pid)), 0o644); err != nil {
		return err
	}
	fmt.Printf("started (pid %d), log: %s\n", cmd.Process.Pid, logPath())
	return nil
}

func stopDaemon() error {
	pid := runningPID()
	if pid == 0 {
		return errors.New("not running")
	}
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		return err
	}
	os.Remove(pidPath())
	fmt.Printf("stopped (pid %d)\n", pid)
	return nil
}

func status() error {
	if pid := runningPID(); pid != 0 {
		fmt.Printf("running (pid %d)\n", pid)
	} else {
		fmt.Println("stopped")
	}
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	fmt.Printf("config: %s\nboard:  %s\ntrace:  %s\n", configPath(), cfg.DB, tracePath())
	for _, a := range cfg.Agents {
		s, cmd := paneState(a.Target)
		fmt.Printf("  @%-10s -> %-24s %s %s\n", a.Name, a.Target, s, cmd)
	}
	return nil
}
