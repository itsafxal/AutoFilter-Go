package plugins

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/amarnathcjd/gogram/telegram"
)

// Registry manages plugin discovery, validation, command conflict detection,
// and dispatcher registration.
type Registry struct {
	mu          sync.RWMutex
	appCtx      *AppContext
	plugins     []Plugin
	pluginNames map[string]struct{}
	commands    map[string]CommandDefinition // primary command name -> definition
	allTriggers map[string]string            // trigger (command or alias) -> "plugin: <plugin_name>, command: <cmd_name>"
	validated   bool
}

// NewRegistry creates a new, empty plugin registry.
func NewRegistry(appCtx *AppContext) *Registry {
	r := &Registry{
		appCtx:      appCtx,
		plugins:     make([]Plugin, 0),
		pluginNames: make(map[string]struct{}),
		commands:    make(map[string]CommandDefinition),
		allTriggers: make(map[string]string),
	}
	if appCtx != nil {
		appCtx.Registry = r
	}
	return r
}

// Register adds one or more plugins to the registry. Must be called before Validate().
func (r *Registry) Register(plugins ...Plugin) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.validated {
		return errors.New("cannot register new plugins after registry has already been validated")
	}

	for _, p := range plugins {
		if p == nil {
			return errors.New("cannot register nil plugin")
		}
		name := strings.TrimSpace(p.Name())
		if name == "" {
			return errors.New("plugin name cannot be empty")
		}
		if _, exists := r.pluginNames[name]; exists {
			return fmt.Errorf("duplicate plugin registration: plugin with name %q is already registered", name)
		}
		r.pluginNames[name] = struct{}{}
		r.plugins = append(r.plugins, p)
	}

	return nil
}

// Validate validates all registered plugins, their command names, aliases, handlers,
// and detects duplicates and conflicts.
func (r *Registry) Validate() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.validated {
		return nil
	}

	for _, p := range r.plugins {
		pName := p.Name()
		cmds := p.Commands()

		for _, cmd := range cmds {
			cmdName := strings.ToLower(strings.TrimSpace(cmd.Name))
			cmdName = strings.TrimPrefix(cmdName, "/")
			if cmdName == "" {
				return fmt.Errorf("plugin %q has a command with an empty name", pName)
			}

			if cmd.Handler == nil {
				return fmt.Errorf("plugin %q command %q has a nil handler", pName, cmdName)
			}

			if strings.TrimSpace(cmd.Description) == "" {
				return fmt.Errorf("plugin %q command %q has an empty description", pName, cmdName)
			}

			// Check if primary command name conflicts with an existing command or alias
			origin := fmt.Sprintf("plugin: %s, command: %s", pName, cmdName)
			if existingOrigin, exists := r.allTriggers[cmdName]; exists {
				return fmt.Errorf("command conflict: trigger %q in %s conflicts with existing %s",
					cmdName, origin, existingOrigin)
			}
			r.allTriggers[cmdName] = origin
			r.commands[cmdName] = cmd

			// Check aliases
			aliasSet := make(map[string]struct{})
			for _, alias := range cmd.Aliases {
				aliasName := strings.ToLower(strings.TrimSpace(alias))
				aliasName = strings.TrimPrefix(aliasName, "/")
				if aliasName == "" {
					return fmt.Errorf("plugin %q command %q has an empty alias", pName, cmdName)
				}
				if aliasName == cmdName {
					return fmt.Errorf("plugin %q command %q defines itself as an alias", pName, cmdName)
				}
				if _, duplicatedWithinCmd := aliasSet[aliasName]; duplicatedWithinCmd {
					return fmt.Errorf("plugin %q command %q defines duplicate alias %q", pName, cmdName, aliasName)
				}
				aliasSet[aliasName] = struct{}{}

				aliasOrigin := fmt.Sprintf("plugin: %s, command: %s (alias: %s)", pName, cmdName, aliasName)
				if existingOrigin, exists := r.allTriggers[aliasName]; exists {
					return fmt.Errorf("alias conflict: alias %q in %s conflicts with existing %s",
						aliasName, aliasOrigin, existingOrigin)
				}
				r.allTriggers[aliasName] = aliasOrigin
			}
		}
	}

	r.validated = true
	return nil
}

// AttachToClient registers all validated command, callback, and message handlers onto the Gogram Client.
func (r *Registry) AttachToClient(client *telegram.Client) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if !r.validated {
		return errors.New("cannot attach handlers to client before registry validation")
	}
	if client == nil {
		return errors.New("cannot attach handlers to nil client")
	}

	for _, p := range r.plugins {
		for _, cmd := range p.Commands() {
			cmdCopy := cmd
			cmdName := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(cmdCopy.Name), "/"))

			wrappedHandler := r.wrapHandler(cmdCopy)

			// Register primary command handler
			client.AddCommandHandler(cmdName, wrappedHandler)

			// Register alias handlers
			for _, alias := range cmdCopy.Aliases {
				aliasName := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(alias), "/"))
				client.AddCommandHandler(aliasName, wrappedHandler)
			}
		}

		// Register callback query handlers if the plugin implements CallbackPlugin
		if cbPlugin, ok := p.(CallbackPlugin); ok {
			for _, cb := range cbPlugin.CallbackHandlers() {
				cbCopy := cb
				wrappedCb := r.wrapCallbackHandler(cbCopy)
				prefixPattern := regexp.MustCompile("^" + regexp.QuoteMeta(cbCopy.Prefix))
				client.AddCallbackHandler(prefixPattern, wrappedCb)
			}
		}

		// Register message handlers if the plugin implements MessagePlugin
		if msgPlugin, ok := p.(MessagePlugin); ok {
			for _, mh := range msgPlugin.MessageHandlers() {
				mhCopy := mh
				wrappedMh := r.wrapMessageHandler(mhCopy)
				if mhCopy.Filter != nil {
					client.AddMessageHandler(string(telegram.OnNewMessage), wrappedMh, telegram.Any(telegram.IsIncoming, telegram.IsOutgoing), telegram.CustomFilter(mhCopy.Filter))
					if mhCopy.AllowChannel {
						client.AddMessageHandler(string(telegram.OnEditMessage), wrappedMh, telegram.Any(telegram.IsIncoming, telegram.IsOutgoing), telegram.CustomFilter(mhCopy.Filter))
					}
				} else {
					client.AddMessageHandler(string(telegram.OnNewMessage), wrappedMh, telegram.Any(telegram.IsIncoming, telegram.IsOutgoing))
					if mhCopy.AllowChannel {
						client.AddMessageHandler(string(telegram.OnEditMessage), wrappedMh, telegram.Any(telegram.IsIncoming, telegram.IsOutgoing))
					}
				}
			}
		}

		// Register join request handlers if the plugin implements JoinRequestPlugin
		if jrPlugin, ok := p.(JoinRequestPlugin); ok {
			for _, jh := range jrPlugin.JoinRequestHandlers() {
				jhCopy := jh
				client.OnJoinRequest(func(req *telegram.JoinRequestUpdate) (err error) {
					defer func() {
						if rec := recover(); rec != nil {
							err = fmt.Errorf("panic in join request handler: %v", rec)
							if r.appCtx != nil && r.appCtx.Logger != nil {
								r.appCtx.Logger.Error("panic recovered in join request handler",
									"panic", rec,
								)
							}
						}
					}()
					return jhCopy(req)
				})
			}
		}
	}

	return nil
}

// wrapMessageHandler wraps a message handler with channel filtering, panic recovery, and structured logging.
func (r *Registry) wrapMessageHandler(mh MessageDefinition) HandlerFunc {
	return func(m *telegram.NewMessage) (err error) {
		if !mh.AllowChannel && m != nil && m.Message != nil && (m.IsChannel() || m.IsChannelPost()) {
			// Never reply or process ambient messages in broadcast channels unless handler explicitly opted in
			return nil
		}
		defer func() {
			if rec := recover(); rec != nil {
				err = fmt.Errorf("panic in message handler: %v", rec)
				if r.appCtx != nil && r.appCtx.Logger != nil {
					r.appCtx.Logger.Error("panic recovered in message handler",
						"panic", rec,
					)
				}
			}
		}()
		err = mh.Handler(m)
		if err != nil && r.appCtx != nil && r.appCtx.Logger != nil {
			r.appCtx.Logger.Error("message handler error", "error", err)
		}
		return err
	}
}

// wrapCallbackHandler wraps a callback handler with panic recovery and structured logging.
func (r *Registry) wrapCallbackHandler(cb CallbackDefinition) CallbackHandlerFunc {
	return func(query *telegram.CallbackQuery) (err error) {
		if query == nil {
			return nil
		}
		defer func() {
			if rec := recover(); rec != nil {
				err = fmt.Errorf("panic in callback prefix %q: %v", cb.Prefix, rec)
				if r.appCtx != nil && r.appCtx.Logger != nil {
					r.appCtx.Logger.Error("panic recovered in callback handler",
						"prefix", cb.Prefix,
						"panic", rec,
					)
				}
			}
		}()
		return cb.Handler(query)
	}
}

// wrapHandler wraps a command handler with channel filtering, admin protection and panic recovery.
func (r *Registry) wrapHandler(cmd CommandDefinition) HandlerFunc {
	return func(m *telegram.NewMessage) (err error) {
		if m != nil && m.Message != nil && (m.IsChannel() || m.IsChannelPost()) {
			// Never reply or process commands in broadcast channels
			return nil
		}
		defer func() {
			if rec := recover(); rec != nil {
				err = fmt.Errorf("panic in command /%s: %v", cmd.Name, rec)
				if r.appCtx != nil && r.appCtx.Logger != nil {
					r.appCtx.Logger.Error("panic recovered in command handler",
						"command", cmd.Name,
						"panic", rec,
					)
				}
			}
		}()

		// Enforce MaintenanceMode: only /start works for non-admins
		if r.appCtx != nil && r.appCtx.Config != nil && r.appCtx.Config.MaintenanceMode {
			var userID int64
			if m != nil {
				if m.Sender != nil && m.Sender.ID != 0 {
					userID = m.Sender.ID
				} else if m.Message != nil {
					userID = m.SenderID()
				}
			}
			if !r.appCtx.Config.IsAdmin(userID) {
				if cmd.Name != "start" {
					if r.appCtx.Logger != nil {
						r.appCtx.Logger.Info("command blocked due to maintenance mode",
							"command", cmd.Name,
							"user_id", userID,
						)
					}
					return nil
				}
			}
		}

		// Enforce AdminOnly if configured
		if cmd.AdminOnly && r.appCtx != nil && r.appCtx.Config != nil {
			var userID int64
			if m != nil {
				if m.Sender != nil && m.Sender.ID != 0 {
					userID = m.Sender.ID
				} else if m.Message != nil {
					userID = m.SenderID()
				}
			}
			if !r.appCtx.Config.IsAdmin(userID) {
				if r.appCtx.Logger != nil {
					r.appCtx.Logger.Warn("unauthorized admin command attempt",
						"command", cmd.Name,
						"user_id", userID,
					)
				}
				if m != nil {
					_, _ = m.Reply("⛔ <b>Access Denied:</b> This command is restricted to bot administrators.", &telegram.SendOptions{
						ParseMode: "html",
					})
				}
				return nil
			}
		}

		return cmd.Handler(m)
	}
}

// GenerateHelpText dynamically builds a formatted HTML command usage guide
// reflecting exclusively the commands registered in the current build.
func (r *Registry) GenerateHelpText() string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Group commands by category
	categories := make(map[string][]CommandDefinition)
	for _, cmd := range r.commands {
		cat := strings.TrimSpace(cmd.Category)
		if cat == "" {
			cat = "General"
		}
		categories[cat] = append(categories[cat], cmd)
	}

	// Sort categories with "General" first
	catNames := make([]string, 0, len(categories))
	for name := range categories {
		catNames = append(catNames, name)
	}
	sort.Slice(catNames, func(i, j int) bool {
		if catNames[i] == "General" {
			return true
		}
		if catNames[j] == "General" {
			return false
		}
		return catNames[i] < catNames[j]
	})

	var sb strings.Builder
	sb.WriteString("<b><u>🤖 BOT COMMANDS & USAGE GUIDE 🤖</u></b>\n\n")

	for _, cat := range catNames {
		cmds := categories[cat]
		// Sort commands alphabetically within category
		sort.Slice(cmds, func(i, j int) bool {
			return cmds[i].Name < cmds[j].Name
		})

		sb.WriteString(fmt.Sprintf("<b>⚡ <u>%s Commands</u></b>\n", cat))
		for _, cmd := range cmds {
			aliasStr := ""
			if len(cmd.Aliases) > 0 {
				aliasStr = fmt.Sprintf(" (aliases: <code>/%s</code>)", strings.Join(cmd.Aliases, ", /"))
			}
			sb.WriteString(fmt.Sprintf("• <code>/%s</code>%s - %s\n", cmd.Name, aliasStr, cmd.Description))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("<i>Use any command by typing it in chat. Commands only available in this build are listed above.</i>")
	return strings.TrimSpace(sb.String())
}

// GenerateAdminHelpText dynamically builds a comprehensive HTML command usage guide
// including all administrative, media, and force-subscribe commands.
func (r *Registry) GenerateAdminHelpText() string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Group commands by category
	categories := make(map[string][]CommandDefinition)
	for _, cmd := range r.commands {
		cat := strings.TrimSpace(cmd.Category)
		if cat == "" {
			cat = "General"
		}
		categories[cat] = append(categories[cat], cmd)
	}

	// Order categories: General, Media, Admin, others
	catNames := make([]string, 0, len(categories))
	for name := range categories {
		catNames = append(catNames, name)
	}
	sort.Slice(catNames, func(i, j int) bool {
		priority := func(name string) int {
			switch name {
			case "General":
				return 1
			case "Media":
				return 2
			case "Admin":
				return 3
			default:
				return 4
			}
		}
		p1, p2 := priority(catNames[i]), priority(catNames[j])
		if p1 != p2 {
			return p1 < p2
		}
		return catNames[i] < catNames[j]
	})

	var sb strings.Builder
	sb.WriteString("<b><u>👑 BOT COMMANDS MASTER LIST (ADMIN) 👑</u></b>\n\n")

	for _, cat := range catNames {
		cmds := categories[cat]
		sort.Slice(cmds, func(i, j int) bool {
			return cmds[i].Name < cmds[j].Name
		})

		sb.WriteString(fmt.Sprintf("<b>⚡ <u>%s Commands</u></b>\n", cat))
		for _, cmd := range cmds {
			aliasStr := ""
			if len(cmd.Aliases) > 0 {
				aliasStr = fmt.Sprintf(" (aliases: <code>/%s</code>)", strings.Join(cmd.Aliases, ", /"))
			}
			adminTag := ""
			if cmd.AdminOnly {
				adminTag = " <i>[Admin]</i>"
			}
			sb.WriteString(fmt.Sprintf("• <code>/%s</code>%s%s - %s\n", cmd.Name, aliasStr, adminTag, cmd.Description))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("<i>Admin privileges verified. All bot commands including administrative and force-subscribe controls are listed above.</i>")
	return strings.TrimSpace(sb.String())
}


// PluginCount returns the number of registered plugins.
func (r *Registry) PluginCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.plugins)
}

// CommandCount returns the number of registered unique primary commands.
func (r *Registry) CommandCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.commands)
}
