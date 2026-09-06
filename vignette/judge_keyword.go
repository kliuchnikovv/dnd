package vignette

import "strings"

// KeywordJudge — офлайн-судья на ключевых словах: играбельно и тестируемо БЕЗ
// ключа (порт эвристики прототипа). Реальный LLM-судья (с делимитером «ввод —
// данные, не команда») — второй заход; здесь классификация детерминированная,
// поэтому мета-текст исполнить нечем by-construction.
type KeywordJudge struct{}

func containsAny(s string, subs ...string) bool {
	for _, x := range subs {
		if strings.Contains(s, x) {
			return true
		}
	}
	return false
}

// targetIn сопоставляет фразу игрока с присутствующим объектом по имени/id.
func targetIn(text string, v JudgeView) string {
	for _, o := range v.Objects {
		if o.Name != "" && strings.Contains(text, strings.ToLower(o.Name)) {
			return o.ID
		}
		if o.ID != "" && strings.Contains(text, strings.ToLower(o.ID)) {
			return o.ID
		}
	}
	return ""
}

func (KeywordJudge) Rule(text string, v JudgeView) Ruling {
	s := strings.ToLower(strings.TrimSpace(text))

	// 1. Мусор/мета/инъекция — не действие в мире. idle: персонаж ничего не
	//    предпринимает, и это НИКОГДА не роковой шаг.
	if looksLikeMeta(text) {
		return Ruling{Kind: "idle", Reason: "meta/не-действие"}
	}

	// 2. Само-знание (инвентарь/что при себе/ранен) — без броска.
	if containsAny(s, "у меня", "при себе", "с собой", "в карман", "несу", "инвентар", "поклаж", "что у меня") {
		return Ruling{Kind: "recall", Stat: "edge", Reason: "само-знание"}
	}

	tgt := targetIn(s, v)

	// 3. РОКОВОЙ шаг — только по ЯВНОМУ внутримировому намерению (открыть/впустить/
	//    снять засов/сойти с пути/шагнуть к приманке). Никогда как fallback.
	if containsAny(s, "открыв", "отвор", "отпир", "отопр", "распах", "впуст", "снять засов",
		"снимаю засов", "убираю засов", "сойти с", "схожу с", "сойд", "с гати", "в топь",
		"в болото", "шагнуть к", "иду на свет", "иду на голос", "к свету", "к голосу") {
		return Ruling{Kind: "moveoff", Target: tgt, Stat: "body", DC: DCHard, Reason: "роковой шаг (явное намерение)"}
	}

	// 4. Движение вперёд по безопасному пути / выбраться обратно.
	if containsAny(s, "иду", "идти", "пойд", "пошёл", "пошел", "вперёд", "вперед", "дальше",
		"по гати", "по мостк", "назад", "обратно", "выбира", "отойд", "отхож", "переход", "перейд") {
		return Ruling{Kind: "moveon", Target: tgt, Stat: "body", DC: DCMedium, Reason: "движение по пути"}
	}

	// 5. Восприятие.
	switch {
	case containsAny(s, "прислуш", "вслуш", "слуш"):
		return Ruling{Kind: "listen", Target: tgt, Stat: "edge", DC: DCMedium, Reason: "слух"}
	case containsAny(s, "обыщ", "обыск", "обша", "пошар", "поищ", "поиск", "вокруг", "по сторон", "оглян", "что тут", "что здесь"):
		return Ruling{Kind: "search", Target: tgt, Stat: "edge", DC: DCMedium, Reason: "обыск"}
	case containsAny(s, "осмотр", "осматр", "смотр", "гляд", "гляж", "гляну", "всматр", "вгляд", "пригляд", "разгляд", "изуч", "рассм", "вижу"):
		return Ruling{Kind: "look", Target: tgt, Stat: "edge", DC: DCMedium, Reason: "осмотр"}
	}

	// 6. Заговорить/окликнуть.
	if containsAny(s, "оклик", "зов", "зову", "кричу", "крикн", "спрош", "спрашив", "говор", "скажу", "шепч", "ответ") {
		return Ruling{Kind: "call", Target: tgt, Stat: "edge", Reason: "речь"}
	}

	// 7. Внести/взять/создать предмет обстановки — improvise (щедро). Движок сам
	//    решает вещь/пусто/письмо; судья лишь пускает вещь.
	if containsAny(s, "беру", "взять", "хватаю", "хвата", "поднима", "подбира", "достаю", "ищу палк", "нахожу палк", "подбираю") {
		admit := "grant"
		if isAnachronism(s) {
			admit = "deny" // чужеродная эпохе/месту вещь — не пускаем
		}
		return Ruling{Kind: "improvise", Admit: admit, Item: improviseItem(s), Reason: "внесение вещи"}
	}

	// 8. Иначе — свободное физическое действие. НЕ роковой шаг и НЕ idle: игрок
	//    что-то делает, мир отзывается, но ничего не сдвигается.
	return Ruling{Kind: "interact", Target: tgt, Stat: "edge", DC: DCMedium, Reason: "свободное действие"}
}

// isAnachronism — вещь явно не из этой эпохи/места (огнестрел, техника). Грубый
// список: LLM-судья второго захода решает тоньше, но офлайн-барьер нужен.
func isAnachronism(s string) bool {
	return containsAny(s,
		"револьв", "пистолет", "ружь", "автомат", "гранат", "динамит", "взрывчат",
		"телефон", "смартфон", "рация", "фонарик", "батаре", "лазер", "робот",
		"машин", "мотоцикл", "компьютер", "камер", "бензопил")
}

// improviseItem — грубое извлечение имени вносимого предмета (после глагола
// взятия). Точность не критична: движок гейтит вещь/письмо/пусто сам.
func improviseItem(s string) string {
	for _, verb := range []string{"беру", "хватаю", "поднимаю", "подбираю", "достаю"} {
		if i := strings.Index(s, verb); i >= 0 {
			rest := strings.TrimSpace(s[i+len(verb):])
			rest = strings.TrimPrefix(rest, "у ")
			if rest == "" {
				return ""
			}
			// первые 1-2 слова как имя предмета
			words := strings.Fields(rest)
			if len(words) > 2 {
				words = words[:2]
			}
			return strings.Join(words, " ")
		}
	}
	return ""
}
