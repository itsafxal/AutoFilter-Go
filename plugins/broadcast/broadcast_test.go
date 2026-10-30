package broadcast

import (
	"testing"

	coreBroadcast "github.com/itsafxal/AutoFilter-Go/internal/broadcast"
)

func TestParseBroadcastArgs(t *testing.T) {
	p := &Plugin{}

	// 1. Defaults
	opts := p.parseBroadcastArgs("", coreBroadcast.TargetUsers)
	if opts.Target != coreBroadcast.TargetUsers {
		t.Errorf("expected TargetUsers, got %s", opts.Target)
	}
	if opts.Mode != coreBroadcast.ModeCopy {
		t.Errorf("expected ModeCopy, got %s", opts.Mode)
	}
	if opts.Skip != 0 {
		t.Errorf("expected Skip 0, got %d", opts.Skip)
	}

	// 2. Groups flag
	optsG := p.parseBroadcastArgs("-groups -forward -skip=150 -c=20", coreBroadcast.TargetUsers)
	if optsG.Target != coreBroadcast.TargetGroups {
		t.Errorf("expected TargetGroups, got %s", optsG.Target)
	}
	if optsG.Mode != coreBroadcast.ModeForward {
		t.Errorf("expected ModeForward, got %s", optsG.Mode)
	}
	if optsG.Skip != 150 {
		t.Errorf("expected Skip 150, got %d", optsG.Skip)
	}
	if optsG.Concurrency != 20 {
		t.Errorf("expected Concurrency 20, got %d", optsG.Concurrency)
	}

	// 3. All flag & numeric skip fallback
	optsAll := p.parseBroadcastArgs("-all -copy 500", coreBroadcast.TargetUsers)
	if optsAll.Target != coreBroadcast.TargetAll {
		t.Errorf("expected TargetAll, got %s", optsAll.Target)
	}
	if optsAll.Mode != coreBroadcast.ModeCopy {
		t.Errorf("expected ModeCopy, got %s", optsAll.Mode)
	}
	if optsAll.Skip != 500 {
		t.Errorf("expected Skip 500, got %d", optsAll.Skip)
	}
}

func TestPluginRegistration(t *testing.T) {
	p := New(nil, nil)

	if p.Name() != "broadcast" {
		t.Errorf("expected plugin name 'broadcast', got %q", p.Name())
	}

	cmds := p.Commands()
	if len(cmds) != 4 {
		t.Fatalf("expected 4 commands, got %d", len(cmds))
	}

	cmdMap := make(map[string]bool)
	for _, c := range cmds {
		cmdMap[c.Name] = true
		if !c.AdminOnly {
			t.Errorf("expected command %s to be AdminOnly", c.Name)
		}
	}

	expectedCmds := []string{"broadcast", "broadcast_groups", "cancel_broadcast", "broadcast_stats"}
	for _, ec := range expectedCmds {
		if !cmdMap[ec] {
			t.Errorf("expected command %s to be registered", ec)
		}
	}

	cbs := p.CallbackHandlers()
	if len(cbs) != 1 || cbs[0].Prefix != "cancel_broadcast" {
		t.Errorf("expected callback handler for 'cancel_broadcast'")
	}
}
