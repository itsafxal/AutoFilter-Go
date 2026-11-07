package search

import (
	"regexp"
	"strings"
	"unicode"
)

var genericStopwords = map[string]struct{}{
	"movie": {}, "movies": {}, "film": {}, "films": {}, "series": {},
	"season": {}, "seasons": {}, "ep": {}, "episode": {}, "episodes": {},
	"the": {}, "a": {}, "an": {}, "of": {}, "in": {}, "on": {}, "at": {},
	"and": {}, "or": {}, "to": {}, "for": {}, "with": {}, "by": {}, "from": {},
	"part": {}, "vol": {}, "volume": {}, "chapter": {}, "chapters": {}, "ch": {}, "hd": {}, "full": {}, "download": {},
	"1080p": {}, "720p": {}, "480p": {}, "360p": {}, "2160p": {}, "4k": {},
	"bluray": {}, "webrip": {}, "web-dl": {}, "web": {}, "hevc": {},
	"x264": {}, "x265": {}, "esubs": {}, "esub": {}, "msub": {}, "aac": {},
}

var transliterationMap = map[string]string{
	// Malayalam
	"പ്രേമം":        "premam",
	"ജി ഡി നായിഡു":  "gd naidu",
	"വിക്രം":        "vikram",
	"വിക്രമാദിത്യൻ": "vikramadithyan",
	"വിക്രം വേധ":    "vikram vedha",
	"വിക്രം വേദ":    "vikram vedha",
	"അമരൻ":          "amaran",
	"ലൂസിഫർ":        "lucifer",
	"ദൃശ്യം":        "drishyam",
	"കെജിഎഫ്":       "kgf",
	"കാന്താര":       "kantara",
	// Tamil
	"கைதி":           "kaithi",
	"விக்ரம்":        "vikram",
	"விக்ரம் வேதா":   "vikram vedha",
	"விக்ரமாதித்தன்": "vikramadithyan",
	"ஜி ഡി നായിഡു2":  "gd naidu",
	"லியோ":           "leo",
	"அமரன்":          "amaran",
	"ஜெயிலர்":        "jailer",
	// Telugu
	"విక్రమ్": "vikram",
	"పుష్ప":   "pushpa",
	// Kannada
	"ಕೆಜಿಎಫ್": "kgf",
	// Hindi
	"मुसाफिर": "musafir",
	"कैफे":    "cafe",
	"जवान":    "jawan",
	"पठान":    "pathaan",
	"केजीएफ":  "kgf",
}

var (
	reChannelTagHandle = regexp.MustCompile(`(?i)(?:^|[\s\[\(\{\-_|~])@[a-zA-Z0-9_]+(?:\s|[\)\]\}\-_|~]|$)`)
	reURL              = regexp.MustCompile(`https?://\S+|www\.\S+|t\.me/\S+`)
	reWhitespace       = regexp.MustCompile(`\s+`)
	rePunctuation      = regexp.MustCompile(`[\._\-\[\(\]\):\=,\+!#\|/\\?;"'<>~` + "`" + `•·]+`)
	reExtensions       = regexp.MustCompile(`(?i)\.(mkv|mp4|avi|webm|flv|mov|m4v|3gp|zip|rar|tar|gz|7z)$`)
	reReleaseTags      = regexp.MustCompile(`(?i)\b(1080p|720p|480p|360p|2160p|4k|576p|hdr|webrip|web-dl|web|bluray|hdrip|dvdrip|brrip|x264|x265|hevc|h264|esub|esubs|msub|sub|subtitle|subtitles|aac\d*(\.\d+)?|ddp5\.1|dd5\.1)\b`)
	reSeasonEp         = regexp.MustCompile(`(?i)\b(?:s|season)[\s._-]*(\d{1,2})[\s._-]*(?:e|ep|episode)[\s._-]*(\d{1,2})\b`)
	reNameLabel        = regexp.MustCompile(`(?i)^[^\w]*(?:name|title|file|movie)[\s:➜\-=]+(.+)$`)
	reIgnoreLines      = regexp.MustCompile(`(?i)^[^\w]*(?:size|quality|language|audio|powered|join|link|download|uploaded)[\s:➜\-=]+`)
)

// isTitleRune checks whether a rune is a meaningful letter, digit, or combining mark.
// unicode.IsMark is crucial for preserving Indic script vowels and matras (e.g. Malayalam, Hindi, Tamil).
func isTitleRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r)
}

// SplitCamelCaseAndNumbers splits concatenated words at camelCase transitions
// and letter/digit boundaries (e.g. "Jailer2" -> "Jailer 2", "IAmGame" -> "I Am Game", "KGFChapter2" -> "KGF Chapter 2").
func SplitCamelCaseAndNumbers(s string) string {
	if s == "" {
		return ""
	}
	runes := []rune(s)
	n := len(runes)
	var b strings.Builder
	b.Grow(len(s) + 8)

	for i := 0; i < n; i++ {
		r := runes[i]
		if i > 0 {
			prev := runes[i-1]
			// Transition 1: lowercase followed by uppercase (e.g., "spiderMan" -> "spider Man", "IAmGame" -> "I Am Game")
			if unicode.IsLower(prev) && unicode.IsUpper(r) {
				b.WriteByte(' ')
			} else if unicode.IsUpper(prev) && unicode.IsUpper(r) && i+1 < n && unicode.IsLower(runes[i+1]) {
				// Transition 2: sequence of uppercase followed by uppercase+lowercase (e.g., "KGFChapter" -> "KGF Chapter")
				b.WriteByte(' ')
			} else if (unicode.IsLetter(prev) && unicode.IsDigit(r)) || (unicode.IsDigit(prev) && unicode.IsLetter(r)) {
				// Transition 3: letter followed by digit or digit followed by letter (e.g. "Jailer2" -> "Jailer 2", "2Fast" -> "2 Fast")
				b.WriteByte(' ')
			}
		}
		b.WriteRune(r)
	}
	return b.String()
}

// NormalizeCompact produces a compact representation of a title with all separators,
// spaces, and punctuation removed, keeping only letters, digits, and combining marks (e.g., "Jailer 2" -> "jailer2").
func NormalizeCompact(text string) string {
	if text == "" {
		return ""
	}
	for src, target := range transliterationMap {
		if strings.Contains(text, src) {
			text = strings.ReplaceAll(text, src, target)
		}
	}
	text = strings.ToLower(text)
	var b strings.Builder
	b.Grow(len(text))
	for _, r := range text {
		if isTitleRune(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// CollapseAcronyms collapses consecutive single-letter words (e.g., "k g f" -> "kgf", "d d l j" -> "ddlj").
func CollapseAcronyms(text string) string {
	words := strings.Fields(text)
	if len(words) <= 1 {
		return text
	}
	var result []string
	var acro strings.Builder
	for _, w := range words {
		runes := []rune(w)
		if len(runes) == 1 && unicode.IsLetter(runes[0]) {
			acro.WriteRune(runes[0])
		} else {
			if acro.Len() > 0 {
				result = append(result, acro.String())
				acro.Reset()
			}
			result = append(result, w)
		}
	}
	if acro.Len() > 0 {
		result = append(result, acro.String())
	}
	return strings.Join(result, " ")
}

// LevenshteinDistance calculates the minimum edit distance between two strings.
func LevenshteinDistance(s1, s2 string) int {
	r1, r2 := []rune(s1), []rune(s2)
	n1, n2 := len(r1), len(r2)
	if n1 == 0 {
		return n2
	}
	if n2 == 0 {
		return n1
	}

	dp := make([]int, n2+1)
	for j := 0; j <= n2; j++ {
		dp[j] = j
	}

	for i := 1; i <= n1; i++ {
		prev := dp[0]
		dp[0] = i
		for j := 1; j <= n2; j++ {
			temp := dp[j]
			cost := 0
			if r1[i-1] != r2[j-1] {
				cost = 1
			}
			cand := dp[j] + 1
			if dp[j-1]+1 < cand {
				cand = dp[j-1] + 1
			}
			if prev+cost < cand {
				cand = prev + cost
			}
			dp[j] = cand
			prev = temp
		}
	}
	return dp[n2]
}

// NormalizeText cleans up raw search query text.
func NormalizeText(text string) string {
	if text == "" {
		return ""
	}

	// Transliteration replacements
	for src, target := range transliterationMap {
		if strings.Contains(text, src) {
			text = strings.ReplaceAll(text, src, target)
		}
	}

	// Remove URLs and Channel Tags at word boundaries
	text = reURL.ReplaceAllString(text, " ")
	text = reChannelTagHandle.ReplaceAllString(text, " ")

	// Lowercase
	text = strings.ToLower(text)

	// Replace non-title runes (punctuation, symbols, dots, hyphens, underscores, emojis, bullets) with spaces
	var b strings.Builder
	b.Grow(len(text))
	for _, r := range text {
		if isTitleRune(r) {
			b.WriteRune(r)
		} else {
			b.WriteByte(' ')
		}
	}
	text = b.String()

	// Collapse whitespace
	text = strings.TrimSpace(reWhitespace.ReplaceAllString(text, " "))
	return text
}

// ExtractMeaningfulTokens extracts search tokens, filtering out stop words when appropriate.
func ExtractMeaningfulTokens(cleanText string) []string {
	if cleanText == "" {
		return nil
	}

	rawTokens := strings.Fields(cleanText)
	if len(rawTokens) == 0 {
		return nil
	}

	meaningful := make([]string, 0, len(rawTokens))
	for _, tok := range rawTokens {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		if _, isStop := genericStopwords[tok]; isStop {
			continue
		}
		meaningful = append(meaningful, tok)
	}

	// If all tokens were stopwords (e.g. user searched for "film"), fallback to raw tokens
	if len(meaningful) == 0 {
		meaningful = rawTokens
	}

	return meaningful
}

// CleanReleaseName extracts a clean, human-readable title from a release filename.
func CleanReleaseName(filename string) string {
	if filename == "" {
		return ""
	}

	// Strip file extension
	title := reExtensions.ReplaceAllString(filename, "")

	// Remove URLs and channel tags
	title = reURL.ReplaceAllString(title, " ")
	title = reChannelTagHandle.ReplaceAllString(title, " ")

	// Remove release tags (codecs, resolutions)
	title = reReleaseTags.ReplaceAllString(title, " ")

	// Replace non-title runes with spaces
	var b strings.Builder
	b.Grow(len(title))
	for _, r := range title {
		if isTitleRune(r) {
			b.WriteRune(r)
		} else {
			b.WriteByte(' ')
		}
	}
	title = b.String()

	// Collapse whitespace
	title = strings.TrimSpace(reWhitespace.ReplaceAllString(title, " "))
	return title
}

// ComputeSimilarity computes a fast similarity score [0.0, 1.0] between query and title.
func ComputeSimilarity(query, title string) float64 {
	qNorm := NormalizeText(query)
	tNorm := NormalizeText(title)
	if qNorm == "" || tNorm == "" {
		return 0.0
	}

	if qNorm == tNorm {
		return 1.0
	}

	// Compact match
	if NormalizeCompact(query) == NormalizeCompact(title) {
		return 0.99
	}

	// Word-boundary prefix match: only if whole word matches or ends at word boundary
	if strings.HasPrefix(tNorm, qNorm) && (len(tNorm) == len(qNorm) || tNorm[len(qNorm)] == ' ') {
		return 0.95
	}

	// Word-boundary substring match
	if strings.Contains(" "+tNorm+" ", " "+qNorm+" ") {
		return 0.90
	}

	// Token overlap
	qTokens := strings.Fields(qNorm)
	tTokens := strings.Fields(tNorm)
	if len(qTokens) == 0 || len(tTokens) == 0 {
		return 0.0
	}

	matched := 0
	for _, qt := range qTokens {
		for _, tt := range tTokens {
			if qt == tt {
				matched++
				break
			}
			if len(qt) >= 4 && strings.HasPrefix(tt, qt) {
				matched++
				break
			}
			if len(qt) >= 3 && len(tt) >= 3 && (strings.HasPrefix(qt, tt) || strings.HasPrefix(tt, qt)) {
				matched++
				break
			}
			if len(qt) >= 4 && len(tt) >= 4 && LevenshteinDistance(qt, tt) <= 1 {
				matched++
				break
			}
		}
	}

	tokenRatio := float64(matched) / float64(len(qTokens))
	return tokenRatio * 0.85
}

// IsValidSearchText verifies whether a message is an eligible search text.
func IsValidSearchText(text string) bool {
	norm := NormalizeText(text)
	if len(norm) < 2 || len(norm) > 150 {
		return false
	}

	// Check if only punctuation or empty after normalization
	hasLetterOrDigit := false
	for _, r := range norm {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			hasLetterOrDigit = true
			break
		}
	}
	return hasLetterOrDigit
}

// ExtractQueryTitle inspects multi-line forwarded message captions or complex formatted
// search queries (e.g., "📕 Name ➜ : Monster S01E08\n📗 Size ➜ : 250MB") and extracts the actual movie title.
func ExtractQueryTitle(raw string) string {
	raw = strings.TrimSpace(raw)
	if !strings.Contains(raw, "\n") {
		return raw
	}

	lines := strings.Split(raw, "\n")
	// 1. First priority: a line starting with "Name:", "Title:", "File:", etc.
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if m := reNameLabel.FindStringSubmatch(trimmed); len(m) > 1 {
			val := strings.TrimSpace(m[1])
			if val != "" {
				return val
			}
		}
	}

	// 2. Second priority: a line containing a media file extension (.mkv, .mp4, etc.)
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if reExtensions.MatchString(trimmed) {
			if m := reNameLabel.FindStringSubmatch(trimmed); len(m) > 1 {
				return strings.TrimSpace(m[1])
			}
			return trimmed
		}
	}

	// 3. Third priority: pick the first non-empty line that isn't metadata (Size, Powered by, etc.)
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || reIgnoreLines.MatchString(trimmed) || strings.HasPrefix(strings.ToLower(trimmed), "powered by") {
			continue
		}
		return trimmed
	}

	return raw
}
