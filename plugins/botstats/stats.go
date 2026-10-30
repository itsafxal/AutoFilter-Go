package botstats

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/amarnathcjd/gogram/telegram"
	"github.com/itsafxal/MYTGBOTZ/internal/plugins"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// StatusTemplate defines the bot statistics overview message format.
const StatusTemplate = "📈 𝐁𝐨𝐭 𝐒𝐭𝐚𝐭𝐢𝐬𝐭𝐢𝐜𝐬 𝐎𝐯𝐞𝐫𝐯𝐢𝐞𝐰 📊\n\n" +
	"📚 𝐓𝐨𝐭𝐚𝐥 𝐅𝐢𝐥𝐞𝐬: %d\n" +
	"👥 𝐓𝐨𝐭𝐚𝐥 𝐔𝐬𝐞𝐫𝐬: %d\n" +
	"💬 𝐓𝐨𝐭𝐚𝐥 𝐂𝐡𝐚𝐭𝐬: %d\n\n" +
	"💾 𝐃𝐚𝐭𝐚𝐛𝐚𝐬𝐞 𝐒𝐢𝐳𝐞𝐬:\n" +
	"• 𝐃𝐁𝟏: %.2f 𝐌𝐁\n" +
	"• 𝐃𝐁𝟐: %.2f 𝐌𝐁\n" +
	"• 𝐃𝐁𝟑: %.2f 𝐌𝐁\n" +
	"• 𝐒𝐐𝐋: %.2f 𝐌𝐁"

// BotStats contains quantitative database and storage metrics.
type BotStats struct {
	TotalFiles int64
	TotalUsers int64
	TotalChats int64
	DB1SizeMB  float64
	DB2SizeMB  float64
	DB3SizeMB  float64
	SQLSizeMB  float64
	DB2Blocked bool
	DB3Blocked bool
}

// Plugin implements the /stats and /status command plugin.
type Plugin struct {
	appCtx *plugins.AppContext
}

// New creates a new stats plugin instance.
func New(appCtx *plugins.AppContext) *Plugin {
	return &Plugin{appCtx: appCtx}
}

// Name returns the unique plugin name.
func (p *Plugin) Name() string {
	return "stats"
}

// Commands returns the command definitions for the stats plugin.
func (p *Plugin) Commands() []plugins.CommandDefinition {
	return []plugins.CommandDefinition{
		{
			Name:        "stats",
			Aliases:     []string{"status"},
			Description: "View bot database and media storage statistics",
			Usage:       "/stats",
			Category:    "General",
			Handler:     p.HandleStats,
		},
		{
			Name:        "vacuum",
			Description: "Reclaim and compact SQLite database and WAL files",
			Usage:       "/vacuum",
			Category:    "Admin",
			AdminOnly:   true,
			Handler:     p.HandleVacuum,
		},
	}
}

// CallbackHandlers returns the interactive callback handlers for stats navigation.
func (p *Plugin) CallbackHandlers() []plugins.CallbackDefinition {
	return []plugins.CallbackDefinition{
		{Prefix: "stats_refresh", Handler: p.HandleRefreshCallback},
		{Prefix: "stats_vacuum", Handler: p.HandleVacuumCallback},
		{Prefix: "close_data", Handler: p.HandleCloseCallback},
	}
}

// FormatStatsMessage formats the bot statistics overview matching STATUS_TXT template.
func FormatStatsMessage(stats BotStats) string {
	db2Extra := ""
	if stats.DB2Blocked || stats.DB2SizeMB >= 510.0 {
		db2Extra = " ⚠️ [FULL]"
	}
	db3Extra := ""
	if stats.DB3Blocked || stats.DB3SizeMB >= 510.0 {
		db3Extra = " ⚠️ [FULL]"
	}

	return fmt.Sprintf(
		"📈 𝐁𝐨𝐭 𝐒𝐭𝐚𝐭𝐢𝐬𝐭𝐢𝐜𝐬 𝐎𝐯𝐞𝐫𝐯𝐢𝐞𝐰 📊\n\n"+
			"📚 𝐓𝐨𝐭𝐚𝐥 𝐅𝐢𝐥𝐞𝐬: %d\n"+
			"👥 𝐓𝐨𝐭𝐚𝐥 𝐔𝐬𝐞𝐫𝐬: %d\n"+
			"💬 𝐓𝐨𝐭𝐚𝐥 𝐂𝐡𝐚𝐭𝐬: %d\n\n"+
			"💾 𝐃𝐚𝐭𝐚𝐛𝐚𝐬𝐞 𝐒𝐢𝐳𝐞𝐬:\n"+
			"• 𝐃𝐁𝟏: %.2f 𝐌𝐁\n"+
			"• 𝐃𝐁𝟐: %.2f 𝐌𝐁%s\n"+
			"• 𝐃𝐁𝟑: %.2f 𝐌𝐁%s\n"+
			"• 𝐒𝐐𝐋: %.2f 𝐌𝐁",
		stats.TotalFiles,
		stats.TotalUsers,
		stats.TotalChats,
		stats.DB1SizeMB,
		stats.DB2SizeMB,
		db2Extra,
		stats.DB3SizeMB,
		db3Extra,
		stats.SQLSizeMB,
	)
}

func (p *Plugin) gatherStats(ctx context.Context) BotStats {
	var stats BotStats
	if p.appCtx == nil || p.appCtx.DB == nil {
		return stats
	}

	// 1. Total Users
	if uRepo := p.appCtx.DB.UserRepo(); uRepo != nil {
		if cnt, err := uRepo.TotalUsersCount(ctx); err == nil {
			stats.TotalUsers = cnt
		}
	}

	// 2. Total Chats (Groups)
	if gRepo := p.appCtx.DB.GroupRepo(); gRepo != nil {
		if cnt, err := gRepo.TotalGroupsCount(ctx); err == nil {
			stats.TotalChats = cnt
		}
	}

	// 3. Total Files (from SQLite and MongoDB Media Shards)
	if sqlRepo := p.appCtx.DB.SqlMediaRepo(); sqlRepo != nil {
		if cnt, err := sqlRepo.Count(ctx); err == nil && cnt > stats.TotalFiles {
			stats.TotalFiles = cnt
		}
	}
	if mongoRepo := p.appCtx.DB.MongoMediaRepo(); mongoRepo != nil {
		if cnt, err := mongoRepo.TotalMediaCount(ctx); err == nil && cnt > stats.TotalFiles {
			stats.TotalFiles = cnt
		}
	}

	// 4. DB1 Size (Primary MongoDB)
	if primaryDB := p.appCtx.DB.MongoPrimaryDB(); primaryDB != nil {
		stats.DB1SizeMB = getMongoDatabaseSizeMB(ctx, primaryDB)
	}

	// 5. DB2 & DB3 Sizes (from configured MongoDB media shards)
	if mongoRepo := p.appCtx.DB.MongoMediaRepo(); mongoRepo != nil {
		stats.DB2Blocked = mongoRepo.IsShardBlocked("DB2")
		stats.DB3Blocked = mongoRepo.IsShardBlocked("DB3")
		for _, shard := range mongoRepo.Shards() {
			if shard.Col != nil && shard.Col.Database() != nil {
				if shard.Label == "DB2" && stats.DB2SizeMB == 0 {
					stats.DB2SizeMB = getMongoDatabaseSizeMB(ctx, shard.Col.Database())
				} else if shard.Label == "DB3" && stats.DB3SizeMB == 0 {
					stats.DB3SizeMB = getMongoDatabaseSizeMB(ctx, shard.Col.Database())
				}
			}
		}
	}

	// 6. SQLite Size (Total physical space on disk: .sqlite3 + .sqlite3-wal + .sqlite3-shm)
	sqlitePath := "media.sqlite3"
	if p.appCtx.Config != nil && p.appCtx.Config.SQLiteDBPath != "" {
		sqlitePath = p.appCtx.Config.SQLiteDBPath
	}
	var totalSQLBytes int64
	if fi, err := os.Stat(sqlitePath); err == nil && fi.Size() > 0 {
		totalSQLBytes += fi.Size()
	}
	if fi, err := os.Stat(sqlitePath + "-wal"); err == nil && fi.Size() > 0 {
		totalSQLBytes += fi.Size()
	}
	if fi, err := os.Stat(sqlitePath + "-shm"); err == nil && fi.Size() > 0 {
		totalSQLBytes += fi.Size()
	}
	if totalSQLBytes > 0 {
		stats.SQLSizeMB = float64(totalSQLBytes) / (1024.0 * 1024.0)
	}

	return stats
}

func getMongoDatabaseSizeMB(ctx context.Context, db *mongo.Database) float64 {
	if db == nil {
		return 0.0
	}
	cmd := "db" + "Stats"
	res := db.RunCommand(ctx, bson.D{{Key: cmd, Value: 1}})
	if res.Err() != nil {
		return 0.0
	}
	var raw bson.M
	if err := res.Decode(&raw); err != nil {
		return 0.0
	}
	var bytes float64
	if v, ok := raw["dataSize"]; ok {
		bytes = toFloat64(v)
	} else if v, ok := raw["storageSize"]; ok {
		bytes = toFloat64(v)
	} else if v, ok := raw["totalSize"]; ok {
		bytes = toFloat64(v)
	}
	return bytes / (1024.0 * 1024.0)
}

func toFloat64(val any) float64 {
	switch v := val.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int64:
		return float64(v)
	case int32:
		return float64(v)
	case int:
		return float64(v)
	default:
		return 0.0
	}
}

func (p *Plugin) buildStatsContent(ctx context.Context) (string, *telegram.ReplyInlineMarkup) {
	stats := p.gatherStats(ctx)
	text := FormatStatsMessage(stats)

	replyMarkup := telegram.NewKeyboard().
		AddRow(
			telegram.Button.Data("🔄 Rᴇғʀᴇsʜ", "stats_refresh"),
			telegram.Button.Data("🧹 Vᴀᴄᴜᴜᴍ DB", "stats_vacuum"),
		).
		AddRow(
			telegram.Button.Data("❌ Cʟᴏsᴇ", "close_data").Danger(),
		).
		Build()

	return text, replyMarkup
}

// HandleVacuum compacts and reclaims physical disk space from SQLite database and WAL files.
func (p *Plugin) HandleVacuum(m *telegram.NewMessage) error {
	if m.IsChannel() || m.IsChannelPost() {
		return nil
	}
	var userID int64
	if m.Sender != nil && m.Sender.ID != 0 {
		userID = m.Sender.ID
	} else {
		userID = m.SenderID()
	}
	isAdmin := p.appCtx != nil && p.appCtx.Config != nil && p.appCtx.Config.IsAdmin(userID)
	if !isAdmin {
		return nil
	}

	reply, _ := m.Reply("⏳ <b>Vacuuming SQLite Database...</b>\n<i>Truncating WAL, compacting FTS5, and reclaiming disk space...</i>", &telegram.SendOptions{
		ParseMode: "html",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	if p.appCtx.DB != nil && p.appCtx.DB.SQLManager() != nil {
		_ = p.appCtx.DB.SQLManager().CheckpointWAL(ctx)
		_ = p.appCtx.DB.SQLManager().OptimizeFTS(ctx)
		_ = p.appCtx.DB.SQLManager().IncrementalVacuum(ctx)
	}

	text, replyMarkup := p.buildStatsContent(ctx)
	msgText := fmt.Sprintf("✅ <b>Database Compacted Successfully!</b>\n\n%s", text)
	if reply != nil {
		_, err := reply.Edit(msgText, &telegram.SendOptions{
			ParseMode:   "html",
			ReplyMarkup: replyMarkup,
		})
		return err
	}
	_, err := m.Reply(msgText, &telegram.SendOptions{
		ParseMode:   "html",
		ReplyMarkup: replyMarkup,
	})
	return err
}

// HandleVacuumCallback handles the interactive vacuum button click on /stats.
func (p *Plugin) HandleVacuumCallback(cb *telegram.CallbackQuery) error {
	if cb == nil {
		return nil
	}
	var userID int64
	if cb.Sender != nil && cb.Sender.ID != 0 {
		userID = cb.Sender.ID
	} else {
		userID = cb.SenderID
	}
	isAdmin := p.appCtx != nil && p.appCtx.Config != nil && p.appCtx.Config.IsAdmin(userID)
	if !isAdmin {
		_, err := cb.Answer("⛔ Only bot administrators can vacuum the database.", &telegram.CallbackOptions{Alert: true})
		return err
	}

	_, _ = cb.Answer("🧹 Compacting database & reclaiming disk space...", &telegram.CallbackOptions{Alert: false})

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	if p.appCtx.DB != nil && p.appCtx.DB.SQLManager() != nil {
		_ = p.appCtx.DB.SQLManager().CheckpointWAL(ctx)
		_ = p.appCtx.DB.SQLManager().OptimizeFTS(ctx)
		_ = p.appCtx.DB.SQLManager().IncrementalVacuum(ctx)
	}

	text, replyMarkup := p.buildStatsContent(ctx)
	_, _ = cb.Edit(text, &telegram.SendOptions{
		ParseMode:   "html",
		ReplyMarkup: replyMarkup,
	})
	return nil
}

// HandleStats renders the bot statistics overview.
func (p *Plugin) HandleStats(m *telegram.NewMessage) error {
	if m.IsChannel() || m.IsChannelPost() {
		return nil
	}

	if p.isMaintenanceMode() {
		var userID int64
		if m.Sender != nil && m.Sender.ID != 0 {
			userID = m.Sender.ID
		} else {
			userID = m.SenderID()
		}
		isAdmin := p.appCtx != nil && p.appCtx.Config != nil && p.appCtx.Config.IsAdmin(userID)
		if !isAdmin {
			return nil
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	text, replyMarkup := p.buildStatsContent(ctx)

	_, err := m.Reply(text, &telegram.SendOptions{
		ParseMode:   "html",
		ReplyMarkup: replyMarkup,
	})
	return err
}

// HandleRefreshCallback handles the interactive refresh button.
func (p *Plugin) HandleRefreshCallback(cb *telegram.CallbackQuery) error {
	if cb == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	text, replyMarkup := p.buildStatsContent(ctx)

	_, _ = cb.Edit(text, &telegram.SendOptions{
		ParseMode:   "html",
		ReplyMarkup: replyMarkup,
	})
	_, err := cb.Answer("Stats updated! 🚀")
	return err
}

// HandleCloseCallback closes the stats message.
func (p *Plugin) HandleCloseCallback(cb *telegram.CallbackQuery) error {
	if cb != nil {
		_, _ = cb.Delete()
		_, _ = cb.Answer("")
	}
	return nil
}

func (p *Plugin) isMaintenanceMode() bool {
	return p.appCtx != nil && p.appCtx.Config != nil && p.appCtx.Config.MaintenanceMode
}
