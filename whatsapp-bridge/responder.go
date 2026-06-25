package main

// Auto-responder: on each inbound WhatsApp message, optionally invoke a headless
// `claude -p` call and (auto-)send its output back to the chat. Event-driven —
// hooked from handleMessage(). All behavior is data-driven via responder.json so
// chats can be added/changed without recompiling. Safety: allowlist, skip
// own-messages, per-chat debounce, dry-run, and a kill-switch touchfile.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

// ResponderChat configures auto-response behavior for a single chat (by JID).
type ResponderChat struct {
	JID              string `json:"jid"`
	Label            string `json:"label"`
	SystemPromptFile string `json:"system_prompt_file"`
	Enabled          *bool  `json:"enabled,omitempty"`        // per-chat override of global Enabled
	DryRun           *bool  `json:"dry_run,omitempty"`        // per-chat override of global DryRun
	RespondToSelf    *bool  `json:"respond_to_self,omitempty"` // answer own messages too (e.g. self-chat sandbox)
}

// ResponderConfig is the whole responder.json document.
type ResponderConfig struct {
	Enabled            bool            `json:"enabled"`
	DryRun             bool            `json:"dry_run"`
	ClaudeBin          string          `json:"claude_bin"`
	TimeoutSeconds     int             `json:"timeout_seconds"`
	MinIntervalSeconds int             `json:"min_interval_seconds"`
	KillSwitchFile     string          `json:"kill_switch_file"`
	Chats              []ResponderChat `json:"chats"`
}

func (cfg *ResponderConfig) findChat(jid string) *ResponderChat {
	for i := range cfg.Chats {
		if cfg.Chats[i].JID == jid {
			return &cfg.Chats[i]
		}
	}
	return nil
}

var (
	responderMu          sync.RWMutex
	responderConfig      *ResponderConfig
	responderConfigMtime time.Time
	responderBaseDir     string

	responderLastMu   sync.Mutex
	responderLastSent = map[string]time.Time{}
)

func responderConfigPath() string {
	if p := os.Getenv("WA_RESPONDER_CONFIG"); p != "" {
		return p
	}
	return "responder.json"
}

func expandResponderHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

// getResponderConfig lazily (re)loads responder.json, reloading whenever the
// file's mtime changes so edits take effect without restarting the bridge.
// Returns nil when no config file exists (responder fully off).
func getResponderConfig() *ResponderConfig {
	path := responderConfigPath()
	fi, err := os.Stat(path)
	if err != nil {
		responderMu.Lock()
		responderConfig = nil
		responderMu.Unlock()
		return nil
	}

	responderMu.RLock()
	cur, curMtime := responderConfig, responderConfigMtime
	responderMu.RUnlock()
	if cur != nil && fi.ModTime().Equal(curMtime) {
		return cur
	}

	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Printf("[responder] failed to read %s: %v\n", path, err)
		return cur
	}
	var cfg ResponderConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		fmt.Printf("[responder] config parse error in %s: %v\n", path, err)
		return cur
	}

	responderMu.Lock()
	responderConfig = &cfg
	responderConfigMtime = fi.ModTime()
	responderBaseDir = filepath.Dir(path)
	responderMu.Unlock()

	fmt.Printf("[responder] loaded %s: enabled=%v dry_run=%v chats=%d\n",
		path, cfg.Enabled, cfg.DryRun, len(cfg.Chats))
	return &cfg
}

func (cfg *ResponderConfig) promptPath(chat *ResponderChat) string {
	p := chat.SystemPromptFile
	if p == "" || filepath.IsAbs(p) {
		return p
	}
	responderMu.RLock()
	base := responderBaseDir
	responderMu.RUnlock()
	return filepath.Join(base, p)
}

// responderAllow enforces a per-chat minimum interval between auto-responses.
func responderAllow(jid string, minInterval int) bool {
	if minInterval <= 0 {
		return true
	}
	responderLastMu.Lock()
	defer responderLastMu.Unlock()
	now := time.Now()
	if last, ok := responderLastSent[jid]; ok && now.Sub(last) < time.Duration(minInterval)*time.Second {
		return false
	}
	responderLastSent[jid] = now
	return true
}

// maybeAutoRespond is the hook called from handleMessage for every inbound
// message. Cheap guards run synchronously; the claude exec + send run in a
// goroutine so the event handler is never blocked.
func maybeAutoRespond(client *whatsmeow.Client, msg *events.Message, content string) {
	cfg := getResponderConfig()
	if cfg == nil || !cfg.Enabled {
		return
	}
	if cfg.KillSwitchFile != "" {
		if _, err := os.Stat(expandResponderHome(cfg.KillSwitchFile)); err == nil {
			return // kill switch present → globally paused
		}
	}
	if content == "" {
		return
	}

	chatJID := msg.Info.Chat.String()
	chat := cfg.findChat(chatJID)
	if chat == nil {
		return // not on the allowlist
	}
	if chat.Enabled != nil && !*chat.Enabled {
		return
	}
	// Skip our own messages, unless this chat opts in (self-chat sandbox).
	respondToSelf := chat.RespondToSelf != nil && *chat.RespondToSelf
	if msg.Info.IsFromMe && !respondToSelf {
		return
	}
	if !responderAllow(chatJID, cfg.MinIntervalSeconds) {
		fmt.Printf("[responder] debounced %s (%s)\n", chat.Label, chatJID)
		return
	}

	go func() {
		promptPath := cfg.promptPath(chat)
		sysPrompt, err := os.ReadFile(promptPath)
		if err != nil {
			fmt.Printf("[responder] %s: cannot read prompt %s: %v\n", chat.Label, promptPath, err)
			return
		}

		reply, err := runClaude(cfg, string(sysPrompt), content)
		if err != nil {
			fmt.Printf("[responder] %s: claude error: %v\n", chat.Label, err)
			return
		}
		reply = strings.TrimSpace(reply)
		if reply == "" || reply == "SKIP" {
			fmt.Printf("[responder] %s: no reply (skipped)\n", chat.Label)
			return
		}

		dry := cfg.DryRun
		if chat.DryRun != nil {
			dry = *chat.DryRun
		}
		if dry {
			fmt.Printf("[responder][dry-run] %s (%s) would send: %s\n", chat.Label, chatJID, reply)
			return
		}
		sendResponderReply(client, chat, chatJID, reply)
	}()
}

// runClaude invokes `claude -p <message> --append-system-prompt <sys>` headless,
// returning trimmed stdout. The bridge — not claude — owns the send, so logging,
// debounce, and dry-run all bind reliably.
func runClaude(cfg *ResponderConfig, sysPrompt, userMsg string) (string, error) {
	bin := cfg.ClaudeBin
	if bin == "" {
		bin = "claude"
	}
	timeout := cfg.TimeoutSeconds
	if timeout <= 0 {
		timeout = 90
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, "-p", userMsg, "--append-system-prompt", sysPrompt)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%v: %s", err, strings.TrimSpace(errb.String()))
	}
	return out.String(), nil
}

func sendResponderReply(client *whatsmeow.Client, chat *ResponderChat, chatJID, text string) {
	jid, err := types.ParseJID(chatJID)
	if err != nil {
		fmt.Printf("[responder] %s: bad JID %s: %v\n", chat.Label, chatJID, err)
		return
	}
	resp, err := client.SendMessage(context.Background(), jid, &waProto.Message{
		Conversation: proto.String(text),
	})
	if err != nil {
		fmt.Printf("[responder] %s: send failed: %v\n", chat.Label, err)
		return
	}
	fmt.Printf("[responder] %s (%s) sent (ID: %s): %s\n", chat.Label, chatJID, resp.ID, text)
}
