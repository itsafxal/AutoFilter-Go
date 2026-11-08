package search

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var (
	reCodecTag  = regexp.MustCompile(`(?i)\b(hevc|x265|x264|h\.?264|h\.?265|avc)\b`)
	reAudioTag  = regexp.MustCompile(`(?i)\b(dd\+?\s*5\.1|ddp\s*5\.1|dd\s*5\.1|dts|aac(?:\s*2\.0)?|ac3|truehd|atmos)\b`)
	reSourceTag = regexp.MustCompile(`(?i)\b(bluray|web-?dl|webrip|hdrip|dvdrip|brrip|hdtv)\b`)
)

// ExtractFileMetadata extracts resolution, codec, audio format, and release source from a media file.
func ExtractFileMetadata(f MediaFile) (res, codec, audio, source string) {
	text := f.FileName + " " + f.Caption
	res = DetectFileResolution(f.FileName, f.Caption)

	if m := reCodecTag.FindString(text); m != "" {
		codec = strings.ToUpper(m)
	}
	if m := reAudioTag.FindString(text); m != "" {
		audio = strings.ToUpper(m)
	}
	if m := reSourceTag.FindString(text); m != "" {
		source = strings.ToUpper(m)
	}
	return
}

// FormatFileOptionLabel formats a clean, readable label for an individual file button under a movie.
// Example: "1080p — x264 — DD+ 5.1 (2.5 GB)" or "720p — HEVC (800 MB)".
func FormatFileOptionLabel(f MediaFile) string {
	res, codec, audio, _ := ExtractFileMetadata(f)
	var parts []string

	if res != "" {
		parts = append(parts, strings.ToUpper(res))
	}
	if codec != "" {
		parts = append(parts, codec)
	}
	if audio != "" {
		parts = append(parts, audio)
	}

	sizeStr := f.Size
	if sizeStr == "" && f.FileSize > 0 {
		sizeStr = formatBytes(f.FileSize)
	}

	if len(parts) == 0 {
		cleanName := CleanReleaseName(f.FileName)
		if len(cleanName) > 25 {
			cleanName = cleanName[:25] + "..."
		}
		if sizeStr != "" {
			return fmt.Sprintf("%s (%s)", cleanName, sizeStr)
		}
		return cleanName
	}

	details := strings.Join(parts, " — ")
	if sizeStr != "" {
		return fmt.Sprintf("%s (%s)", details, sizeStr)
	}
	return details
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func resolutionRank(res string) int {
	switch strings.ToLower(strings.TrimSpace(res)) {
	case "4k", "2160p":
		return 5
	case "1080p":
		return 4
	case "720p":
		return 3
	case "576p", "480p":
		return 2
	case "360p":
		return 1
	default:
		return 0
	}
}

// GroupFilesIntoCanonicalMovies groups media files by canonical movie identity and language variants.
func GroupFilesIntoCanonicalMovies(files []MediaFile, query string, intent QueryIntent) []CanonicalMovie {
	if len(files) == 0 {
		return nil
	}

	type movieBuilder struct {
		movie    CanonicalMovie
		variants map[string][]MediaFile
	}

	builders := make(map[string]*movieBuilder)
	var movieOrder []string

	for _, file := range files {
		rel := ParseRelease(file.FileName, intent.CleanTitle)
		if rel.CleanTitle == "" {
			rel.CleanTitle = CleanReleaseName(file.FileName)
		}
		if rel.Year == 0 && file.Caption != "" {
			if m := reYearPattern.FindStringSubmatch(file.Caption); len(m) > 1 {
				rel.Year = parseYear(m[1])
			}
		}

		sequel := extractSequelNumber(rel.TitleTokens)
		mediaType := "movie"
		if rel.Season > 0 || rel.Episode > 0 {
			mediaType = "series"
		}

		canonicalTitle := rel.CleanTitle
		normalizedTitle := NormalizeText(canonicalTitle)
		compactTitle := NormalizeCompact(canonicalTitle)

		movieKey := fmt.Sprintf("%s|%d|%s|%d|%d", compactTitle, rel.Year, sequel, rel.Season, rel.Episode)

		b, exists := builders[movieKey]
		if !exists {
			canonicalID := fmt.Sprintf("cm_%x", len(builders)+1)
			b = &movieBuilder{
				movie: CanonicalMovie{
					CanonicalID:     canonicalID,
					CanonicalTitle:  canonicalTitle,
					NormalizedTitle: normalizedTitle,
					CompactTitle:    compactTitle,
					Year:            rel.Year,
					Sequel:          sequel,
					MediaType:       mediaType,
					Season:          rel.Season,
					Episode:         rel.Episode,
				},
				variants: make(map[string][]MediaFile),
			}
			builders[movieKey] = b
			movieOrder = append(movieOrder, movieKey)
		}

		// Detect file languages
		langs := DetectFileLanguages(file.FileName, file.Caption)
		primaryLang := "original"
		if len(langs) > 0 {
			primaryLang = langs[0]
		}
		b.variants[primaryLang] = append(b.variants[primaryLang], file)
	}

	var results []CanonicalMovie
	for _, key := range movieOrder {
		b := builders[key]

		// Build language variants
		var variants []LanguageVariant
		var variantKeys []string
		for langKey := range b.variants {
			variantKeys = append(variantKeys, langKey)
		}

		// Sort variants: requested language first, then alphabetically
		sort.Slice(variantKeys, func(i, j int) bool {
			ki := variantKeys[i]
			kj := variantKeys[j]
			if intent.Language != "" {
				if ki == intent.Language && kj != intent.Language {
					return true
				}
				if kj == intent.Language && ki != intent.Language {
					return false
				}
			}
			return ki < kj
		})

		totalFiles := 0
		for _, langKey := range variantKeys {
			vFiles := b.variants[langKey]
			// Sort files within variant: resolution descending, then size descending
			sort.SliceStable(vFiles, func(i, j int) bool {
				resI := DetectFileResolution(vFiles[i].FileName, vFiles[i].Caption)
				resJ := DetectFileResolution(vFiles[j].FileName, vFiles[j].Caption)
				rI := resolutionRank(resI)
				rJ := resolutionRank(resJ)
				if rI != rJ {
					return rI > rJ
				}
				return vFiles[i].FileSize > vFiles[j].FileSize
			})

			variants = append(variants, LanguageVariant{
				Language:           FormatLanguageDisplay(langKey),
				NormalizedLanguage: langKey,
				Files:              vFiles,
			})
			totalFiles += len(vFiles)
		}

		b.movie.LanguageVariants = variants
		b.movie.TotalFiles = totalFiles
		if len(variants) > 0 {
			b.movie.PrimaryLanguage = variants[0].Language
		}

		// Score computation for canonical movie
		score := 1.0
		if intent.Language != "" {
			hasRequestedLang := false
			for _, v := range variants {
				if v.NormalizedLanguage == intent.Language {
					hasRequestedLang = true
					break
				}
			}
			if !hasRequestedLang {
				score -= 0.10
			}
		}
		b.movie.Score = score

		results = append(results, b.movie)
	}

	// Sort canonical movies: score descending, total files descending
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		return results[i].TotalFiles > results[j].TotalFiles
	})

	return results
}

func parseYear(s string) int {
	var y int
	for _, r := range s {
		if r >= '0' && r <= '9' {
			y = y*10 + int(r-'0')
		}
	}
	return y
}
