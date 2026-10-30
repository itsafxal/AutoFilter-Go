package broadcast

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/amarnathcjd/gogram/telegram"
)

var (
	floodWaitRegex = regexp.MustCompile(`FLOOD(?:_PREMIUM)?_WAIT_(\d+)`)
	waitSecRegex   = regexp.MustCompile(`(?i)wait (?:of )?(\d+) seconds`)
)

// isFloodWaitError checks if an error is a Telegram FloodWait rate limit.
func isFloodWaitError(err error) bool {
	if err == nil {
		return false
	}
	if telegram.MatchError(err, "FLOOD_WAIT_") || telegram.MatchError(err, "FLOOD_PREMIUM_WAIT_") {
		return true
	}
	s := strings.ToUpper(err.Error())
	return strings.Contains(s, "FLOOD_WAIT") ||
		strings.Contains(s, "FLOOD_PREMIUM_WAIT") ||
		strings.Contains(s, "420") ||
		strings.Contains(s, "WAIT OF") ||
		strings.Contains(s, "WAIT ")
}

// extractFloodWaitSeconds extracts the required sleep duration from a Telegram FloodWait error.
func extractFloodWaitSeconds(err error) int {
	if err == nil {
		return 0
	}
	if wait := telegram.GetFloodWait(err); wait > 0 {
		return wait
	}
	errStr := err.Error()
	if matches := floodWaitRegex.FindStringSubmatch(errStr); len(matches) > 1 {
		if val, pErr := strconv.Atoi(matches[1]); pErr == nil && val > 0 {
			return val
		}
	}
	if matches := waitSecRegex.FindStringSubmatch(errStr); len(matches) > 1 {
		if val, pErr := strconv.Atoi(matches[1]); pErr == nil && val > 0 {
			return val
		}
	}
	return 1
}

// isUserBlockedError checks if delivery failed because the user blocked the bot.
func isUserBlockedError(err error) bool {
	if err == nil {
		return false
	}
	if telegram.MatchError(err, "USER_IS_BLOCKED") {
		return true
	}
	s := strings.ToUpper(err.Error())
	return strings.Contains(s, "USER_IS_BLOCKED") ||
		strings.Contains(s, "BLOCKED BY THE USER") ||
		strings.Contains(s, "BOT WAS BLOCKED")
}

// isUserDeactivatedError checks if the user account has been deleted/deactivated from Telegram.
func isUserDeactivatedError(err error) bool {
	if err == nil {
		return false
	}
	if telegram.MatchError(err, "USER_DEACTIVATED") || telegram.MatchError(err, "INPUT_USER_DEACTIVATED") {
		return true
	}
	s := strings.ToUpper(err.Error())
	return strings.Contains(s, "USER_DEACTIVATED") ||
		strings.Contains(s, "INPUT_USER_DEACTIVATED") ||
		strings.Contains(s, "USER_DEACTIVATED_BAN")
}

// isChatForbiddenError checks if the bot cannot write to a group/channel due to permissions or being removed.
func isChatForbiddenError(err error) bool {
	if err == nil {
		return false
	}
	if telegram.MatchError(err, "CHAT_WRITE_FORBIDDEN") ||
		telegram.MatchError(err, "CHAT_ADMIN_REQUIRED") ||
		telegram.MatchError(err, "PEER_ID_INVALID") ||
		telegram.MatchError(err, "CHANNEL_PRIVATE") ||
		telegram.MatchError(err, "USER_NOT_MUTUAL_CONTACT") {
		return true
	}
	s := strings.ToUpper(err.Error())
	return strings.Contains(s, "CHAT_WRITE_FORBIDDEN") ||
		strings.Contains(s, "CHAT_ADMIN_REQUIRED") ||
		strings.Contains(s, "PEER_ID_INVALID") ||
		strings.Contains(s, "CHANNEL_PRIVATE") ||
		strings.Contains(s, "USER_NOT_MUTUAL_CONTACT") ||
		strings.Contains(s, "KICKED FROM") ||
		strings.Contains(s, "NOT A MEMBER")
}

// isTransientError checks if an error is temporary (network glitch, socket timeout) and suitable for retry.
func isTransientError(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "timeout") ||
		strings.Contains(s, "connection reset") ||
		strings.Contains(s, "broken pipe") ||
		strings.Contains(s, "i/o timeout") ||
		strings.Contains(s, "temporary failure")
}
