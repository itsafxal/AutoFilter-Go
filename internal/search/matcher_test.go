package search

import (
	"testing"
	"time"
)

func TestParseRelease(t *testing.T) {
	tests := []struct {
		name        string
		filename    string
		query       string
		wantTitle   string
		wantYear    int
		wantSeason  int
		wantEpisode int
		wantChannel string
	}{
		{
			name:        "bracket channel with year and quality",
			filename:    "[CinemaCourt] Vikram (2022) 1080p WEB-DL x264 AAC.mkv",
			query:       "Vikram",
			wantTitle:   "Vikram",
			wantYear:    2022,
			wantChannel: "CinemaCourt",
		},
		{
			name:        "unbracketed channel prefix with year",
			filename:    "Cinemacourt Vikram 2022 1080p.mkv",
			query:       "Vikram",
			wantTitle:   "Vikram",
			wantYear:    2022,
			wantChannel: "Cinemacourt",
		},
		{
			name:        "delimiter channel prefix with language and quality",
			filename:    "CinemaCourt - Vikram 2022 Malayalam 720p HQ.mkv",
			query:       "Vikram",
			wantTitle:   "Vikram",
			wantYear:    2022,
			wantChannel: "CinemaCourt",
		},
		{
			name:        "unbracketed channel prefix with no year",
			filename:    "Cinemacourt Vikram.mkv",
			query:       "Vikram",
			wantTitle:   "Vikram",
			wantChannel: "Cinemacourt",
		},
		{
			name:        "two word unbracketed channel prefix",
			filename:    "Cinema Court Vikram 2022.mkv",
			query:       "Vikram",
			wantTitle:   "Vikram",
			wantYear:    2022,
			wantChannel: "Cinema Court",
		},
		{
			name:        "tv series with season and episode and character in episode name",
			filename:    "Hargadimaju Season 3 Episode 6 Vikram Thilak.mkv",
			query:       "Vikram",
			wantTitle:   "Hargadimaju",
			wantSeason:  3,
			wantEpisode: 6,
		},
		{
			name:      "vikramadithyan movie file",
			filename:  "Vikramadithyan (2014) Malayalam 720p HDRip.mkv",
			query:     "Vikram",
			wantTitle: "Vikramadithyan",
			wantYear:  2014,
		},
		{
			name:      "vikram vedha movie file",
			filename:  "Vikram Vedha (2017) Tamil 1080p.mkv",
			query:     "Vikram",
			wantTitle: "Vikram Vedha",
			wantYear:  2017,
		},
		{
			name:      "plain vikram file with dot separators",
			filename:  "Vikram.2022.Tamil.1080p.WEBRip.x264.mkv",
			query:     "Vikram",
			wantTitle: "Vikram",
			wantYear:  2022,
		},
		{
			name:      "kgf chapter 2",
			filename:  "KGF.Chapter.2.2022.1080p.mkv",
			query:     "kgf 2",
			wantTitle: "KGF Chapter 2",
			wantYear:  2022,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseRelease(tt.filename, tt.query)
			if got.CleanTitle != tt.wantTitle {
				t.Errorf("CleanTitle = %q, want %q", got.CleanTitle, tt.wantTitle)
			}
			if got.Year != tt.wantYear {
				t.Errorf("Year = %d, want %d", got.Year, tt.wantYear)
			}
			if got.Season != tt.wantSeason {
				t.Errorf("Season = %d, want %d", got.Season, tt.wantSeason)
			}
			if got.Episode != tt.wantEpisode {
				t.Errorf("Episode = %d, want %d", got.Episode, tt.wantEpisode)
			}
			if tt.wantChannel != "" && got.ChannelTag != tt.wantChannel {
				t.Errorf("ChannelTag = %q, want %q", got.ChannelTag, tt.wantChannel)
			}
		})
	}
}

func TestMatchMovie_StrictResults(t *testing.T) {
	tests := []struct {
		name      string
		query     string
		filename  string
		wantMatch bool
	}{
		// Search "Vikram" -> MUST match Vikram prints, even with channel tags
		{
			name:      "Vikram matches plain file",
			query:     "Vikram",
			filename:  "Vikram.2022.Tamil.1080p.mkv",
			wantMatch: true,
		},
		{
			name:      "Vikram matches [CinemaCourt] Vikram",
			query:     "Vikram",
			filename:  "[CinemaCourt] Vikram (2022) 1080p WEB-DL x264 AAC.mkv",
			wantMatch: true,
		},
		{
			name:      "Vikram matches unbracketed Cinemacourt Vikram",
			query:     "Vikram",
			filename:  "Cinemacourt Vikram 2022 1080p.mkv",
			wantMatch: true,
		},
		{
			name:      "Vikram matches Cinemacourt Vikram with no year",
			query:     "Vikram",
			filename:  "Cinemacourt Vikram.mkv",
			wantMatch: true,
		},
		{
			name:      "Vikram matches CinemaCourt - Vikram",
			query:     "Vikram",
			filename:  "CinemaCourt - Vikram 2022 Malayalam 720p HQ.mkv",
			wantMatch: true,
		},
		{
			name:      "Vikram matches Cinema Court Vikram",
			query:     "Vikram",
			filename:  "Cinema Court Vikram 2022.mkv",
			wantMatch: true,
		},

		// Search "Vikram" -> MUST REJECT non-matching movies and series
		{
			name:      "Vikram MUST REJECT Vikramadithyan",
			query:     "Vikram",
			filename:  "Vikramadithyan (2014) Malayalam 720p HDRip.mkv",
			wantMatch: false,
		},
		{
			name:      "Vikram MUST REJECT Vikram Vedha",
			query:     "Vikram",
			filename:  "Vikram Vedha (2017) Tamil 1080p.mkv",
			wantMatch: false,
		},
		{
			name:      "Vikram MUST REJECT Hargadimaju Season 3 Episode 6 Vikram Thilak",
			query:     "Vikram",
			filename:  "Hargadimaju Season 3 Episode 6 Vikram Thilak.mkv",
			wantMatch: false,
		},
		{
			name:      "Vikram MUST REJECT Vikramarkudu",
			query:     "Vikram",
			filename:  "Vikramarkudu (2006) Telugu.mkv",
			wantMatch: false,
		},

		// Search "Vikram Vedha" -> MUST MATCH Vikram Vedha and REJECT plain Vikram
		{
			name:      "Vikram Vedha matches Vikram Vedha",
			query:     "Vikram Vedha",
			filename:  "Vikram Vedha (2017) Tamil 1080p.mkv",
			wantMatch: true,
		},
		{
			name:      "Vikram Vedha REJECTS plain Vikram",
			query:     "Vikram Vedha",
			filename:  "Vikram (2022) 1080p.mkv",
			wantMatch: false,
		},
		{
			name:      "Vikram Vedha REJECTS Vikramadithyan",
			query:     "Vikram Vedha",
			filename:  "Vikramadithyan (2014).mkv",
			wantMatch: false,
		},

		// Search "Vikramadithyan" -> matches Vikramadithyan
		{
			name:      "Vikramadithyan matches Vikramadithyan",
			query:     "Vikramadithyan",
			filename:  "Vikramadithyan (2014) Malayalam 720p HDRip.mkv",
			wantMatch: true,
		},
		{
			name:      "Vikramadithyan REJECTS Vikram",
			query:     "Vikramadithyan",
			filename:  "Vikram (2022).mkv",
			wantMatch: false,
		},

		// Search "Hargadimaju" -> matches Hargadimaju series episode
		{
			name:      "Hargadimaju matches Hargadimaju episode",
			query:     "Hargadimaju",
			filename:  "Hargadimaju Season 3 Episode 6 Vikram Thilak.mkv",
			wantMatch: true,
		},
		{
			name:      "Hargadimaju S3 matches Hargadimaju season 3",
			query:     "Hargadimaju S3",
			filename:  "Hargadimaju Season 3 Episode 6 Vikram Thilak.mkv",
			wantMatch: true,
		},

		// Year filtering
		{
			name:      "Vikram 2022 matches 2022 file",
			query:     "Vikram 2022",
			filename:  "Vikram (2022) 1080p.mkv",
			wantMatch: true,
		},
		{
			name:      "Vikram 2022 rejects 1986 file",
			query:     "Vikram 2022",
			filename:  "Vikram (1986) 720p.mkv",
			wantMatch: false,
		},

		// Franchise & sequel
		{
			name:      "KGF matches KGF Chapter 1",
			query:     "KGF",
			filename:  "KGF.Chapter.1.2018.1080p.mkv",
			wantMatch: true,
		},
		{
			name:      "KGF matches KGF Chapter 2",
			query:     "KGF",
			filename:  "KGF.Chapter.2.2022.1080p.mkv",
			wantMatch: true,
		},
		{
			name:      "KGF 2 matches KGF Chapter 2",
			query:     "KGF 2",
			filename:  "KGF.Chapter.2.2022.1080p.mkv",
			wantMatch: true,
		},
		{
			name:      "KGF 2 rejects KGF Chapter 1",
			query:     "KGF 2",
			filename:  "KGF.Chapter.1.2018.1080p.mkv",
			wantMatch: false,
		},

		// Malayalam transliteration
		{
			name:      "Malayalam script വിക്രം matches Vikram",
			query:     "വിക്രം",
			filename:  "[CinemaCourt] Vikram (2022) 1080p.mkv",
			wantMatch: true,
		},
		{
			name:      "Malayalam script വിക്രം rejects Vikramadithyan",
			query:     "വിക്രം",
			filename:  "Vikramadithyan (2014).mkv",
			wantMatch: false,
		},
		{
			name:      "The Early Spring matches episode file",
			query:     "The Early Spring",
			filename:  "The Early Spring S01E12 480p NF WEB DL Chinese AAC -@MNTG",
			wantMatch: true,
		},
		{
			name:      "The Early Spring matches episode file with hyphen before season",
			query:     "The Early Spring",
			filename:  "The Early Spring - S01E12 480p NF WEB DL Chinese AAC -@MNTG",
			wantMatch: true,
		},
		{
			name:      "The Early Spring matches episode file with trailing handle",
			query:     "The Early Spring",
			filename:  "The Early Spring S01E12 480p - @MNTG",
			wantMatch: true,
		},
		{
			name:      "The Early Spring S1 matches episode file",
			query:     "The Early Spring S1",
			filename:  "The Early Spring S01E12 480p NF WEB DL Chinese AAC -@MNTG",
			wantMatch: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rel := ParseRelease(tt.filename, tt.query)
			gotMatch, _ := MatchMovie(tt.query, rel)
			if gotMatch != tt.wantMatch {
				t.Errorf("MatchMovie(%q, %q) = %v, want %v (parsed title: %q, year: %d, season: %d, ep: %d)",
					tt.query, tt.filename, gotMatch, tt.wantMatch, rel.CleanTitle, rel.Year, rel.Season, rel.Episode)
			}
		})
	}
}

func TestFilterAndRankStrict(t *testing.T) {
	now := time.Now()
	records := []MediaFile{
		{FileID: "1", FileName: "Vikramadithyan (2014) Malayalam 720p HDRip.mkv", DateSaved: now.Add(-1 * time.Hour)},
		{FileID: "2", FileName: "Vikram Vedha (2017) Tamil 1080p.mkv", DateSaved: now.Add(-2 * time.Hour)},
		{FileID: "3", FileName: "Hargadimaju Season 3 Episode 6 Vikram Thilak.mkv", DateSaved: now.Add(-3 * time.Hour)},
		{FileID: "4", FileName: "[CinemaCourt] Vikram (2022) 1080p WEB-DL.mkv", DateSaved: now.Add(-4 * time.Hour)},
		{FileID: "5", FileName: "Cinemacourt Vikram 2022 720p.mkv", DateSaved: now.Add(-5 * time.Hour)},
		{FileID: "6", FileName: "Vikram.2022.Tamil.1080p.mkv", DateSaved: now.Add(-30 * time.Minute)},
	}

	filtered := FilterAndRankStrict("Vikram", records)

	// Should only contain the 3 genuine Vikram files (IDs 6, 4, 5)
	if len(filtered) != 3 {
		t.Fatalf("expected 3 files, got %d", len(filtered))
	}

	expectedIDs := []string{"6", "4", "5"}
	for i, f := range filtered {
		if f.FileID != expectedIDs[i] {
			t.Errorf("at index %d: expected FileID %q, got %q (%s)", i, expectedIDs[i], f.FileID, f.FileName)
		}
	}

	// Series search: "The Early Spring" matches episodes
	seriesRecords := []MediaFile{
		{FileID: "11", FileName: "The Early Spring S01E11 480p NF WEB DL Chinese AAC -@MNTG", DateSaved: now.Add(-2 * time.Hour)},
		{FileID: "12", FileName: "The Early Spring S01E12 480p NF WEB DL Chinese AAC -@MNTG", DateSaved: now.Add(-1 * time.Hour)},
		{FileID: "99", FileName: "Spring Blossom 2021 720p.mkv", DateSaved: now},
	}
	seriesFiltered := FilterAndRankStrict("The Early Spring", seriesRecords)
	if len(seriesFiltered) != 2 {
		t.Fatalf("expected 2 episodes matching 'The Early Spring', got %d", len(seriesFiltered))
	}
	if seriesFiltered[0].FileID != "12" || seriesFiltered[1].FileID != "11" {
		t.Errorf("expected episodes 12 and 11, got %v and %v", seriesFiltered[0].FileID, seriesFiltered[1].FileID)
	}
}
