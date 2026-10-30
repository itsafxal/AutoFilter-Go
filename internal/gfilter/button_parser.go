package gfilter

import (
	"regexp"
	"strings"

	"github.com/amarnathcjd/gogram/telegram"
)

var (
	// Regex matching markdown button syntax: [Text](url) or [Text](buttonurl:url) or [Text](buttondata:data)
	buttonRegex = regexp.MustCompile(`\[([^\]\n]+)\]\(((?:buttonurl:|buttondata:|url:|https?:\/\/)[^\)\n]+)\)`)
)

// ParseButtonsFromText parses inline button definitions from text and returns clean message text and button rows.
// Supports the standard Telegram bot :same modifier for placing buttons on the same row.
func ParseButtonsFromText(rawText string) (string, []ButtonRow) {
	if strings.TrimSpace(rawText) == "" {
		return rawText, nil
	}

	lines := strings.Split(rawText, "\n")
	var cleanLines []string
	var rows []ButtonRow

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		matches := buttonRegex.FindAllStringSubmatch(trimmed, -1)
		if len(matches) == 0 {
			cleanLines = append(cleanLines, line)
			continue
		}

		// Line contains button(s) - check if it also has regular text
		cleanedLine := buttonRegex.ReplaceAllString(trimmed, "")
		cleanedLine = strings.TrimSpace(cleanedLine)
		if cleanedLine != "" {
			cleanLines = append(cleanLines, cleanedLine)
		}

		var currentRow ButtonRow
		for _, match := range matches {
			if len(match) < 3 {
				continue
			}
			btnText := strings.TrimSpace(match[1])
			target := strings.TrimSpace(match[2])

			isSameRow := false
			if strings.HasSuffix(target, ":same") {
				isSameRow = true
				target = strings.TrimSuffix(target, ":same")
				target = strings.TrimSpace(target)
			}

			btn := InlineButton{Text: btnText}
			if strings.HasPrefix(target, "buttondata:") {
				btn.Data = strings.TrimPrefix(target, "buttondata:")
			} else {
				target = strings.TrimPrefix(target, "buttonurl:")
				target = strings.TrimPrefix(target, "url:")
				btn.URL = target
			}

			if isSameRow && len(rows) > 0 {
				rows[len(rows)-1] = append(rows[len(rows)-1], btn)
			} else if isSameRow && len(currentRow) > 0 {
				currentRow = append(currentRow, btn)
			} else {
				if len(currentRow) > 0 {
					rows = append(rows, currentRow)
					currentRow = nil
				}
				currentRow = append(currentRow, btn)
			}
		}
		if len(currentRow) > 0 {
			rows = append(rows, currentRow)
		}
	}

	cleanText := strings.TrimSpace(strings.Join(cleanLines, "\n"))
	return cleanText, rows
}

// ExtractButtonsFromMarkup extracts inline button rows from an existing Telegram ReplyMarkup object.
func ExtractButtonsFromMarkup(markup telegram.ReplyMarkup) []ButtonRow {
	if markup == nil {
		return nil
	}
	inlineMarkup, ok := markup.(*telegram.ReplyInlineMarkup)
	if !ok || inlineMarkup == nil || len(inlineMarkup.Rows) == 0 {
		return nil
	}

	var result []ButtonRow
	for _, row := range inlineMarkup.Rows {
		if row == nil || len(row.Buttons) == 0 {
			continue
		}
		var r ButtonRow
		for _, btn := range row.Buttons {
			switch b := btn.(type) {
			case *telegram.KeyboardButtonURL:
				r = append(r, InlineButton{Text: b.Text, URL: b.URL})
			case *telegram.KeyboardButtonCallback:
				r = append(r, InlineButton{Text: b.Text, Data: string(b.Data)})
			}
		}
		if len(r) > 0 {
			result = append(result, r)
		}
	}
	return result
}

// BuildReplyMarkup constructs a native MTProto *telegram.ReplyInlineMarkup from button rows.
func BuildReplyMarkup(buttons []ButtonRow) *telegram.ReplyInlineMarkup {
	if len(buttons) == 0 {
		return nil
	}
	builder := telegram.NewKeyboard()
	hasAny := false

	for _, row := range buttons {
		if len(row) == 0 {
			continue
		}
		var rowBtns []telegram.KeyboardButton
		for _, btn := range row {
			text := strings.TrimSpace(btn.Text)
			if text == "" {
				continue
			}
			if btn.URL != "" {
				rowBtns = append(rowBtns, telegram.Button.URL(text, btn.URL))
			} else if btn.Data != "" {
				rowBtns = append(rowBtns, telegram.Button.Data(text, btn.Data))
			}
		}
		if len(rowBtns) > 0 {
			builder.AddRow(rowBtns...)
			hasAny = true
		}
	}

	if !hasAny {
		return nil
	}
	return builder.Build()
}
