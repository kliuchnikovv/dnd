package intent

import (
	"strings"
	"unicode"
)

// resolveByName ищет в фразе игрока имя того, кто есть в сцене.
//
// Это НЕ семантическое суждение, а поиск подстроки — та же дисциплина, что у
// применимости тегов по списку. Игрок почти всегда называет персонажа по
// имени, и просить за это модель — расточительство: имя либо в сцене есть,
// либо нет.
//
// Возвращает единственное совпадение. Два совпадения означают двусмысленность,
// и тогда пусть спрашивает.
func resolveByName(text string, list []Named) (string, bool) {
	lower := strings.ToLower(text)
	var found string
	for _, n := range list {
		if !mentions(lower, n.Name) {
			continue
		}
		if found != "" && found != n.ID {
			return "", false // двусмысленно
		}
		found = n.ID
	}
	return found, found != ""
}

// mentions сообщает, упомянуто ли имя. Сравниваются значимые слова имени:
// «Берн, стражник» даёт «берн» и «стражник», и любое из них считается
// упоминанием.
//
// Падежи покрываются общим префиксом, а не полным совпадением: «кузницу» и
// «кузница» расходятся в последней букве, «мальчишку» и «мальчишка» тоже.
// Правило — общий префикс должен покрывать слово целиком, кроме последней
// буквы. Этого хватает для русской инфлексии и мало для случайных созвучий:
// «страна» и «стражник» делят лишь четыре буквы из шести.
func mentions(lowerText, name string) bool {
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

// inflectionMatch — совпадение с точностью до окончания.
func inflectionMatch(a, b string) bool {
	ra, rb := []rune(a), []rune(b)
	shorter := len(ra)
	if len(rb) < shorter {
		shorter = len(rb)
	}
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
