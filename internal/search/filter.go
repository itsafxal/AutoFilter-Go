package search

import (
	"regexp"
	"strings"
)

var (
	reResolutionFilter = regexp.MustCompile(`(?i)\b(2160p|4k|1080p|720p|480p|360p|576p)\b`)
	reMultiAudio       = regexp.MustCompile(`(?i)\b(multi|dual[\s._-]*audio|dual[\s._-]*track|multi[\s._-]*audio|multi[\s._-]*track)\b`)

	// Language word boundary regexes
	reMalayalam = regexp.MustCompile(`(?i)\b(malayalam|mal)\b`)
	reTamil     = regexp.MustCompile(`(?i)\b(tamil|tam)\b`)
	reHindi     = regexp.MustCompile(`(?i)\b(hindi|hin)\b`)
	reTelugu    = regexp.MustCompile(`(?i)\b(telugu|tel)\b`)
	reKannada   = regexp.MustCompile(`(?i)\b(kannada|kan)\b`)
	reEnglish   = regexp.MustCompile(`(?i)\b(english|eng)\b`)
	reBengali   = regexp.MustCompile(`(?i)\b(bengali|ben)\b`)
	reMarathi   = regexp.MustCompile(`(?i)\b(marathi|mar)\b`)
	rePunjabi   = regexp.MustCompile(`(?i)\b(punjabi|pun)\b`)
	reGujarati  = regexp.MustCompile(`(?i)\b(gujarati|guj)\b`)
)

func canonicalRes(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "4k" || raw == "2160p" {
		return "4k"
	}
	return raw
}

// DetectFileResolution extracts the video resolution from a filename or caption.
// Filename is authoritative; caption is only used as fallback when filename has no resolution tag
// and caption contains exactly one unique resolution tag.
func DetectFileResolution(fileName, caption string) string {
	cleanName := NormalizeText(fileName)
	if m := reResolutionFilter.FindString(cleanName); m != "" {
		return canonicalRes(m)
	}

	if strings.TrimSpace(caption) == "" {
		return ""
	}

	cleanCap := NormalizeText(caption)
	matches := reResolutionFilter.FindAllString(cleanCap, -1)
	if len(matches) == 0 {
		return ""
	}

	unique := make(map[string]bool)
	for _, m := range matches {
		unique[canonicalRes(m)] = true
	}

	// Multiple distinct resolutions in caption indicate a promotional channel template
	if len(unique) > 1 {
		return ""
	}

	for res := range unique {
		return res
	}
	return ""
}

// MatchesQuality checks whether the media file matches the requested resolution.
func MatchesQuality(fileName, caption, targetQuality string) bool {
	target := canonicalRes(targetQuality)
	if target == "" {
		return true
	}

	fileRes := DetectFileResolution(fileName, caption)
	if fileRes == "" {
		return false
	}

	return fileRes == target
}

// DetectFileLanguages extracts recognized language tags from a filename or caption.
// Filename is authoritative; caption is only checked if filename contains no language tags.
func DetectFileLanguages(fileName, caption string) []string {
	cleanName := NormalizeText(fileName)
	langs := extractLanguagesFromCleanText(cleanName)

	if len(langs) > 0 {
		return langs
	}

	if strings.TrimSpace(caption) == "" {
		return nil
	}

	cleanCap := NormalizeText(caption)
	capLangs := extractLanguagesFromCleanText(cleanCap)

	// If caption contains 4 or more regional languages without explicit multi/dual audio tag,
	// it's a generic channel promotional footer listing supported languages.
	hasMulti := false
	for _, l := range capLangs {
		if l == "multi" {
			hasMulti = true
			break
		}
	}
	if len(capLangs) >= 4 && !hasMulti {
		return nil
	}

	return capLangs
}

func extractLanguagesFromCleanText(text string) []string {
	if text == "" {
		return nil
	}

	var found []string
	addIfMatch := func(re *regexp.Regexp, code string) {
		if re.MatchString(text) {
			found = append(found, code)
		}
	}

	addIfMatch(reMalayalam, "malayalam")
	addIfMatch(reTamil, "tamil")
	addIfMatch(reHindi, "hindi")
	addIfMatch(reTelugu, "telugu")
	addIfMatch(reKannada, "kannada")
	addIfMatch(reEnglish, "english")
	addIfMatch(reBengali, "bengali")
	addIfMatch(reMarathi, "marathi")
	addIfMatch(rePunjabi, "punjabi")
	addIfMatch(reGujarati, "gujarati")

	isMultiAudio := reMultiAudio.MatchString(text)
	if isMultiAudio || len(found) >= 2 {
		found = append(found, "multi")
	}

	return found
}

// MatchesLanguage checks whether the media file contains the requested language.
func MatchesLanguage(fileName, caption, targetLang string) bool {
	target := strings.ToLower(strings.TrimSpace(targetLang))
	if target == "" {
		return true
	}

	fileLangs := DetectFileLanguages(fileName, caption)
	if len(fileLangs) == 0 {
		return false
	}

	for _, l := range fileLangs {
		if l == target {
			return true
		}
	}

	return false
}

// MatchesFilter checks whether the media file matches both language and quality filters.
func MatchesFilter(file MediaFile, filter SearchFilter) bool {
	if filter.Language != "" && !MatchesLanguage(file.FileName, file.Caption, filter.Language) {
		return false
	}
	if filter.Quality != "" && !MatchesQuality(file.FileName, file.Caption, filter.Quality) {
		return false
	}
	return true
}

// FilterByMediaFilter filters candidate records according to language and quality filters.
func FilterByMediaFilter(records []MediaFile, filter SearchFilter) []MediaFile {
	if filter.Language == "" && filter.Quality == "" {
		return records
	}

	var matched []MediaFile
	for _, r := range records {
		if MatchesFilter(r, filter) {
			matched = append(matched, r)
		}
	}
	return matched
}
