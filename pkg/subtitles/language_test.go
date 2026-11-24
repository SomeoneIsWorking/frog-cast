package subtitles

import (
	"fmt"
	"strings"
	"testing"
)

func vttOf(lines ...string) string {
	var b strings.Builder
	b.WriteString("WEBVTT\n\n")
	for i, line := range lines {
		fmt.Fprintf(&b, "%d\n00:00:%02d.000 --> 00:00:%02d.500\n%s\n\n", i+1, i, i, line)
	}
	return b.String()
}

func TestLanguageNamesTheDialogue(t *testing.T) {
	cases := map[string]string{
		"Turkish": vttOf("Affedersiniz, affedersiniz.", "Hayır, kalkma. Burada kal.", "Adın ne senin?", "Bunu neden yaptığını bilmiyorum ama devam etmelisin."),
		"English": vttOf("I'm sorry. I'm sorry.", "No, stay. What's your name?", "I don't know why you did that, but you have to keep going."),
		"French":  vttOf("<i>Désolé...</i> Non, reste là.", "Comment tu t'appelles ?", "Je ne sais pas pourquoi tu as fait ça, mais tu dois continuer.", "Il faut travailler encore plus dur si tu veux jouer dans mon orchestre."),
	}
	for want, vtt := range cases {
		if got := Language(vtt); got != want {
			t.Errorf("Language = %q, want %q", got, want)
		}
	}
	if got := Language("WEBVTT\n"); got != "" {
		t.Errorf("empty file = %q, want none", got)
	}
}
