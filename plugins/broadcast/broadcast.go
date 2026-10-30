package broadcast

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/amarnathcjd/gogram/telegram"
	coreBroadcast "github.com/itsafxal/AutoFilter-Go/internal/broadcast"
	"github.com/itsafxal/AutoFilter-Go/internal/plugins"
)

// Plugin implements administrative broadcast commands and callbacks.
type Plugin struct {
	appCtx *plugins.AppContext
	engine coreBroadcast.Service
}

// New creates a new broadcast plugin instance.
func New(appCtx *plugins.AppContext, engine coreBroadcast.Service) *Plugin {
	return &Plugin{
		appCtx: appCtx,
		engine: engine,
	}
}

// Name returns the unique plugin name.
func (p *Plugin) Name() string {
	return "broadcast"
}

// Commands returns the command definitions for broadcasting.
func (p *Plugin) Commands() []plugins.CommandDefinition {
	return []plugins.CommandDefinition{
		{
			Name:        "broadcast",
			Aliases:     []string{"bc", "bcast"},
			Description: "Broadcast replied message to users and/or groups with bounded concurrency",
			Usage:       "/broadcast [-users|-groups|-all] [-copy|-forward] [-skip=N] [-c=N]",
			Category:    "Admin Commands",
			AdminOnly:   true,
			Handler:     p.HandleBroadcast,
		},
		{
			Name:        "broadcast_groups",
			Aliases:     []string{"bc_groups", "bcgroups"},
			Description: "Broadcast replied message exclusively to registered groups",
			Usage:       "/broadcast_groups [-copy|-forward] [-skip=N]",
			Category:    "Admin Commands",
			AdminOnly:   true,
			Handler:     p.HandleBroadcastGroups,
		},
		{
			Name:        "cancel_broadcast",
			Aliases:     []string{"stop_broadcast", "stopbc", "cancelbc"},
			Description: "Stop and cancel any currently active broadcast operation",
			Usage:       "/cancel_broadcast",
			Category:    "Admin Commands",
			AdminOnly:   true,
			Handler:     p.HandleCancelBroadcast,
		},
		{
			Name:        "broadcast_stats",
			Aliases:     []string{"bc_stats", "bcstatus"},
			Description: "View real-time progress or last completed broadcast statistics",
			Usage:       "/broadcast_stats",
			Category:    "Admin Commands",
			AdminOnly:   true,
			Handler:     p.HandleBroadcastStats,
		},
	}
}

// CallbackHandlers returns the inline button callback queries.
func (p *Plugin) CallbackHandlers() []plugins.CallbackDefinition {
	return []plugins.CallbackDefinition{
		{
			Prefix:  "cancel_broadcast",
			Handler: p.HandleCancelCallback,
		},
	}
}

// HandleBroadcast processes the main /broadcast command.
func (p *Plugin) HandleBroadcast(m *telegram.NewMessage) error {
	return p.initiateBroadcast(m, coreBroadcast.TargetUsers)
}

// HandleBroadcastGroups processes the /broadcast_groups command.
func (p *Plugin) HandleBroadcastGroups(m *telegram.NewMessage) error {
	return p.initiateBroadcast(m, coreBroadcast.TargetGroups)
}

func (p *Plugin) initiateBroadcast(m *telegram.NewMessage, defaultTarget coreBroadcast.TargetType) error {
	if m == nil {
		return nil
	}

	// 1. Verify that the command is in reply to a message
	if !m.IsReply() {
		_, err := m.Reply(
			"⚠️ <b>Broadcast Source Message Required!</b>\n\n"+
				"Please <b>reply to the message</b> (text, photo, video, document, audio, sticker) you wish to broadcast with:\n"+
				"<code>/broadcast [flags]</code>\n\n"+
				"<b>Supported Flags:</b>\n"+
				"• <code>-users</code> (or <code>-u</code>): Broadcast to users (default)\n"+
				"• <code>-groups</code> (or <code>-g</code>): Broadcast to groups\n"+
				"• <code>-all</code> (or <code>-a</code>): Broadcast to both users & groups\n"+
				"• <code>-copy</code>: Forward as copy without author header (default)\n"+
				"• <code>-forward</code>: Forward with original attribution tag\n"+
				"• <code>-skip=N</code>: Skip the first N recipients\n"+
				"• <code>-c=N</code>: Set worker concurrency (1–50)\n\n"+
				"<b>Examples:</b>\n"+
				"• <code>/broadcast</code> (replying to message)\n"+
				"• <code>/broadcast -groups</code>\n"+
				"• <code>/broadcast -all -copy -skip=250</code>",
			&telegram.SendOptions{ParseMode: "html"},
		)
		return err
	}

	// 2. Fetch the replied-to source message
	sourceMsg, err := m.GetReplyMessage()
	if err != nil || sourceMsg == nil {
		_, rErr := m.Reply("❌ <b>Failed to resolve the replied message.</b> Please reply directly to the content.", &telegram.SendOptions{ParseMode: "html"})
		return rErr
	}

	// 3. Guard against concurrent broadcasts
	if p.engine != nil && p.engine.IsBroadcasting() {
		_, rErr := m.Reply(
			"⚠️ <b>Another Broadcast is Currently Active!</b>\n\n"+
				"Please wait for the running broadcast to complete, or stop it via:\n"+
				"<code>/cancel_broadcast</code>",
			&telegram.SendOptions{ParseMode: "html"},
		)
		return rErr
	}

	// 4. Parse command arguments and flags
	opts := p.parseBroadcastArgs(m.Args(), defaultTarget)
	if p.appCtx != nil && p.appCtx.Config != nil && opts.Concurrency <= 0 {
		opts.Concurrency = p.appCtx.Config.BroadcastConcurrency
	}
	opts.AdminID = m.SenderID()

	// 5. Send initial progress message
	progressMsg, err := m.Reply("⏳ <b>Initializing high-performance broadcast engine...</b>", &telegram.SendOptions{ParseMode: "html"})
	if err != nil {
		return err
	}

	// 6. Launch the broadcast job
	if p.engine == nil {
		_, _ = progressMsg.Edit("❌ <b>Broadcast service engine is uninitialized.</b>", &telegram.SendOptions{ParseMode: "html"})
		return nil
	}

	ctx := context.Background()
	if startErr := p.engine.StartBroadcast(ctx, sourceMsg, progressMsg, opts); startErr != nil {
		_, _ = progressMsg.Edit(fmt.Sprintf("❌ <b>Failed to start broadcast:</b> %s", startErr.Error()), &telegram.SendOptions{ParseMode: "html"})
		return startErr
	}

	return nil
}

// HandleCancelBroadcast cancels any running broadcast.
func (p *Plugin) HandleCancelBroadcast(m *telegram.NewMessage) error {
	if m == nil {
		return nil
	}

	if p.engine == nil || !p.engine.IsBroadcasting() {
		_, err := m.Reply("ℹ️ <b>No active broadcast is currently in progress.</b>", &telegram.SendOptions{ParseMode: "html"})
		return err
	}

	stopped := p.engine.CancelBroadcast()
	if stopped {
		_, err := m.Reply("🛑 <b>Cancelling active broadcast... please wait for workers to drain.</b>", &telegram.SendOptions{ParseMode: "html"})
		return err
	}

	_, err := m.Reply("ℹ️ <b>No active broadcast was running.</b>", &telegram.SendOptions{ParseMode: "html"})
	return err
}

// HandleBroadcastStats displays current or recent broadcast statistics.
func (p *Plugin) HandleBroadcastStats(m *telegram.NewMessage) error {
	if m == nil {
		return nil
	}

	if p.engine == nil {
		_, err := m.Reply("❌ Broadcast service engine is not configured.", &telegram.SendOptions{ParseMode: "html"})
		return err
	}

	stats, exists := p.engine.GetStats()
	if !exists {
		_, err := m.Reply("ℹ️ <b>No broadcast history available for this session.</b>", &telegram.SendOptions{ParseMode: "html"})
		return err
	}

	statusIcon := "⏳"
	switch stats.Status {
	case coreBroadcast.StatusCompleted:
		statusIcon = "✅"
	case coreBroadcast.StatusCancelled:
		statusIcon = "🛑"
	case coreBroadcast.StatusRunning:
		statusIcon = "📢"
	}

	text := fmt.Sprintf(
		"%s <b>Broadcast Statistics</b>\n\n"+
			"• <b>Status:</b> <code>%s</code>\n"+
			"• <b>Target:</b> <code>%s</code>\n"+
			"• <b>Mode:</b> <code>%s</code>\n"+
			"• <b>Total Recipients:</b> <code>%d</code>\n"+
			"• <b>Processed:</b> <code>%d</code>\n"+
			"• <b>Successfully Sent:</b> <code>%d</code>\n"+
			"• <b>Failed:</b> <code>%d</code>\n"+
			"• <b>Blocked Users:</b> <code>%d</code>\n"+
			"• <b>Deactivated Users:</b> <code>%d</code>\n"+
			"• <b>Delivery Speed:</b> <code>%.1f msgs/sec</code>",
		statusIcon,
		stats.Status,
		stats.Target,
		stats.Mode,
		stats.TotalRecipients,
		stats.Processed,
		stats.Successful,
		stats.Failed,
		stats.Blocked,
		stats.Deactivated,
		stats.Speed,
	)

	_, err := m.Reply(text, &telegram.SendOptions{ParseMode: "html"})
	return err
}

// HandleCancelCallback handles inline button taps on the "🛑 Cancel Broadcast" button.
func (p *Plugin) HandleCancelCallback(cb *telegram.CallbackQuery) error {
	if cb == nil {
		return nil
	}

	// Verify admin authority
	senderID := cb.SenderID
	if p.appCtx != nil && p.appCtx.Config != nil && !p.appCtx.Config.IsAdmin(senderID) {
		_, _ = cb.Answer("⚠️ Access Denied: Only administrators can cancel broadcasts.", &telegram.CallbackOptions{
			Alert: true,
		})
		return nil
	}

	if p.engine == nil || !p.engine.IsBroadcasting() {
		_, _ = cb.Answer("ℹ️ No active broadcast in progress.", &telegram.CallbackOptions{
			Alert: false,
		})
		return nil
	}

	p.engine.CancelBroadcast()
	_, _ = cb.Answer("🛑 Broadcast cancellation requested!", &telegram.CallbackOptions{
		Alert: true,
	})

	return nil
}

// parseBroadcastArgs parses flags like -users, -groups, -all, -copy, -forward, -skip=N, -c=N.
func (p *Plugin) parseBroadcastArgs(args string, defaultTarget coreBroadcast.TargetType) coreBroadcast.Options {
	opts := coreBroadcast.Options{
		Target: defaultTarget,
		Mode:   coreBroadcast.ModeCopy,
		Skip:   0,
	}

	fields := strings.Fields(args)
	for _, f := range fields {
		fLower := strings.ToLower(f)

		switch {
		case fLower == "-users" || fLower == "-u" || fLower == "-user":
			opts.Target = coreBroadcast.TargetUsers
		case fLower == "-groups" || fLower == "-g" || fLower == "-group":
			opts.Target = coreBroadcast.TargetGroups
		case fLower == "-all" || fLower == "-a":
			opts.Target = coreBroadcast.TargetAll
		case fLower == "-copy" || fLower == "-as_copy":
			opts.Mode = coreBroadcast.ModeCopy
		case fLower == "-forward" || fLower == "-fwd":
			opts.Mode = coreBroadcast.ModeForward
		case strings.HasPrefix(fLower, "-skip="):
			if val, err := strconv.ParseInt(strings.TrimPrefix(fLower, "-skip="), 10, 64); err == nil && val >= 0 {
				opts.Skip = val
			}
		case strings.HasPrefix(fLower, "-c=") || strings.HasPrefix(fLower, "-concurrency="):
			parts := strings.Split(fLower, "=")
			if len(parts) == 2 {
				if val, err := strconv.Atoi(parts[1]); err == nil && val > 0 {
					opts.Concurrency = val
				}
			}
		default:
			// If a bare positive integer is provided, treat it as skip count (compatible with TGxCyberBot syntax)
			if val, err := strconv.ParseInt(f, 10, 64); err == nil && val > 0 && opts.Skip == 0 {
				opts.Skip = val
			}
		}
	}

	return opts
}
