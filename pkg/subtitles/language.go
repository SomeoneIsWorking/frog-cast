package subtitles

import (
	"regexp"
	"strings"

	"github.com/abadojack/whatlanggo"
)

// languageSample caps the text detection reads: whatlanggo's confidence breaks
// down on long input (Turkish reads as Azerbaijani past ~3000 bytes).
const languageSample = 2000

var markup = regexp.MustCompile(`<[^>]*>|\{[^}]*\}`)

// Language names the language of a WebVTT file's dialogue in English
// ("Turkish"), or "" when it can't be told reliably.
func Language(vttContent string) string {
	vtt, err := Parse(vttContent)
	if err != nil {
		return ""
	}
	var text strings.Builder
	for _, entry := range vtt.Entries {
		line := markup.ReplaceAllString(entry.Text, " ")
		if text.Len()+len(line) >= languageSample {
			break
		}
		text.WriteString(line)
		text.WriteByte('\n')
	}
	info := whatlanggo.Detect(text.String())
	if !info.IsReliable() {
		return ""
	}
	return info.Lang.String()
}
