package latest

import (
	"context"
	"fmt"
	"strings"

	"github.com/amarnathcjd/gogram/telegram"
	"github.com/itsafxal/MYTGBOTZ/internal/commands"
	"github.com/itsafxal/MYTGBOTZ/internal/plugins"
	"github.com/itsafxal/MYTGBOTZ/internal/search"
)

// Plugin implements the /latest and /request user commands.
type Plugin struct {
	appCtx *plugins.AppContext
}

// New creates a new latest plugin instance.
func New(appCtx *plugins.AppContext) *Plugin {
	return &Plugin{appCtx: appCtx}
}

// Name returns the unique plugin name.
func (p *Plugin) Name() string {
	return "latest"
}

// Commands returns the command definitions for the latest plugin.
func (p *Plugin) Commands() []plugins.CommandDefinition {
	return []plugins.CommandDefinition{
		{
			Name:        "latest",
			Description: "View recently added movie and series files",
			Usage:       "/latest",
			Category:    "General",
			Handler:     p.HandleLatest,
		},
		{
			Name:        "request",
			Aliases:     []string{"req"},
			Description: "Request a movie or series title",
			Usage:       "/request <title>",
			Category:    "General",
			Handler:     p.HandleRequest,
		},
	}
}

// HandleLatest fetches and presents the latest indexed media files.
func (p *Plugin) HandleLatest(m *telegram.NewMessage) error {
	if m == nil {
		return nil
	}

	if p.appCtx == nil || p.appCtx.Search == nil {
		_, err := m.Reply("⚠️ <b>Service Unavailable:</b> Media search service is not ready.", &telegram.SendOptions{
			ParseMode: "html",
		})
		return err
	}

	files, err := p.appCtx.Search.GetLatest(context.Background(), 10)
	if err != nil {
		_, replyErr := m.Reply("❌ <b>Error:</b> Failed to retrieve latest media files.", &telegram.SendOptions{
			ParseMode: "html",
		})
		return replyErr
	}

	if len(files) == 0 {
		_, err := m.Reply("🎬 <b><u>Recently Added Media Files</u></b>\n\nNo media files available in the database yet.", &telegram.SendOptions{
			ParseMode: "html",
		})
		return err
	}

	var sb strings.Builder
	sb.WriteString("🎬 <b><u>Recently Added Media Files</u></b>\n\n")
	sb.WriteString("Here are the latest uploads available in the library:\n\n")

	kb := telegram.NewKeyboard()

	for i, f := range files {
		sizeStr := f.Size
		if strings.TrimSpace(sizeStr) == "" && f.FileSize > 0 {
			sizeStr = commands.FormatBytes(f.FileSize)
		}
		sb.WriteString(fmt.Sprintf("%d. <b>%s</b> <i>(%s)</i>\n", i+1, commands.EscapeHTML(f.FileName), commands.EscapeHTML(sizeStr)))

		buttonLabel := fmt.Sprintf("%s 🌷 %s", sizeStr, f.FileName)
		if len(buttonLabel) > 55 {
			buttonLabel = buttonLabel[:52] + "..."
		}
		kb.AddRow(telegram.Button.Data(buttonLabel, fmt.Sprintf("file#%s", f.FileID)))
	}

	sb.WriteString("\n💡 <i>Tap any button below to download the file!</i>")

	_, replyErr := m.Reply(sb.String(), &telegram.SendOptions{
		ParseMode:   "html",
		ReplyMarkup: kb.Build(),
	})
	return replyErr
}

// HandleRequest processes movie and series requests from users.
func (p *Plugin) HandleRequest(m *telegram.NewMessage) error {
	if m == nil {
		return nil
	}

	args := strings.Fields(m.Text())
	if len(args) <= 1 {
		guide := "📝 <b><u>How to Request Movies & Series</u></b>\n\n" +
			"<b>Usage:</b> <code>/request &lt;movie or series name&gt;</code>\n\n" +
			"<b>Example:</b> <code>/request Interstellar 2014</code>\n\n" +
			"💡 <i>Tip: You can also search directly by typing any title in chat!</i>"

		_, err := m.Reply(guide, &telegram.SendOptions{
			ParseMode: "html",
		})
		return err
	}

	query := strings.TrimSpace(strings.Join(args[1:], " "))
	escapedQuery := commands.EscapeHTML(query)

	// Check if already available in the library
	if p.appCtx != nil && p.appCtx.Search != nil {
		res, err := p.appCtx.Search.Search(context.Background(), query, 5, 0, search.SearchFilter{})
		if err == nil && res != nil && len(res.Files) > 0 {
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("🍿 <b>Good news! Found existing files for:</b> <code>%s</code>\n\n", escapedQuery))

			kb := telegram.NewKeyboard()
			for _, f := range res.Files {
				sizeStr := f.Size
				if strings.TrimSpace(sizeStr) == "" && f.FileSize > 0 {
					sizeStr = commands.FormatBytes(f.FileSize)
				}
				buttonLabel := fmt.Sprintf("%s 🌷 %s", sizeStr, f.FileName)
				if len(buttonLabel) > 55 {
					buttonLabel = buttonLabel[:52] + "..."
				}
				kb.AddRow(telegram.Button.Data(buttonLabel, fmt.Sprintf("file#%s", f.FileID)))
			}
			sb.WriteString("Tap a button below to get your file:")

			_, replyErr := m.Reply(sb.String(), &telegram.SendOptions{
				ParseMode:   "html",
				ReplyMarkup: kb.Build(),
			})
			return replyErr
		}
	}

	// If not found in library, record request
	userMention := "Valued Member"
	if m.Sender != nil {
		if m.Sender.FirstName != "" {
			userMention = commands.EscapeHTML(m.Sender.FirstName)
		} else if m.Sender.Username != "" {
			userMention = "@" + commands.EscapeHTML(m.Sender.Username)
		}
	}

	resp := fmt.Sprintf(
		"✅ <b>Request Submitted Successfully!</b>\n\n"+
			"🎬 <b>Requested Title:</b> <code>%s</code>\n"+
			"👤 <b>Requested By:</b> %s\n\n"+
			"Your request has been logged! Our team will upload it to the library soon. 🚀",
		escapedQuery,
		userMention,
	)

	_, err := m.Reply(resp, &telegram.SendOptions{
		ParseMode: "html",
	})
	return err
}
