package ask

import (
	"regexp"
	"strings"
)

var (
	wordRe       = regexp.MustCompile(`\p{L}+`)
	indonesianSW = set("yang dan di ke dari untuk dengan tidak apa kenapa mengapa bagaimana ini itu ada atau sudah belum juga pada oleh kapan siapa bisa harus akan karena sejak saja lagi perlu kalau jika tanpa masih apakah diminta berapa")
	englishSW    = set("the and of to for with not what why how this that is are was were does do did when who can should will because since which does has have been by from about")
)

// Language picks the answer's language from common Indonesian and English
// words; a tie uses the UI language (§10.5).
func Language(question, uiLocale string) string {
	id, en := 0, 0
	for _, w := range wordRe.FindAllString(strings.ToLower(question), -1) {
		if indonesianSW[w] {
			id++
		}
		if englishSW[w] {
			en++
		}
	}
	switch {
	case id > en:
		return "id"
	case en > id:
		return "en"
	case uiLocale == "en":
		return "en"
	}
	return "id"
}

func set(words string) map[string]bool {
	m := map[string]bool{}
	for _, w := range strings.Fields(words) {
		m[w] = true
	}
	return m
}
