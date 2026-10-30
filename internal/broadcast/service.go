package broadcast

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/amarnathcjd/gogram/telegram"
	"github.com/itsafxal/AutoFilter-Go/internal/database"
)

// recipientItem is a decoded recipient ID from database cursors.
type recipientItem struct {
	ID int64 `bson:"id"`
}

// floodCoordinator synchronizes rate limits across all concurrent workers.
type floodCoordinator struct {
	mu         sync.RWMutex
	pauseUntil time.Time
	isPaused   bool
	floodSec   int
}

func (fc *floodCoordinator) WaitIfNeeded(ctx context.Context) error {
	fc.mu.RLock()
	until := fc.pauseUntil
	fc.mu.RUnlock()

	now := time.Now()
	if until.After(now) {
		select {
		case <-time.After(until.Sub(now)):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func (fc *floodCoordinator) TriggerPause(sec int) {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	if sec <= 0 {
		sec = 1
	}
	target := time.Now().Add(time.Duration(sec+1) * time.Second)
	if target.After(fc.pauseUntil) {
		fc.pauseUntil = target
		fc.isPaused = true
		fc.floodSec = sec
	}
}

func (fc *floodCoordinator) ClearPauseIfExpired() {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	if fc.isPaused && time.Now().After(fc.pauseUntil) {
		fc.isPaused = false
		fc.floodSec = 0
	}
}

func (fc *floodCoordinator) Status() (bool, int) {
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	if fc.isPaused && time.Now().Before(fc.pauseUntil) {
		rem := int(time.Until(fc.pauseUntil).Seconds())
		if rem < 1 {
			rem = 1
		}
		return true, rem
	}
	return false, 0
}

// activeStatsContainer tracks atomic broadcast progress metrics.
type activeStatsContainer struct {
	total       int64
	processed   int64
	successful  int64
	failed      int64
	blocked     int64
	deactivated int64
	startTime   time.Time
	endTime     time.Time
	status      Status
	target      TargetType
	mode        Mode
	floodCoord  *floodCoordinator
}

// BroadcastEngine implements the Service interface for fast concurrent message delivery.
type BroadcastEngine struct {
	client             *telegram.Client
	db                 *database.DatabaseManager
	logger             *slog.Logger
	defaultConcurrency int

	mu             sync.Mutex
	isBroadcasting int32 // atomic: 0 = idle, 1 = running
	activeCancel   context.CancelFunc
	activeStats    *activeStatsContainer
}

// New creates a new BroadcastEngine instance.
func New(client *telegram.Client, db *database.DatabaseManager, logger *slog.Logger, defaultConcurrency int) *BroadcastEngine {
	if defaultConcurrency <= 0 {
		defaultConcurrency = 10
	}
	if defaultConcurrency > 50 {
		defaultConcurrency = 50
	}
	if logger == nil {
		logger = slog.Default()
	}

	return &BroadcastEngine{
		client:             client,
		db:                 db,
		logger:             logger,
		defaultConcurrency: defaultConcurrency,
	}
}

// IsBroadcasting returns true if a broadcast operation is currently active.
func (e *BroadcastEngine) IsBroadcasting() bool {
	return atomic.LoadInt32(&e.isBroadcasting) == 1
}

// CancelBroadcast stops any currently running broadcast job.
func (e *BroadcastEngine) CancelBroadcast() bool {
	e.mu.Lock()
	defer e.mu.Unlock()

	if atomic.LoadInt32(&e.isBroadcasting) == 0 || e.activeCancel == nil {
		return false
	}

	e.activeCancel()
	if e.logger != nil {
		e.logger.Info("broadcast cancellation requested by administrator")
	}
	return true
}

// GetStats returns a snapshot of the active or most recent broadcast statistics.
func (e *BroadcastEngine) GetStats() (Stats, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.activeStats == nil {
		return Stats{}, false
	}

	s := e.activeStats
	proc := atomic.LoadInt64(&s.processed)
	succ := atomic.LoadInt64(&s.successful)
	fail := atomic.LoadInt64(&s.failed)
	bloc := atomic.LoadInt64(&s.blocked)
	deac := atomic.LoadInt64(&s.deactivated)

	isFlood, floodSec := s.floodCoord.Status()

	elapsed := time.Since(s.startTime).Seconds()
	var speed float64
	if elapsed > 0 {
		speed = float64(proc) / elapsed
	}

	return Stats{
		TotalRecipients: s.total,
		Processed:       proc,
		Successful:      succ,
		Failed:          fail,
		Blocked:         bloc,
		Deactivated:     deac,
		StartTime:       s.startTime,
		EndTime:         s.endTime,
		Status:          s.status,
		IsFloodWait:     isFlood,
		FloodSec:        floodSec,
		Speed:           speed,
		Target:          s.target,
		Mode:            s.mode,
	}, true
}

// StartBroadcast initiates an asynchronous, bounded-concurrency broadcast to users/groups.
func (e *BroadcastEngine) StartBroadcast(
	ctx context.Context,
	sourceMsg *telegram.NewMessage,
	progressMsg *telegram.NewMessage,
	opts Options,
) error {
	if sourceMsg == nil {
		return errors.New("source message cannot be nil")
	}
	if progressMsg == nil {
		return errors.New("progress message cannot be nil")
	}

	// 1. Guard against concurrent broadcasts
	if !atomic.CompareAndSwapInt32(&e.isBroadcasting, 0, 1) {
		return errors.New("another broadcast is currently in progress")
	}

	// 2. Normalize options
	if opts.Target == "" {
		opts.Target = TargetUsers
	}
	if opts.Mode == "" {
		opts.Mode = ModeCopy
	}
	if opts.Concurrency <= 0 {
		opts.Concurrency = e.defaultConcurrency
	}
	if opts.Concurrency > 50 {
		opts.Concurrency = 50
	}
	if opts.Skip < 0 {
		opts.Skip = 0
	}

	// 3. Calculate total recipients from MongoDB
	totalRecipients, err := e.countRecipients(ctx, opts.Target)
	if err != nil {
		atomic.StoreInt32(&e.isBroadcasting, 0)
		return fmt.Errorf("failed to count recipients: %w", err)
	}

	// Adjust total for skip
	netTotal := totalRecipients - opts.Skip
	if netTotal <= 0 {
		atomic.StoreInt32(&e.isBroadcasting, 0)
		return fmt.Errorf("no recipients remaining after skipping %d (total available: %d)", opts.Skip, totalRecipients)
	}

	// 4. Initialize active stats and context
	bCtx, cancel := context.WithCancel(context.Background())
	e.mu.Lock()
	e.activeCancel = cancel
	floodCoord := &floodCoordinator{}
	e.activeStats = &activeStatsContainer{
		total:      netTotal,
		startTime:  time.Now(),
		status:     StatusRunning,
		target:     opts.Target,
		mode:       opts.Mode,
		floodCoord: floodCoord,
	}
	e.mu.Unlock()

	// 5. Launch asynchronous broadcast pipeline
	go e.runBroadcast(bCtx, sourceMsg, progressMsg, opts, netTotal, floodCoord)

	return nil
}

// countRecipients returns total active recipients based on target type.
func (e *BroadcastEngine) countRecipients(ctx context.Context, target TargetType) (int64, error) {
	if e.db == nil {
		return 0, errors.New("database manager is nil")
	}

	var count int64
	switch target {
	case TargetUsers:
		if uRepo := e.db.UserRepo(); uRepo != nil {
			c, err := uRepo.ActiveUsersCount(ctx)
			if err != nil {
				return 0, err
			}
			count = c
		}
	case TargetGroups:
		if gRepo := e.db.GroupRepo(); gRepo != nil {
			c, err := gRepo.ActiveGroupsCount(ctx)
			if err != nil {
				return 0, err
			}
			count = c
		}
	case TargetAll:
		if uRepo := e.db.UserRepo(); uRepo != nil {
			uc, err := uRepo.ActiveUsersCount(ctx)
			if err != nil {
				return 0, err
			}
			count += uc
		}
		if gRepo := e.db.GroupRepo(); gRepo != nil {
			gc, err := gRepo.ActiveGroupsCount(ctx)
			if err != nil {
				return 0, err
			}
			count += gc
		}
	}
	return count, nil
}

// runBroadcast executes the bounded concurrency broadcast pipeline.
func (e *BroadcastEngine) runBroadcast(
	ctx context.Context,
	sourceMsg *telegram.NewMessage,
	progressMsg *telegram.NewMessage,
	opts Options,
	totalRecipients int64,
	floodCoord *floodCoordinator,
) {
	defer func() {
		e.mu.Lock()
		if e.activeStats != nil {
			e.activeStats.endTime = time.Now()
			if ctx.Err() != nil {
				e.activeStats.status = StatusCancelled
			} else {
				e.activeStats.status = StatusCompleted
			}
		}
		e.activeCancel = nil
		e.mu.Unlock()
		atomic.StoreInt32(&e.isBroadcasting, 0)
	}()

	startTime := time.Now()
	var (
		processed   int64
		successful  int64
		failed      int64
		blocked     int64
		deactivated int64
	)

	// Cancellation button keyboard
	cancelKb := telegram.NewKeyboard().
		AddRow(telegram.Button.Data("🛑 Cancel Broadcast", "cancel_broadcast")).
		Build()

	// 1. Start periodic progress updater (throttled every 3.5s to prevent edit rate limits)
	stopUpdater := make(chan struct{})
	updaterDone := make(chan struct{})
	go func() {
		defer close(updaterDone)
		ticker := time.NewTicker(3500 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-stopUpdater:
				return
			case <-ticker.C:
				floodCoord.ClearPauseIfExpired()
				isFlood, floodSec := floodCoord.Status()

				curProc := atomic.LoadInt64(&processed)
				curSucc := atomic.LoadInt64(&successful)
				curFail := atomic.LoadInt64(&failed)
				curBloc := atomic.LoadInt64(&blocked)
				curDeac := atomic.LoadInt64(&deactivated)

				text := formatProgressMessage(
					opts, totalRecipients, curProc, curSucc, curFail, curBloc, curDeac,
					startTime, isFlood, floodSec,
				)
				safeEditMessage(progressMsg, text, cancelKb)
			}
		}
	}()

	// 2. Setup worker pool & recipient channel
	concurrency := opts.Concurrency
	recipientCh := make(chan int64, concurrency*2)
	var workerWg sync.WaitGroup

	asCopy := (opts.Mode == ModeCopy)

	// Spawn N workers
	for w := 0; w < concurrency; w++ {
		workerWg.Add(1)
		go func(workerID int) {
			defer workerWg.Done()

			for recipientID := range recipientCh {
				if ctx.Err() != nil {
					return
				}

				// Wait if global flood control is active
				if err := floodCoord.WaitIfNeeded(ctx); err != nil {
					return
				}

				// Execute message delivery with retry on FloodWait/transient error
				delivered := false
				retries := 0

				for retries < 3 && !delivered && ctx.Err() == nil {
					_, err := sourceMsg.ForwardTo(recipientID, &telegram.ForwardOptions{
						HideAuthor: asCopy,
					})

					if err == nil {
						delivered = true
						atomic.AddInt64(&successful, 1)
						break
					}

					// 1. FloodWait Handling
					if isFloodWaitError(err) {
						waitSec := extractFloodWaitSeconds(err)
						if e.logger != nil {
							e.logger.Warn("telegram rate limit encountered during broadcast",
								"worker", workerID,
								"recipient", recipientID,
								"wait_seconds", waitSec,
							)
						}
						floodCoord.TriggerPause(waitSec)
						select {
						case <-time.After(time.Duration(waitSec+1) * time.Second):
						case <-ctx.Done():
							return
						}
						retries++
						continue
					}

					// 2. User Blocked
					if isUserBlockedError(err) {
						atomic.AddInt64(&blocked, 1)
						break
					}

					// 3. Account Deactivated
					if isUserDeactivatedError(err) {
						atomic.AddInt64(&deactivated, 1)
						if e.db != nil && e.db.UserRepo() != nil {
							go func(id int64) {
								_ = e.db.UserRepo().DeleteUser(context.Background(), id)
							}(recipientID)
						}
						break
					}

					// 4. Chat Forbidden / Invalid Peer
					if isChatForbiddenError(err) {
						atomic.AddInt64(&failed, 1)
						if e.db != nil && e.db.GroupRepo() != nil && recipientID < 0 {
							go func(id int64) {
								_ = e.db.GroupRepo().DeleteGroup(context.Background(), id)
							}(recipientID)
						}
						break
					}

					// 5. Transient Error
					if isTransientError(err) && retries < 2 {
						retries++
						time.Sleep(time.Duration(retries*500) * time.Millisecond)
						continue
					}

					// 6. Generic Failure
					atomic.AddInt64(&failed, 1)
					break
				}

				atomic.AddInt64(&processed, 1)
			}
		}(w)
	}

	// 3. Producer: stream recipient IDs from MongoDB cursor without loading into RAM
	var seen sync.Map
	skipCount := opts.Skip

	// Stream Users
	if opts.Target == TargetUsers || opts.Target == TargetAll {
		if uRepo := e.db.UserRepo(); uRepo != nil && ctx.Err() == nil {
			var uSkip int64
			if opts.Target == TargetUsers {
				uSkip = skipCount
			}
			cursor, cErr := uRepo.GetActiveUsersCursor(ctx, uSkip)
			if cErr == nil && cursor != nil {
				defer cursor.Close(ctx)
				for cursor.Next(ctx) {
					if ctx.Err() != nil {
						break
					}
					var item recipientItem
					if decErr := cursor.Decode(&item); decErr == nil && item.ID != 0 {
						if _, loaded := seen.LoadOrStore(item.ID, struct{}{}); !loaded {
							recipientCh <- item.ID
						}
					}
				}
			}
		}
	}

	// Stream Groups
	if opts.Target == TargetGroups || opts.Target == TargetAll {
		if gRepo := e.db.GroupRepo(); gRepo != nil && ctx.Err() == nil {
			var gSkip int64
			if opts.Target == TargetGroups {
				gSkip = skipCount
			}
			cursor, cErr := gRepo.GetActiveGroupsCursor(ctx, gSkip)
			if cErr == nil && cursor != nil {
				defer cursor.Close(ctx)
				for cursor.Next(ctx) {
					if ctx.Err() != nil {
						break
					}
					var item recipientItem
					if decErr := cursor.Decode(&item); decErr == nil && item.ID != 0 {
						if _, loaded := seen.LoadOrStore(item.ID, struct{}{}); !loaded {
							recipientCh <- item.ID
						}
					}
				}
			}
		}
	}

	// Close recipient channel & wait for workers
	close(recipientCh)
	workerWg.Wait()

	// Stop progress updater
	close(stopUpdater)
	<-updaterDone

	// Final Report
	finalProc := atomic.LoadInt64(&processed)
	finalSucc := atomic.LoadInt64(&successful)
	finalFail := atomic.LoadInt64(&failed)
	finalBloc := atomic.LoadInt64(&blocked)
	finalDeac := atomic.LoadInt64(&deactivated)
	elapsed := time.Since(startTime)

	var finalText string
	if ctx.Err() != nil {
		finalText = formatCancelledMessage(opts, totalRecipients, finalProc, finalSucc, finalFail, finalBloc, finalDeac, elapsed)
	} else {
		finalText = formatCompletedMessage(opts, totalRecipients, finalSucc, finalFail, finalBloc, finalDeac, elapsed)
	}

	// Remove inline keyboard on completion
	safeEditMessage(progressMsg, finalText, nil)
}

// formatProgressMessage constructs the live progress notification.
func formatProgressMessage(
	opts Options,
	total, processed, successful, failed, blocked, deactivated int64,
	startTime time.Time,
	isFlood bool, floodSec int,
) string {
	elapsed := time.Since(startTime)
	var speed float64
	if elapsed.Seconds() > 0 {
		speed = float64(processed) / elapsed.Seconds()
	}

	remaining := total - processed
	if remaining < 0 {
		remaining = 0
	}

	var etaStr string
	if speed > 0 && remaining > 0 {
		eta := time.Duration(float64(remaining)/speed) * time.Second
		etaStr = formatDuration(eta)
	} else {
		etaStr = "calculating..."
	}

	var pct float64
	if total > 0 {
		pct = (float64(processed) / float64(total)) * 100
		if pct > 100 {
			pct = 100
		}
	}

	floodNotice := ""
	if isFlood && floodSec > 0 {
		floodNotice = fmt.Sprintf("\n⚠️ <b>Telegram Rate Limit:</b> Paused for <code>%ds</code> before auto-resuming...\n", floodSec)
	}

	modeStr := "Forward as Copy"
	if opts.Mode == ModeForward {
		modeStr = "Forward with Tag"
	}

	return fmt.Sprintf(
		"📢 <b>Fast Broadcast in Progress...</b>\n\n"+
			"🎯 <b>Target:</b> <code>%s</code>\n"+
			"🔄 <b>Mode:</b> <code>%s</code>\n"+
			"📊 <b>Progress:</b> <code>[%s]</code> <b>%.1f%%</b>\n\n"+
			"👥 <b>Total Recipients:</b> <code>%s</code>\n"+
			"✅ <b>Delivered:</b> <code>%s</code>\n"+
			"❌ <b>Failed:</b> <code>%s</code>\n"+
			"🚫 <b>Blocked:</b> <code>%s</code>\n"+
			"💀 <b>Deactivated:</b> <code>%s</code>\n"+
			"⏳ <b>Remaining:</b> <code>%s</code>\n\n"+
			"%s"+
			"⚡ <b>Delivery Speed:</b> <code>%.1f msgs/sec</code>\n"+
			"⏱️ <b>Elapsed:</b> <code>%s</code> | <b>ETA:</b> <code>%s</code>",
		formatTarget(opts.Target),
		modeStr,
		renderProgressBar(processed, total, 10),
		pct,
		formatNumber(total),
		formatNumber(successful),
		formatNumber(failed),
		formatNumber(blocked),
		formatNumber(deactivated),
		formatNumber(remaining),
		floodNotice,
		speed,
		formatDuration(elapsed),
		etaStr,
	)
}

// formatCompletedMessage constructs the final broadcast report.
func formatCompletedMessage(
	opts Options,
	total, successful, failed, blocked, deactivated int64,
	elapsed time.Duration,
) string {
	var speed float64
	if elapsed.Seconds() > 0 {
		speed = float64(successful+failed+blocked+deactivated) / elapsed.Seconds()
	}

	modeStr := "Forward as Copy"
	if opts.Mode == ModeForward {
		modeStr = "Forward with Tag"
	}

	return fmt.Sprintf(
		"🎉 <b>Broadcast Completed Successfully!</b>\n\n"+
			"🎯 <b>Target:</b> <code>%s</code>\n"+
			"🔄 <b>Mode:</b> <code>%s</code>\n"+
			"⏱️ <b>Total Time:</b> <code>%s</code>\n"+
			"⚡ <b>Average Speed:</b> <code>%.1f msgs/sec</code>\n\n"+
			"👥 <b>Total Recipients:</b> <code>%s</code>\n"+
			"✅ <b>Successfully Delivered:</b> <code>%s</code>\n"+
			"❌ <b>Failed Deliveries:</b> <code>%s</code>\n"+
			"🚫 <b>Blocked Users:</b> <code>%s</code>\n"+
			"💀 <b>Deactivated Accounts:</b> <code>%s</code>",
		formatTarget(opts.Target),
		modeStr,
		formatDuration(elapsed),
		speed,
		formatNumber(total),
		formatNumber(successful),
		formatNumber(failed),
		formatNumber(blocked),
		formatNumber(deactivated),
	)
}

// formatCancelledMessage constructs the report when an admin cancels.
func formatCancelledMessage(
	opts Options,
	total, processed, successful, failed, blocked, deactivated int64,
	elapsed time.Duration,
) string {
	return fmt.Sprintf(
		"🛑 <b>Broadcast Cancelled by Administrator!</b>\n\n"+
			"🎯 <b>Target:</b> <code>%s</code>\n"+
			"⏱️ <b>Time Elapsed:</b> <code>%s</code>\n"+
			"👥 <b>Processed Before Stop:</b> <code>%s / %s</code>\n\n"+
			"✅ <b>Delivered:</b> <code>%s</code>\n"+
			"❌ <b>Failed:</b> <code>%s</code>\n"+
			"🚫 <b>Blocked:</b> <code>%s</code>\n"+
			"💀 <b>Deactivated:</b> <code>%s</code>",
		formatTarget(opts.Target),
		formatDuration(elapsed),
		formatNumber(processed),
		formatNumber(total),
		formatNumber(successful),
		formatNumber(failed),
		formatNumber(blocked),
		formatNumber(deactivated),
	)
}

func renderProgressBar(current, total int64, width int) string {
	if total <= 0 {
		return strings.Repeat("░", width)
	}
	filled := int((current * int64(width)) / total)
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}

func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second

	if h > 0 {
		return fmt.Sprintf("%02dh %02dm %02ds", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%02dm %02ds", m, s)
	}
	return fmt.Sprintf("%02ds", s)
}

func formatNumber(n int64) string {
	in := fmt.Sprintf("%d", n)
	if len(in) <= 3 {
		return in
	}
	var out []byte
	rem := len(in) % 3
	if rem > 0 {
		out = append(out, in[:rem]...)
		if len(in) > rem {
			out = append(out, ',')
		}
	}
	for i := rem; i < len(in); i += 3 {
		out = append(out, in[i:i+3]...)
		if i+3 < len(in) {
			out = append(out, ',')
		}
	}
	return string(out)
}

func formatTarget(t TargetType) string {
	switch t {
	case TargetGroups:
		return "Groups Only"
	case TargetAll:
		return "Users & Groups (All)"
	default:
		return "Users Only"
	}
}

func safeEditMessage(msg *telegram.NewMessage, text string, kb telegram.ReplyMarkup) {
	if msg == nil {
		return
	}
	opts := &telegram.SendOptions{
		ParseMode: "html",
	}
	if kb != nil {
		opts.ReplyMarkup = kb
	}
	_, _ = msg.Edit(text, opts)
}
