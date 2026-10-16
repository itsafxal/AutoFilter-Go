package ping

import (
	"fmt"
	"runtime"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/itsafxal/MYTGBOTZ/internal/commands"
	"github.com/itsafxal/MYTGBOTZ/internal/plugins"
)

// Plugin implements the /ping command plugin.
type Plugin struct {
	appCtx *plugins.AppContext
}

// New creates a new ping plugin instance.
func New(appCtx *plugins.AppContext) *Plugin {
	return &Plugin{appCtx: appCtx}
}

// Name returns the unique plugin name.
func (p *Plugin) Name() string {
	return "ping"
}

// Commands returns the command definitions for ping plugin.
func (p *Plugin) Commands() []plugins.CommandDefinition {
	return []plugins.CommandDefinition{
		{
			Name:        "ping",
			Description: "Check bot response speed and Telegram API latency",
			Usage:       "/ping",
			Category:    "General",
			Handler:     p.HandlePing,
		},
	}
}

// BuildPingReport creates the formatted report text distinguishing local processing time
// from Telegram API round-trip latency.
func BuildPingReport(localDuration, apiRoundTrip time.Duration, startTime time.Time) string {
	uptime := commands.FormatUptime(startTime)
	localTimeStr := commands.FormatDurationMs(localDuration)
	apiTimeStr := commands.FormatDurationMs(apiRoundTrip)
	goroutines := runtime.NumGoroutine()
	goVersion := runtime.Version()

	return fmt.Sprintf(
		"📡 <b>Ping Status Report</b> 📊\n\n"+
			"  ➡️ <b>Telegram API Round-Trip:</b> <code>%s</code>\n"+
			"  ⏱️ <b>Local Processing Time:</b> <code>%s</code>\n"+
			"  ⏱️ <b>Bot Uptime:</b> <code>%s</code>\n"+
			"  🤖 <b>Go Runtime:</b> <code>%s</code>\n"+
			"  🧠 <b>Goroutines:</b> <code>%d</code>",
		apiTimeStr,
		localTimeStr,
		uptime,
		goVersion,
		goroutines,
	)
}

// HandlePing handles the /ping command.
func (p *Plugin) HandlePing(b *gotgbot.Bot, ctx *ext.Context) error {
	localStart := time.Now()

	// Local processing time: time taken before initiating the API call
	localProcessingDuration := time.Since(localStart)

	// Telegram API round-trip latency: time taken to send and acknowledge the reply
	apiStart := time.Now()
	sentMsg, err := ctx.EffectiveMessage.Reply(b, "⚡️ Pinging...", &gotgbot.SendMessageOpts{
		ParseMode: "HTML",
	})
	if err != nil {
		return err
	}
	apiRoundTrip := time.Since(apiStart)

	var startTime time.Time
	if p.appCtx != nil {
		startTime = p.appCtx.StartTime
	}

	report := BuildPingReport(localProcessingDuration, apiRoundTrip, startTime)

	_, _, err = sentMsg.EditText(b, &gotgbot.EditMessageTextOpts{
		Text:      report,
		ParseMode: "HTML",
	})
	return err
}
