package search

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// ReleaseMetadata contains structured metadata extracted from a media filename.
type ReleaseMetadata struct {
	RawFileName     string
	CleanTitle      string   // Human-readable title: e.g. "Vikram", "Vikram Vedha", "Hargadimaju"
	NormalizedTitle string   // Lowercase clean title: e.g. "vikram", "vikram vedha"
	TitleTokens     []string // Meaningful tokens of the title
	Year            int      // 4-digit release year (0 if not found)
	Season          int      // Season number (0 if not found)
	Episode         int      // Episode number (0 if not found)
	EpisodeTitle    string   // Episode title if applicable
	ChannelTag      string   // Uploader/Channel tag if identified
	Languages       []string
	Qualities       []string
}

var (
	// Strip file extensions
	reMediaExt = regexp.MustCompile(`(?i)\.(mkv|mp4|avi|webm|flv|mov|m4v|3gp|zip|rar|tar|gz|7z)$`)

	// Leading brackets: [CinemaCourt], (TamilRockers), {TG}, etc.
	reLeadingBracket = regexp.MustCompile(`^(\[|\(|\{|【)([^\]\)\}】]+)(\]|\)|\}|】)\s*`)

	// Trailing brackets: [CinemaCourt], [TG], etc.
	reTrailingBracket = regexp.MustCompile(`\s*(\[|\(|\{|【)([^\]\)\}】]+)(\]|\)|\}|】)$`)

	// Leading handles: @CinemaCourt
	reLeadingHandle = regexp.MustCompile(`^@[a-zA-Z0-9_]+\s*[-_:]?\s*`)

	// Leading URL or domain: www.cinemacourt.com, cinemacourt.me, 1tamilmv.cz
	reLeadingDomain = regexp.MustCompile(`(?i)^(?:https?://\S+|www\.\S+|t\.me/\S+|[a-zA-Z0-9_-]+\.(com|net|org|in|cz|is|lat|to|me|cc|co|club|shop|tube|site|top|vip|link|info|biz|tv|io))\s*[-_:~|]?\s*`)

	// Delimiter separator in the first 35 chars: "CinemaCourt - Vikram" or "CinemaCourt | Vikram"
	reDelimiterPrefix = regexp.MustCompile(`^([a-zA-Z0-9\s._@]+?)\s*[-|:~]\s*(.+)$`)

	// Trailing handle or tag: "-@MNTG", "- @CinemaCourt", etc.
	reTrailingHandle = regexp.MustCompile(`(?i)\s*[-_–|:~]?\s*@[a-zA-Z0-9_]+\s*$`)

	// Season and Episode patterns
	reSeasonAndEpisode = regexp.MustCompile(`(?i)\b(?:s|season)[\s._-]*(\d{1,2})[\s._-]*(?:e|ep|episode)[\s._-]*(\d{1,2})\b`)
	reSeasonOnly       = regexp.MustCompile(`(?i)\b(?:s|season)[\s._-]*(\d{1,2})\b`)
	reEpisodeOnly      = regexp.MustCompile(`(?i)\b(?:e|ep|episode)[\s._-]*(\d{1,2})\b`)

	// 4-digit year: (19xx or 20xx)
	reYearPattern = regexp.MustCompile(`\b(19\d\d|20\d\d)\b`)

	// Quality & resolution tags
	reQualityPattern = regexp.MustCompile(`(?i)\b(1080p|720p|480p|360p|2160p|4k|576p|hdr|webrip|web-dl|web|bluray|hdrip|dvdrip|brrip|x264|x265|hevc|h264|h265|avc|10bit|remux|bdrip|esub|esubs|msub|sub|subtitle|aac\d*(\.\d+)?|ddp5\.1|dd5\.1|ac3|atmos)\b`)

	// Language tags
	reLanguagePattern = regexp.MustCompile(`(?i)\b(malayalam|tamil|hindi|telugu|kannada|english|bengali|marathi|multi|dual[\s._-]*audio|dubbed|mal|tam|hin|tel|kan|eng)\b`)
)

var knownChannelKeywords = map[string]struct{}{
	"cinemacourt": {}, "cinemacourts": {}, "cinemacourtz": {}, "cinema_court": {},
	"tamilrockers": {}, "1tamilmv": {}, "tamilmv": {}, "tamilblasters": {},
	"tamildbox": {}, "isaimini": {}, "kuttymovies": {}, "tamilyogi": {},
	"bollyflix": {}, "mallumv": {}, "malayalamrockers": {}, "dvdplay": {},
	"sargamify": {}, "tgxcyberbot": {}, "cyberbot": {}, "mytgbotz": {},
	"mntg": {}, "mnt": {}, "tg": {}, "tgx": {}, "channel": {}, "media": {}, "studios": {},
	"official": {}, "group": {}, "team": {}, "bot": {},
}

// isChannelPrefixWord checks whether a token looks like an uploader/channel name,
// provided it is NOT present in the user query.
func isChannelPrefixWord(token string, queryTokens []string) bool {
	tok := strings.ToLower(strings.TrimSpace(token))
	if tok == "" {
		return false
	}

	// Never strip words that the user explicitly searched for!
	for _, qt := range queryTokens {
		if strings.ToLower(qt) == tok {
			return false
		}
	}

	if strings.HasPrefix(tok, "@") {
		return true
	}

	if _, ok := knownChannelKeywords[tok]; ok {
		return true
	}

	if strings.HasPrefix(tok, "cinema") && (strings.HasSuffix(tok, "court") || strings.HasSuffix(tok, "hub") || strings.HasSuffix(tok, "world") || strings.HasSuffix(tok, "flix")) {
		return true
	}

	if strings.HasSuffix(tok, "rockers") || strings.HasSuffix(tok, "blasters") || strings.HasSuffix(tok, "yogi") || strings.HasSuffix(tok, "flix") {
		return true
	}

	return false
}

// normalizeSequelToken maps roman numerals and numbers for movie sequels.
func normalizeSequelToken(tok string) string {
	switch strings.ToLower(tok) {
	case "i", "1", "part1", "chapter1":
		return "1"
	case "ii", "2", "part2", "chapter2":
		return "2"
	case "iii", "3", "part3", "chapter3":
		return "3"
	case "iv", "4", "part4", "chapter4":
		return "4"
	case "v", "5", "part5", "chapter5":
		return "5"
	case "vi", "6", "part6", "chapter6":
		return "6"
	default:
		return tok
	}
}

// ParseRelease parses a raw media filename into structured ReleaseMetadata,
// intelligently identifying channel tags, movie title, year, season, episode, etc.
func ParseRelease(filename string, userQuery string) ReleaseMetadata {
	meta := ReleaseMetadata{RawFileName: filename}
	if strings.TrimSpace(filename) == "" {
		return meta
	}

	s := reMediaExt.ReplaceAllString(filename, "")

	var queryTokens []string
	if userQuery != "" {
		qNorm := NormalizeText(userQuery)
		queryTokens = strings.Fields(qNorm)
	}

	// 1. Strip leading bracketed tags: [CinemaCourt], (TamilRockers), etc.
	for {
		m := reLeadingBracket.FindStringSubmatch(s)
		if len(m) < 3 {
			break
		}
		inner := strings.TrimSpace(m[2])
		// Check if it's a 4-digit year like [2022]
		if reYearPattern.MatchString(inner) {
			break
		}
		// Check if it's a quality tag like [1080p]
		if reQualityPattern.MatchString(inner) {
			meta.Qualities = append(meta.Qualities, inner)
			s = s[len(m[0]):]
			continue
		}
		meta.ChannelTag = inner
		s = s[len(m[0]):]
	}

	// 2. Strip leading URLs or domain prefixes: www.cinemacourt.com -
	if m := reLeadingDomain.FindStringSubmatch(s); len(m) > 0 {
		meta.ChannelTag = strings.TrimSpace(m[0])
		s = s[len(m[0]):]
	}

	// 3. Strip leading @handles: @CinemaCourt
	if m := reLeadingHandle.FindStringSubmatch(s); len(m) > 0 {
		meta.ChannelTag = strings.TrimSpace(m[0])
		s = s[len(m[0]):]
	}

	// 3b. Strip trailing @channel handles: e.g. "The Early Spring S01E12 480p -@MNTG"
	if loc := reTrailingHandle.FindStringIndex(s); loc != nil {
		meta.ChannelTag = strings.TrimSpace(s[loc[0]:loc[1]])
		s = strings.TrimRight(s[:loc[0]], " -_–|:~")
	}

	// 4. Check for delimiter channel prefix: e.g. "CinemaCourt - Vikram" or "@CinemaCourt - Vikram"
	if len(s) > 3 {
		checkPrefix := s
		if len(checkPrefix) > 40 {
			checkPrefix = checkPrefix[:40]
		}
		if m := reDelimiterPrefix.FindStringSubmatch(checkPrefix); len(m) == 3 {
			prefix := strings.TrimSpace(m[1])
			cleanPrefix := strings.ToLower(rePunctuation.ReplaceAllString(prefix, ""))

			// A prefix before a delimiter is ONLY a channel tag if it matches known channel patterns
			// and does NOT contain any search tokens.
			hasQueryWord := false
			for _, qt := range queryTokens {
				if len(qt) >= 2 && strings.Contains(cleanPrefix, strings.ToLower(qt)) {
					hasQueryWord = true
					break
				}
			}

			if !hasQueryWord && (isChannelPrefixWord(cleanPrefix, queryTokens) || strings.HasPrefix(prefix, "@")) {
				meta.ChannelTag = prefix
				s = s[len(prefix):]
				s = strings.TrimLeft(s, " -|:~_")
			}
		}
	}

	// 5. Detect Season / Episode
	var titlePart string
	if loc := reSeasonAndEpisode.FindStringSubmatchIndex(s); loc != nil {
		sNum, _ := strconv.Atoi(s[loc[2]:loc[3]])
		eNum, _ := strconv.Atoi(s[loc[4]:loc[5]])
		meta.Season = sNum
		meta.Episode = eNum
		titlePart = strings.TrimSpace(s[:loc[0]])
		afterPart := strings.TrimSpace(s[loc[1]:])
		// Episode title is after season/ep (before quality/year)
		if qLoc := reQualityPattern.FindStringIndex(afterPart); qLoc != nil {
			meta.EpisodeTitle = strings.TrimSpace(afterPart[:qLoc[0]])
		} else {
			meta.EpisodeTitle = afterPart
		}
	} else if loc := reSeasonOnly.FindStringSubmatchIndex(s); loc != nil {
		sNum, _ := strconv.Atoi(s[loc[2]:loc[3]])
		meta.Season = sNum
		titlePart = strings.TrimSpace(s[:loc[0]])
	} else if loc := reEpisodeOnly.FindStringSubmatchIndex(s); loc != nil {
		eNum, _ := strconv.Atoi(s[loc[2]:loc[3]])
		meta.Episode = eNum
		titlePart = strings.TrimSpace(s[:loc[0]])
	}

	// 6. Detect Year (if titlePart not already isolated by season/ep)
	if titlePart == "" {
		if loc := reYearPattern.FindStringIndex(s); loc != nil {
			yNum, _ := strconv.Atoi(s[loc[0]:loc[1]])
			meta.Year = yNum
			titlePart = strings.TrimSpace(s[:loc[0]])
		}
	} else {
		// Also record year if present in remaining string
		if m := reYearPattern.FindStringSubmatch(s); len(m) > 1 {
			meta.Year, _ = strconv.Atoi(m[1])
		}
	}

	// 7. If no season/ep or year, detect first Quality or Language marker
	if titlePart == "" {
		qIdx := -1
		if loc := reQualityPattern.FindStringIndex(s); loc != nil {
			qIdx = loc[0]
		}
		lIdx := -1
		if loc := reLanguagePattern.FindStringIndex(s); loc != nil {
			lIdx = loc[0]
		}

		firstCut := -1
		if qIdx >= 0 && lIdx >= 0 {
			if qIdx < lIdx {
				firstCut = qIdx
			} else {
				firstCut = lIdx
			}
		} else if qIdx >= 0 {
			firstCut = qIdx
		} else if lIdx >= 0 {
			firstCut = lIdx
		}

		if firstCut > 0 {
			titlePart = strings.TrimSpace(s[:firstCut])
		} else {
			titlePart = strings.TrimSpace(s)
		}
	}

	// 8. Clean titlePart: remove punctuation, trailing brackets/hyphens
	titlePart = strings.TrimRight(titlePart, " .-_:[(<{")
	titlePart = strings.TrimLeft(titlePart, " .-_:[(<{")

	// 9. Check for unbracketed Channel / Group prefix at start of title:
	// e.g. "Cinemacourt Vikram" or "Cinema Court Vikram"
	words := strings.Fields(titlePart)
	if len(words) >= 2 {
		firstWord := words[0]
		cleanFirst := strings.ToLower(rePunctuation.ReplaceAllString(firstWord, ""))
		if isChannelPrefixWord(cleanFirst, queryTokens) {
			meta.ChannelTag = firstWord
			words = words[1:]
			titlePart = strings.Join(words, " ")
		} else if len(words) >= 3 {
			// Check two-word channel prefix: "Cinema Court Vikram"
			twoWords := cleanFirst + " " + strings.ToLower(rePunctuation.ReplaceAllString(words[1], ""))
			if twoWords == "cinema court" {
				meta.ChannelTag = words[0] + " " + words[1]
				words = words[2:]
				titlePart = strings.Join(words, " ")
			}
		}
	}

	// 10. Clean up Title: replace dots and underscores with spaces for CleanTitle
	cleanTitle := strings.ReplaceAll(titlePart, ".", " ")
	cleanTitle = strings.ReplaceAll(cleanTitle, "_", " ")
	cleanTitle = strings.TrimSpace(reWhitespace.ReplaceAllString(cleanTitle, " "))
	meta.CleanTitle = cleanTitle
	meta.NormalizedTitle = NormalizeText(cleanTitle)

	// Extract meaningful tokens of title
	meta.TitleTokens = ExtractMeaningfulTokens(meta.NormalizedTitle)
	if len(meta.TitleTokens) == 0 && meta.NormalizedTitle != "" {
		meta.TitleTokens = strings.Fields(meta.NormalizedTitle)
	}

	return meta
}

// MatchMovie performs intelligent, strict matching between a user search query and ReleaseMetadata.
// Returns whether it strictly matches, along with a similarity score [0.0, 1.0].
func MatchMovie(userQuery string, rel ReleaseMetadata) (bool, float64) {
	normQuery := NormalizeText(userQuery)
	if normQuery == "" {
		return false, 0.0
	}

	// 1. Extract query year if user specified one (e.g. "Vikram 2022")
	var qYear int
	if m := reYearPattern.FindStringSubmatch(userQuery); len(m) > 1 {
		qYear, _ = strconv.Atoi(m[1])
	}

	// 2. Extract query season if user specified one (e.g. "Hargadimaju S3")
	var qSeason int
	if m := reSeasonOnly.FindStringSubmatch(userQuery); len(m) > 1 {
		qSeason, _ = strconv.Atoi(m[1])
	}

	// 3. Extract query episode if user specified one (e.g. "Episode 6")
	var qEpisode int
	if m := reEpisodeOnly.FindStringSubmatch(userQuery); len(m) > 1 {
		qEpisode, _ = strconv.Atoi(m[1])
	}

	// Strip season/ep/year tokens from query string before extracting title tokens
	cleanQueryText := normQuery
	if qYear > 0 {
		cleanQueryText = strings.ReplaceAll(cleanQueryText, strconv.Itoa(qYear), " ")
	}
	if qSeason > 0 {
		cleanQueryText = reSeasonOnly.ReplaceAllString(cleanQueryText, " ")
		cleanQueryText = strings.ReplaceAll(cleanQueryText, "s"+strconv.Itoa(qSeason), " ")
		cleanQueryText = strings.ReplaceAll(cleanQueryText, "season "+strconv.Itoa(qSeason), " ")
	}
	if qEpisode > 0 {
		cleanQueryText = reEpisodeOnly.ReplaceAllString(cleanQueryText, " ")
		cleanQueryText = strings.ReplaceAll(cleanQueryText, "e"+strconv.Itoa(qEpisode), " ")
		cleanQueryText = strings.ReplaceAll(cleanQueryText, "ep "+strconv.Itoa(qEpisode), " ")
	}

	// Extract meaningful query tokens
	qTokens := ExtractMeaningfulTokens(cleanQueryText)
	if len(qTokens) == 0 {
		qTokens = strings.Fields(cleanQueryText)
	}

	// RULE 1: Year Mismatch
	if qYear > 0 && rel.Year > 0 && qYear != rel.Year {
		return false, 0.0
	}

	// RULE 2: Season / Episode Mismatch
	if qSeason > 0 && rel.Season > 0 && qSeason != rel.Season {
		return false, 0.0
	}
	if qEpisode > 0 && rel.Episode > 0 && qEpisode != rel.Episode {
		return false, 0.0
	}

	// RULE 3: TV Show vs Episode Title Check
	// If the file is a TV series episode (e.g. "Hargadimaju Season 3 Episode 6 Vikram Thilak"),
	// the search MUST match the TV show's title ("Hargadimaju"), not just an episode name or character name.
	if rel.Season > 0 || rel.Episode > 0 {
		showMatched := false
		for _, qt := range qTokens {
			for _, tt := range rel.TitleTokens {
				if qt == tt {
					showMatched = true
					break
				}
			}
			if showMatched {
				break
			}
		}
		if !showMatched {
			// Show title was not part of the search query!
			return false, 0.0
		}
	}

	// RULE 4: All Query Tokens Must Match in File's Title Tokens
	matchedTitleIndices := make(map[int]struct{})
	for _, qt := range qTokens {
		qNormTok := normalizeSequelToken(qt)
		found := false
		for i, tt := range rel.TitleTokens {
			tNormTok := normalizeSequelToken(tt)
			if qNormTok == tNormTok {
				matchedTitleIndices[i] = struct{}{}
				found = true
				break
			}
		}
		if !found {
			// Query token not found in title!
			return false, 0.0
		}
	}

	// RULE 5: Strict Check for Extra Title Tokens (Reject Different Movies)
	// Example: Query is "Vikram", file is "Vikram Vedha".
	// The extra word "vedha" is a substantive movie title word, meaning it's a DIFFERENT movie!
	var extraSubstantiveWords []string
	for i, tt := range rel.TitleTokens {
		if _, matched := matchedTitleIndices[i]; matched {
			continue
		}
		tok := strings.ToLower(tt)
		// Stopwords are allowed
		if _, isStop := genericStopwords[tok]; isStop {
			continue
		}
		// Sequel numbers: if query didn't specify sequel, allowed as franchise match
		normSeq := normalizeSequelToken(tok)
		if normSeq == "1" || normSeq == "2" || normSeq == "3" || normSeq == "4" || normSeq == "5" || normSeq == "6" {
			continue
		}
		// Any other substantive word is a distinct title word!
		extraSubstantiveWords = append(extraSubstantiveWords, tt)
	}

	if len(extraSubstantiveWords) > 0 {
		// Title has extra words not requested by user (e.g. "Vedha" in "Vikram Vedha")
		return false, 0.0
	}

	// Scoring:
	// Exact match with no extra tokens: 1.0
	// Sequel/franchise match (e.g. user searched "KGF" and matched "KGF 2"): 0.95
	if len(rel.TitleTokens) == len(matchedTitleIndices) {
		return true, 1.0
	}
	return true, 0.95
}

// FilterAndRankStrict takes raw candidate files and returns only strictly matching files,
// ordered by score descending, then date_saved descending.
func FilterAndRankStrict(query string, records []MediaFile) []MediaFile {
	if len(records) == 0 {
		return nil
	}

	type scoredItem struct {
		file  MediaFile
		score float64
	}

	var matched []scoredItem
	for _, r := range records {
		rel := ParseRelease(r.FileName, query)
		isMatch, score := MatchMovie(query, rel)
		if isMatch {
			matched = append(matched, scoredItem{file: r, score: score})
		}
	}

	if len(matched) == 0 {
		return nil
	}

	sort.SliceStable(matched, func(i, j int) bool {
		if matched[i].score != matched[j].score {
			return matched[i].score > matched[j].score
		}
		return matched[i].file.DateSaved.After(matched[j].file.DateSaved)
	})

	result := make([]MediaFile, len(matched))
	for i, m := range matched {
		result[i] = m.file
	}
	return result
}
