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
	"പ്രേമം":       "premam",
	"ജി ഡി നായിഡു": "gd naidu",
	"വിക്രം":       "vikram",
	"വിക്രമാദിത്യൻ": "vikramadithyan",
	"വിക്രം വേധ":   "vikram vedha",
	"വിക്രം വേദ":   "vikram vedha",
	"അമരൻ":         "amaran",
	"ലൂസിഫർ":       "lucifer",
	"ദൃശ്യം":       "drishyam",
	"കെജിഎഫ്":      "kgf",
	"കാന്താര":      "kantara",
	// Tamil
	"கைதி":          "kaithi",
	"விக்ரம்":       "vikram",
	"விக்ரம் வேதா":  "vikram vedha",
	"விக்ரமாதித்தன்": "vikramadithyan",
	"ஜி ഡി നായിഡു2": "gd naidu",
	"லியோ":          "leo",
	"அமரன்":         "amaran",
	"ஜெயிலர்":       "jailer",
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
	reChannelTag  = regexp.MustCompile(`@[a-zA-Z0-9_]+`)
	reURL         = regexp.MustCompile(`https?://\S+|www\.\S+|t\.me/\S+`)
	rePunctuation = regexp.MustCompile(`[\._\-\[\(\]\):\=,\+!#\|/\\?;"'<>~` + "`" + `]+`)
	reWhitespace  = regexp.MustCompile(`\s+`)
	reExtensions  = regexp.MustCompile(`(?i)\.(mkv|mp4|avi|webm|flv|mov|m4v|3gp|zip|rar|tar|gz|7z)$`)
	reReleaseTags = regexp.MustCompile(`(?i)\b(1080p|720p|480p|360p|2160p|4k|576p|hdr|webrip|web-dl|web|bluray|hdrip|dvdrip|brrip|x264|x265|hevc|h264|esub|esubs|msub|sub|subtitle|subtitles|aac\d*(\.\d+)?|ddp5\.1|dd5\.1)\b`)
	reSeasonEp    = regexp.MustCompile(`(?i)\b(?:s|season)[\s._-]*(\d{1,2})[\s._-]*(?:e|ep|episode)[\s._-]*(\d{1,2})\b`)
)

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

	// Remove URLs and Channel Tags
	text = reChannelTag.ReplaceAllString(text, " ")
	text = reURL.ReplaceAllString(text, " ")

	// Lowercase
	text = strings.ToLower(text)

	// Replace non-alphanumeric punctuation with spaces
	text = rePunctuation.ReplaceAllString(text, " ")

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
		if len(tok) < 2 && !unicode.IsDigit(rune(tok[0])) {
			continue
		}
		if _, isStop := genericStopwords[tok]; isStop {
			continue
		}
		meaningful = append(meaningful, tok)
	}

	// If all tokens were stopwords (e.g. user searched for "film"), fallback to raw tokens
	if len(meaningful) == 0 {
		for _, tok := range rawTokens {
			if len(tok) >= 2 || (len(tok) == 1 && unicode.IsDigit(rune(tok[0]))) {
				meaningful = append(meaningful, tok)
			}
		}
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
	title = reChannelTag.ReplaceAllString(title, " ")
	title = reURL.ReplaceAllString(title, " ")

	// Remove release tags (codecs, resolutions)
	title = reReleaseTags.ReplaceAllString(title, " ")

	// Replace punctuation
	title = rePunctuation.ReplaceAllString(title, " ")

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
