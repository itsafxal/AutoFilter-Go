package cmds

import (
	"github.com/amarnathcjd/gogram/telegram"
	"github.com/itsafxal/MYTGBOTZ/internal/plugins"
)

// Plugin implements the /cmds and /help command plugin.
type Plugin struct {
	appCtx *plugins.AppContext
}

// New creates a new cmds plugin instance.
func New(appCtx *plugins.AppContext) *Plugin {
	return &Plugin{appCtx: appCtx}
}

// Name returns the unique plugin name.
func (p *Plugin) Name() string {
	return "cmds"
}

// Commands returns the command definitions for cmds plugin.
func (p *Plugin) Commands() []plugins.CommandDefinition {
	return []plugins.CommandDefinition{
		{
			Name:        "cmds",
			Aliases:     []string{"help"},
			Description: "View list of available commands and usage guide",
			Usage:       "/cmds",
			Category:    "General",
			Handler:     p.HandleCmds,
		},
	}
}

// HandleCmds renders the dynamically generated help text based on registered plugins.
func (p *Plugin) HandleCmds(m *telegram.NewMessage) error {
	helpText := "<b><u>🤖 BOT COMMANDS & USAGE GUIDE 🤖</u></b>\n\nNo commands registered."
	if p.appCtx != nil && p.appCtx.Registry != nil {
		helpText = p.appCtx.Registry.GenerateHelpText()
	}

	_, err := m.Reply(helpText, &telegram.SendOptions{
		ParseMode: "html",
	})
	return err
}
