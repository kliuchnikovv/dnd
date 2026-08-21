package actor

import (
	"strings"

	"github.com/kliuchnikovv/dnd/naming"
)

// Act — что игрок сделал репликой. Классифицируется КОДОМ, а не моделью:
// «инструкция отвечать на сказанное» дважды дала перекос — сначала персонаж
// вываливал известное без повода, потом перестал делиться там, где его прямо
// приглашали. Связь «реплика игрока → уместный ход» должна быть проверяемой.
type Act string

const (
	ActGreeting Act = "greeting" // поздоровался
	ActProbe    Act = "probe"    // открытый вопрос: что слышно, что нового
	ActPress    Act = "press"    // давит: почему, при чём тут, объясни
	ActAsk      Act = "ask"      // спрашивает о чём-то конкретном
	ActThanks   Act = "thanks"   // благодарит
	ActThreat   Act = "threat"   // угрожает
	ActFarewell Act = "farewell" // прощается
	ActOther    Act = "other"    // прочее
)

// Ключевые слова по актам. Список открыт и заведомо неполон — это эвристика,
// а не разбор языка. Промах даёт ActOther, при котором доступны все ходы.
var actWords = []struct {
	act   Act
	words []string
}{
	{ActProbe, []string{"слышно", "нового", "новости", "слухи", "слухов", "происходит",
		"видел", "видели", "заметил", "рассказать", "расскажи", "интересного",
		"heard", "news", "rumour", "rumor", "anything", "happening"}},
	{ActPress, []string{"почему", "зачем", "объясни", "поясни",
		"при чем", "при чём", "причем", "причём", "как так", "в смысле",
		"why", "explain", "meaning"}},
	{ActGreeting, []string{"привет", "здравствуй", "здравствуйте", "поздороваться",
		"добрый", "доброго", "здорово", "hello", "good day", "greet"}},
	{ActThanks, []string{"спасибо", "благодарю", "признателен", "thanks", "thank you"}},
	{ActThreat, []string{"пожалеешь", "арестую", "заставлю", "хуже будет", "угрожаю",
		"arrest", "or else"}},
	{ActFarewell, []string{"прощай", "до свидания", "пока", "всего доброго",
		"goodbye", "farewell"}},
}

// Classify определяет акт по реплике игрока. Вопросительный знак без
// ключевых слов считается конкретным вопросом: игрок о чём-то спросил, пусть
// и непонятно о чём.
func Classify(text string) Act {
	lower := strings.ToLower(strings.TrimSpace(text))
	if lower == "" {
		return ActOther
	}
	for _, group := range actWords {
		for _, w := range group.words {
			if containsWord(lower, w) {
				return group.act
			}
		}
	}
	if strings.Contains(lower, "?") {
		return ActAsk
	}
	return ActOther
}

// containsWord ищет слово с поправкой на падеж, а не подстроку: «видел» не
// должно находиться в «видельщик», зато «слухов» обязано найтись по «слухи».
func containsWord(lowerText, word string) bool {
	if strings.Contains(word, " ") {
		return strings.Contains(lowerText, word)
	}
	return naming.Mentions(lowerText, word)
}

// movesFor — ходы, доступные при этом акте и этом материале.
//
// Ограничение набора и есть механизм: когда игрок прямо приглашает рассказать,
// светская болтовня о погоде просто отсутствует в грамматике, и «промолчать
// про три известные вещи» становится невозможным вариантом, а не выбором.
func movesFor(act Act, sit Situation) []string {
	hasNotes := len(sit.Talks) > 0
	hasFacts := len(sit.Known) > 0
	hasThreads := len(sit.Threads) > 0

	var allowed []Move
	switch act {
	case ActProbe:
		// Открытый вопрос — это приглашение. Если есть чем поделиться,
		// отмолчаться нельзя.
		if hasNotes {
			allowed = []Move{MoveVolunteer, MoveHint}
			if hasThreads {
				allowed = append(allowed, MoveRaiseThread)
			}
			break
		}
		allowed = []Move{MoveHint, MoveDeflect, MoveObserve, MoveAskBack}
	case ActGreeting:
		allowed = []Move{MoveSmalltalk, MoveObserve, MoveAskBack}
		if hasThreads {
			allowed = append(allowed, MoveRaiseThread)
		}
	case ActPress:
		allowed = []Move{MoveDeflect, MoveAskBack, MoveRefuse, MoveHint}
		if hasNotes {
			allowed = append(allowed, MoveVolunteer)
		}
	case ActThanks:
		allowed = []Move{MoveSmalltalk, MoveAskBack, MoveObserve}
	case ActThreat:
		allowed = []Move{MoveRefuse, MoveDeflect, MoveAskBack}
	case ActFarewell:
		allowed = []Move{MoveSmalltalk, MoveObserve}
		if hasThreads {
			allowed = append(allowed, MoveRaiseThread)
		}
	default:
		allowed = []Move{MoveDeflect, MoveAskBack, MoveObserve, MoveSmalltalk, MoveHint}
		if hasNotes {
			allowed = append(allowed, MoveVolunteer)
		}
	}

	if hasFacts {
		allowed = append(allowed, MoveConfirmKnown)
	}
	out := make([]string, 0, len(allowed))
	seen := map[Move]bool{}
	for _, m := range allowed {
		if !seen[m] {
			seen[m] = true
			out = append(out, string(m))
		}
	}
	return out
}

// actHint — как объяснить акт модели. Классификацию делает код, но модель
// должна понимать, на что отвечает.
func actHint(act Act) string {
	switch act {
	case ActProbe:
		return "открытый вопрос: игрок приглашает рассказать, что происходит"
	case ActGreeting:
		return "приветствие: ответить приветствием, не сводкой"
	case ActPress:
		return "давит и требует объяснений"
	case ActAsk:
		return "спрашивает о чём-то конкретном"
	case ActThanks:
		return "благодарит"
	case ActThreat:
		return "угрожает"
	case ActFarewell:
		return "прощается"
	default:
		return "говорит что-то, не требующее прямого ответа"
	}
}
