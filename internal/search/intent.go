package search

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// QueryIntent represents the structured intent extracted from a raw user query.
type QueryIntent struct {
	RawQuery            string
	CleanTitle          string
	Language            string // canonical code: "malayalam", "tamil", "telugu", "hindi", etc.
	Quality             string // canonical quality: "1080p", "720p", "4k", etc.
	Year                int
	Season              int
	Episode             int
	HasExplicitLanguage bool
	HasExplicitQuality  bool
}

var languageAliases = map[string]string{
	"malayalam":      "malayalam",
	"malay":          "malayalam",
	"mal":            "malayalam",
	"മലയാളം":         "malayalam",
	"tamil":          "tamil",
	"tam":            "tamil",
	"தமிழ்":          "tamil",
	"telugu":         "telugu",
	"tel":            "telugu",
	"తెలుగు":         "telugu",
	"hindi":          "hindi",
	"hin":            "hindi",
	"हिन्दी":         "hindi",
	"kannada":        "kannada",
	"kan":            "kannada",
	"ಕನ್ನಡ":          "kannada",
	"english":        "english",
	"eng":            "english",
	"bengali":        "bengali",
	"ben":            "bengali",
	"বাংলা":          "bengali",
	"marathi":        "marathi",
	"mar":            "marathi",
	"मराठी":          "marathi",
	"punjabi":        "punjabi",
	"pun":            "punjabi",
	"ਪੰਜਾਬੀ":         "punjabi",
	"gujarati":       "gujarati",
	"guj":            "gujarati",
	"ગુજરાતી":        "gujarati",
	"multi":          "multi",
	"dual":           "multi",
	"multi audio":    "multi",
	"dual audio":     "multi",
	"multiaudio":     "multi",
	"dualaudio":      "multi",
	"original audio": "original",
	"original":       "original",
}

var (
	reIntentQuality  = regexp.MustCompile(`(?i)\b(2160p|4k|1080p|720p|480p|360p|576p)\b`)
	reIntentYear     = regexp.MustCompile(`(?i)\b(19\d{2}|20\d{2})\b`)
	reIntentSeasonEp = regexp.MustCompile(`(?i)\b(?:s|season)[\s._-]*(\d{1,2})[\s._-]*(?:e|ep|episode)[\s._-]*(\d{1,2})\b`)
	reIntentSeason   = regexp.MustCompile(`(?i)\b(?:s|season)[\s._-]*(\d{1,2})\b`)
	reIntentEpisode  = regexp.MustCompile(`(?i)\b(?:e|ep|episode)[\s._-]*(\d{1,2})\b`)
	reIntentDelims   = regexp.MustCompile(`[._\-@·•]+`)
)

// NormalizeLanguageCode maps any raw language text or alias to its canonical lowercase code.
func NormalizeLanguageCode(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if code, ok := languageAliases[raw]; ok {
		return code
	}
	// Check if ends with "dubbed" (e.g. "malayalam dubbed" -> "malayalam")
	trimmed := strings.TrimSuffix(raw, " dubbed")
	trimmed = strings.TrimSuffix(trimmed, " audio")
	trimmed = strings.TrimSpace(trimmed)
	if code, ok := languageAliases[trimmed]; ok {
		return code
	}
	return ""
}

// FormatLanguageDisplay returns the human-readable display string for a canonical language code.
func FormatLanguageDisplay(code string) string {
	switch strings.ToLower(strings.TrimSpace(code)) {
	case "malayalam":
		return "Malayalam"
	case "tamil":
		return "Tamil"
	case "telugu":
		return "Telugu"
	case "hindi":
		return "Hindi"
	case "kannada":
		return "Kannada"
	case "english":
		return "English"
	case "bengali":
		return "Bengali"
	case "marathi":
		return "Marathi"
	case "punjabi":
		return "Punjabi"
	case "gujarati":
		return "Gujarati"
	case "multi":
		return "Multi Audio"
	case "original":
		return "Original Audio"
	default:
		if code == "" || code == "unknown" {
			return "Original"
		}
		runes := []rune(code)
		if len(runes) > 0 {
			runes[0] = unicode.ToUpper(runes[0])
			return string(runes)
		}
		return code
	}
}

// ParseQueryIntent separates a raw user search query into structured components:
// title, requested language, quality, year, and season/episode.
func ParseQueryIntent(rawQuery string) QueryIntent {
	rawQuery = strings.TrimSpace(rawQuery)
	intent := QueryIntent{
		RawQuery:   rawQuery,
		CleanTitle: rawQuery,
	}

	if rawQuery == "" {
		return intent
	}

	workingText := reIntentDelims.ReplaceAllString(rawQuery, " ")

	// 1. Detect Season / Episode
	if m := reIntentSeasonEp.FindStringSubmatch(workingText); len(m) == 3 {
		sNum, _ := strconv.Atoi(m[1])
		eNum, _ := strconv.Atoi(m[2])
		intent.Season = sNum
		intent.Episode = eNum
		workingText = reIntentSeasonEp.ReplaceAllString(workingText, " ")
	} else {
		if m := reIntentSeason.FindStringSubmatch(workingText); len(m) == 2 {
			sNum, _ := strconv.Atoi(m[1])
			intent.Season = sNum
			workingText = reIntentSeason.ReplaceAllString(workingText, " ")
		}
		if m := reIntentEpisode.FindStringSubmatch(workingText); len(m) == 2 {
			eNum, _ := strconv.Atoi(m[1])
			intent.Episode = eNum
			workingText = reIntentEpisode.ReplaceAllString(workingText, " ")
		}
	}

	// 2. Detect Quality
	if m := reIntentQuality.FindString(workingText); m != "" {
		intent.Quality = canonicalRes(m)
		intent.HasExplicitQuality = true
		workingText = reIntentQuality.ReplaceAllString(workingText, " ")
	}

	// 3. Detect Language
	// Look for multi-word language aliases first (e.g. "multi audio", "dual audio", "malayalam dubbed")
	words := strings.Fields(workingText)
	if len(words) > 1 {
		// Check two-word suffix: e.g. "multi audio", "dual audio"
		lastTwo := strings.ToLower(words[len(words)-2] + " " + words[len(words)-1])
		if code := NormalizeLanguageCode(lastTwo); code != "" {
			intent.Language = code
			intent.HasExplicitLanguage = true
			words = words[:len(words)-2]
			workingText = strings.Join(words, " ")
		}
	}

	if intent.Language == "" && len(words) > 1 {
		// Check single-word suffix: e.g. "I'm Game Malayalam" -> last word is "Malayalam"
		lastWord := strings.ToLower(words[len(words)-1])
		// Strip trailing punctuation from lastWord (e.g. "malayalam?")
		lastWord = strings.TrimRight(lastWord, "?!.,;:)")
		if code := NormalizeLanguageCode(lastWord); code != "" {
			intent.Language = code
			intent.HasExplicitLanguage = true
			words = words[:len(words)-1]
			workingText = strings.Join(words, " ")
		}
	}

	// 4. Detect Year (only if after a title or at end)
	if m := reIntentYear.FindStringSubmatch(workingText); len(m) == 2 {
		yNum, _ := strconv.Atoi(m[1])
		if yNum >= 1920 && yNum <= 2035 {
			intent.Year = yNum
			// Remove year only if not the entire title
			if len(strings.TrimSpace(reIntentYear.ReplaceAllString(workingText, ""))) > 0 {
				workingText = reIntentYear.ReplaceAllString(workingText, " ")
			}
		}
	}

	// 5. Clean Title
	title := strings.TrimSpace(workingText)
	title = strings.Trim(title, " -|:_·•[](){}~")
	title = strings.TrimSpace(reWhitespace.ReplaceAllString(title, " "))

	// If title became empty (e.g. user literally searched "1080p" or "Malayalam"), preserve original query
	if title == "" {
		title = rawQuery
	}

	intent.CleanTitle = title
	return intent
}
