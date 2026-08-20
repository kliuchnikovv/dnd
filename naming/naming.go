// Package naming сопоставляет имена из текста игрока с именами в сцене.
//
// Это поиск подстроки с поправкой на падеж, а не семантическое суждение —
// та же дисциплина, что у применимости тегов по списку. Пакет не зависит ни
// от домена, ни от моделей, потому что нужен обоим входам: и разбору команд,
// и переводу свободного текста.
package naming

import (
	"strings"
	"unicode"
)

// Candidate — то, что может быть упомянуто: сущность, узел, что угодно с
// именем и идентификатором.
type Candidate struct {
	ID   string
	Name string
}

// Resolve возвращает единственного упомянутого кандидата. Два совпадения
// означают двусмысленность, и разрешать её догадкой нельзя.
func Resolve(text string, list []Candidate) (string, bool) {
	lower := strings.ToLower(text)
	var found string
	for _, c := range list {
		if !Mentions(lower, c.Name) {
			continue
		}
		if found != "" && found != c.ID {
			return "", false
		}
		found = c.ID
	}
	return found, found != ""
}

// Mentions сообщает, упомянуто ли имя. Сравниваются значимые слова имени:
// «Берн, стражник» даёт «берн» и «стражник», и любое из них считается
// упоминанием.
func Mentions(lowerText, name string) bool {
	words := splitWords(lowerText)
	for _, nameWord := range splitWords(strings.ToLower(name)) {
		if len([]rune(nameWord)) < 4 {
			continue // короткие слова дают ложные совпадения
		}
		for _, w := range words {
			if inflectionMatch(nameWord, w) {
				return true
			}
		}
	}
	return false
}

// inflectionMatch — совпадение с точностью до окончания. «кузницу» и
// «кузница» расходятся в последней букве, «мальчишку» и «мальчишка» тоже.
// Правило: общий префикс покрывает слово целиком, кроме последней буквы. Для
// случайных созвучий этого мало — «страна» и «стражник» делят четыре буквы
// из шести.
func inflectionMatch(a, b string) bool {
	ra, rb := []rune(a), []rune(b)
	shorter := min(len(ra), len(rb))
	if shorter < 4 {
		return false
	}
	common := 0
	for common < shorter && ra[common] == rb[common] {
		common++
	}
	return common >= shorter-1
}

func splitWords(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}
