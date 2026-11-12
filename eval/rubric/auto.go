package rubric

import (
	"fmt"
	"strings"
)

// AutoJudge вычисляет пункты A2, A3, A4, D13 по СТРУКТУРЕ хода (без LLM).
// Это четыре пункта, которые §4 Эталона называет «машинно проверяемыми»:
// есть ли показанный бросок, назван ли порог, есть ли последствие, не
// слит ли секрет.
//
// Контракт: судья ничего не пишет в stdout и не имеет побочных эффектов.
// Все эвристики — эвристики; несовершенство фиксируется в Reason при провале
// («бросок не показан», «порог не назван», «на провале нет последствия»,
// «секрет сцены в прозе до заслуги»).
//
// Чего AutoJudge НЕ делает:
//   - A1 (да — попытке): требует нарративной оценки, чтобы понять «приземлил
//     заявку или отбил». Оставлено LLM-судье.
//   - B5..C12: нарративные; нужен LLM-судья.
//
// Применимость определяется по структуре записи (HasRoll/Ended и т. д.), а
// не по эвристике «может быть»: см. Applyable для каждого правила.

// ─── A2. Кость честная и показанная. Исход не сглаживается.
//
// Эталон:
//   [ ] Бросок показан игроку (термы, порог, маржа, исход).
//   [ ] Плохой бросок даёт плохой исход; хороший ход на плохой кости всё равно
//       может провалиться.
// Провал тряпки: красивая заявка → автоуспех без броска или с «подкрученным»
// броском.
//
// Авто-проверка по двум подпунктам:
//   A2.a: MasterOutput упоминает бросок — числа d20, маржи, total/DC.
//   A2.b: Исход согласован с маржой (на сильном провале нет «всё вышло»,
//         на nat-20 — нет «промах»).

// rollTermsLoose — ослабленный матч «броска в прозе». Ловит и явные строки
// («бросок: d20=17+2 = 19 против 14 · +5»), и человеческие пересказы
// («кубик лёг удачно, выпало 17», «d20=nat-1»), и просто число рядом с
// «бросок/кубик/кость/d20». Если ничего из этого — провал A2.a.
//
// Особенность: regexp в Go (RE2) не умеет Unicode \b даже с флагом (?U), а
// в русской прозе часто нужны границы «кириллица|пробел». Поэтому проверка
// делается через strings.Contains + helpers для шаблонов с цифрами.
var rollMarkers = []string{"d20", "кубик", "кость", "бросок", "roll", "nat-1", "nat-20", "нат-1", "нат-20"}

// dieMarkers — маркеры «Мастер говорит о кости». Нужны для ходов без броска,
// где Мастер ещё только устанавливает доверие к кости («бросаю в терминале»):
// там A2 применим и проверяет, что кость показана как честная. В Эталоне на
// старте тег A2 стоит как раз за это.
var dieMarkers = []string{"брос", "кост", "d20", "кубик", "терминал", "честн", "видиш", "покажу"}

// judgeRollShown — A2.a: бросок показан игроку. Применим, если в ходе был
// бросок (HasRoll) ИЛИ Мастер говорил о кости (ExpectsDie).
//
//   - при броске: ищем маркер броска ИЛИ формат «число+число»;
//   - без броска, но с разговором о кости: ищем маркер кости.
//
// без строгих \b (RE2 не умеет Unicode), чтобы кириллица работала естественно.
func judgeRollShown() Judge {
	return func(rec TurnRecord) Verdict {
		text := strings.ToLower(rec.MasterOutput)
		if !rec.HasRoll() {
			if !rec.ExpectsDie {
				return Verdict{Item: A2, Outcome: OutcomeNA, Reason: "нет броска и разговора о кости — A2.a не применим", Source: KindAuto}
			}
			for _, m := range dieMarkers {
				if strings.Contains(text, m) {
					return Verdict{Item: A2, Outcome: OutcomePass, Source: KindAuto}
				}
			}
			return Verdict{Item: A2, Outcome: OutcomeFail, Reason: "Мастер не показал кость игроку, хотя обещал её честность", Source: KindAuto}
		}
		for _, m := range rollMarkers {
			if strings.Contains(text, m) {
				return Verdict{Item: A2, Outcome: OutcomePass, Source: KindAuto}
			}
		}
		if hasNumberPlusNumber(text) {
			return Verdict{Item: A2, Outcome: OutcomePass, Source: KindAuto}
		}
		return Verdict{Item: A2, Outcome: OutcomeFail, Reason: "бросок не показан игроку в прозе хода", Source: KindAuto}
	}
}

// hasNumberPlusNumber — ищет в тексте образ «число +/число» (типичный roll:
// d20=15+2). Это ловит «15+2», «17+5», «на 12+3», но не «15.2» или «15,2»
// благодаря пробелам вокруг +-.
func hasNumberPlusNumber(text string) bool {
	// Соседние цифры с + или - между ними. Минимум 1 цифра с каждой стороны.
	for i := 0; i < len(text)-2; i++ {
		if (text[i] == '+' || text[i] == '-') && i > 0 && i < len(text)-1 {
			if isDigit(text[i-1]) && isDigit(text[i+1]) {
				return true
			}
		}
	}
	return false
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }

// outcomeWordsFail — слова, маркирующие честный провал в прозе. Для A2.b
// ищем хотя бы одно — на сильном провале (IsTotalFailure). Если на nat-1
// Мастер пишет «всё получилось» — это тряпка.
var outcomeWordsFail = []string{
	"провалил", "провал", "не вышло", "мимо", "не получи",
	"плачевно", "дорого", "потерял", "потеря", "цена", "кровь",
	"упустил", "упущение", "нат-1", "nat 1", "nat-1",
}
var outcomeWordsWinStrong = []string{
	"чисто", "блестяще", "успех", "вышло", "получилос", "получи",
	"в цель", "точно", "верно", "нат-20", "nat 20", "nat-20",
}

// judgeRollHonest — A2.b: исход согласован с броском. На сильном провале
// должна быть «плохая» лексика, на nat-20 — «хорошая». На нейтральном
// исходе (маржа 0..4) — пункту всё равно (N/A).
func judgeRollHonest() Judge {
	return func(rec TurnRecord) Verdict {
		if !rec.HasRoll() {
			return Verdict{Item: A2, Outcome: OutcomeNA, Reason: "нет броска — A2.b не применим", Source: KindAuto}
		}
		// Натуральный 20 на d20 — должно быть «блестяще». Если в прозе ни
		// одного «победного» слова — подозрение на сглаживание (тряпка).
		if rec.Roll.Natural == 20 {
			if matchAny(rec.MasterOutput, outcomeWordsWinStrong) {
				return Verdict{Item: A2, Outcome: OutcomePass, Source: KindAuto}
			}
			return Verdict{Item: A2, Outcome: OutcomeFail, Reason: "нат-20 без честной победы — возможно сглаживание (тряпка)", Source: KindAuto}
		}
		// Натуральный 1 или маржа <= -5 — плохой исход обязателен.
		if rec.Roll.Natural == 1 || rec.Roll.Margin <= -5 {
			if matchAny(rec.MasterOutput, outcomeWordsFail) {
				return Verdict{Item: A2, Outcome: OutcomePass, Source: KindAuto}
			}
			return Verdict{Item: A2, Outcome: OutcomeFail, Reason: "на плохом броске нет плохого исхода — тряпка?", Source: KindAuto}
		}
		return Verdict{Item: A2, Outcome: OutcomeNA, Reason: "средняя маржа, A2.b не решает", Source: KindAuto}
	}
}

// ─── A3. Порог читается из фикшена и подписан.
//
// Эталон:
//   [ ] Сложность выведена из позиции/ситуации, а не из настроения.
//   [ ] Порог и модификаторы названы, логика видна игроку.
// Провал: порог произвольный или скрытый (игрок не понимает, почему трудно).
//
// Авто-проверка по одному подпункту (A3.a, т. к. A3.b — нарратив):
//   Если в ходе был бросок с известным DC, проза упоминает DC явно
//   («DC 14», «Норма 14», «против 14», «сложно/средне/легко»). Если нет —
//   провал A3.

// dcMarkers — упоминания DC в прозе (рус/англ канон + категории). Для слов
// «против/сложность/DC» мы дополнительно требуем рядом число — иначе фразы
// вроде «против натиска» без чисел не считаются за DC.
var dcMarkers = []string{"dc ", "d.c.", "сложност", "легко", "норма", "трудно", "easy", "medium", "hard"}
var dcPhrasesWithNumber = []string{"против", "difficulty", "check dc"}

// judgeDCStated — A3: порог назван/подписан. Смотрит на одну из:
//   - слово-маркер категории (легко/норма/трудно/easy/medium/hard/DC);
//   - «против N» / «сложность N» / «difficulty N» — рядом с числом.
//
// Без строгих \b (RE2 не умеет Unicode-границы); вместо этого смотрим
// подстроку против lower-cased текста. Это пере-ловит «против натиска»
// без числа — и это правильно: фразы вроде «против дракона» без явного
// порога скрывают сложность.
func judgeDCStated() Judge {
	return func(rec TurnRecord) Verdict {
		if !rec.HasRoll() {
			return Verdict{Item: A3, Outcome: OutcomeNA, Reason: "нет броска — A3 не применим", Source: KindAuto}
		}
		text := strings.ToLower(rec.MasterOutput)
		for _, m := range dcMarkers {
			if strings.Contains(text, m) {
				return Verdict{Item: A3, Outcome: OutcomePass, Source: KindAuto}
			}
		}
		for _, p := range dcPhrasesWithNumber {
			if strings.Contains(text, p) && hasNearbyNumber(text, p) {
				return Verdict{Item: A3, Outcome: OutcomePass, Source: KindAuto}
			}
		}
		return Verdict{Item: A3, Outcome: OutcomeFail, Reason: "порог не назван/не подписан — DC скрыт", Source: KindAuto}
	}
}

// hasNearbyNumber — есть ли в окне ±6 символов после `phrase` хотя бы одна
// цифра. Грубая мера «фраза упомянута с числом рядом»; для авто-судьи этого
// достаточно, а LLM-судья проверит семантику.
func hasNearbyNumber(text, phrase string) bool {
	i := strings.Index(text, phrase)
	if i < 0 {
		return false
	}
	end := i + len(phrase) + 6
	if end > len(text) {
		end = len(text)
	}
	for _, c := range text[i+len(phrase) : end] {
		if c >= '0' && c <= '9' {
			return true
		}
	}
	return false
}

// ─── A4. Нет тупика — любой бросок двигает сцену вперёд (fail-forward).
//
// Эталон:
//   [ ] Даже полный провал рождает новый фикшен/новую проблему, а не стоп.
//   [ ] Частичный успех (маржа −1..−4) = получил желаемое + цена.
// Провал: «мимо, ход впустую» без последствия и движения.
//
// Авто-проверка:
//   A4.a (полный провал): даже на IsTotalFailure в прозе или state есть
//         «что-то новое» — изменившееся положение/beat/стоимость.
//   A4.b (частичный успех): цена показана (любое из: ранение/потеря/состояние).

func judgeFailForward() Judge {
	return func(rec TurnRecord) Verdict {
		if !rec.HasRoll() {
			return Verdict{Item: A4, Outcome: OutcomeNA, Reason: "нет броска — A4 не применим", Source: KindAuto}
		}
		switch {
		case rec.IsTotalFailure():
			// Полный провал: сцена должна сдвинуться. Структурные сигналы
			// (revelation/beat/ended) ИЛИ движение в прозе.
			if stateChanged(rec) || hasCost(rec.MasterOutput) || hasMotion(rec.MasterOutput) {
				return Verdict{Item: A4, Outcome: OutcomePass, Source: KindAuto}
			}
			return Verdict{Item: A4, Outcome: OutcomeFail, Reason: "полный провал не породил новой проблемы/движения", Source: KindAuto}
		case rec.IsPartialSuccess():
			// Частичный успех: получи желаемое + цена. Цена обязательна;
			// одного «движения» мало (это просто «что-то случилось»).
			if hasCost(rec.MasterOutput) || stateChanged(rec) {
				return Verdict{Item: A4, Outcome: OutcomePass, Source: KindAuto}
			}
			return Verdict{Item: A4, Outcome: OutcomeFail, Reason: "частичный успех без цены — fail-forward не доехал", Source: KindAuto}
		default:
			return Verdict{Item: A4, Outcome: OutcomeNA, Reason: "средняя маржа, A4 не решает", Source: KindAuto}
		}
	}
}

// stateChanged — даёт ли запись признак движения (положение/бит/state).
// Это структурный аналог «новая проблема»: сменилось то, что игрок видит.
//
// ВНИМАНИЕ: StateNote НЕ считается за forward motion. StateNote обновляется
// механически на каждом ходе (`position`/`inventory`) и его наличие не
// говорит, что сцена сдвинулась. Forward motion — это либо открылось новое
// знание (len(Revealed)>0), либо случился эскалационный бит (Beat), либо
// сцена закрылась. «Ничего нового» = тупик, и A4.a это ловит.
func stateChanged(rec TurnRecord) bool {
	return len(rec.Revealed) > 0 || rec.Beat != "" || (rec.Ended && rec.EndText != "")
}

// costLex — лексика цены в прозе (рана/шрам/кровь/расход/цена/удача/потеря/
// шрам/ближе к смерти/и т. п.). Грубо, но покрывает большинство живых
// формулировок; тонкую работу делает LLM-судья (поставлено за рамками этого
// файла, чтобы AutoJudge не зависел от модели).
var costLex = []string{
	"ранен", "рана", "ран", "кровь", "крови", "шрам", "потерял",
	"потеря", "расход", "ценой", "дорог", "платить", "заплатил",
	"штраф", "усталос", "ушиб", "больно", "ближе к смерт",
	"потерян", "без ножа", "без верёвк", "без ",
}

// motionLex — шире, чем costLex: «что-то произошло» на любом языке. Ловит
// «скрипнула», «слышно», «видно», «темнеет», «зашевелилось», «новое», «теперь»
// — то есть вперёд-двигающиеся сигналы в прозе. Используется в A4.a как
// дополнительный к структурным сигналам способ сказать «сцена сдвинулась».
var motionLex = []string{
	"скрип", "слышн", "видн", "темне", "зашевел", "новое", "новое ",
	"теперь", "дальше", "вдруг", "затем", "появл", "двига", "приближа",
	"отступ", "пятн", "тень", "хруст", "стук", "звук", "запах", "холод",
	"тепл", "мокр", "сух", "крепче", "сильнее", "слабее", "больше", "меньше",
	"раньше", "позже", "затем ", "после ", "прежде",
}

func hasCost(text string) bool   { return matchAny(text, costLex) }
func hasMotion(text string) bool { return matchAny(text, motionLex) }

// ─── D13. Мастер знает секрет и дозирует его.
//
// Эталон:
//   [ ] Ключевая тайна не выдаётся прозой/подсказкой раньше, чем её заслужили.
//   [ ] Раскрытие привязано к действиям игрока и драматическим битам.
// Провал: Мастер называет разгадку/уязвимость до того, как игрок к ней пришёл.
//
// Авто-проверка: в MasterOutput НЕТ ни одной фразы из SceneTruth / закрытых
// тиров, если они НЕ в Revealed этого хода (т. е. если заслуги не было).
//
// Простая эвристика для текста: режем прозу на предложения и для каждого
// предложения проверяем, не содержит ли оно длинной (>5 символов) фразы из
// ключ-ответа. Точная «семантическая» сверка оставлена LLM-судье; эвристика
// ловит дословные утечки и подсказки-формулировки.

const minLeakPhraseLen = 8

func judgeNoEarlySecret() Judge {
	return func(rec TurnRecord) Verdict {
		if rec.SceneTruth == "" && len(rec.ClosedTiers) == 0 {
			return Verdict{Item: D13, Outcome: OutcomeNA, Reason: "нет ключ-ответа сцены — D13 не проверяем", Source: KindAuto}
		}
		leak := findLeak(rec)
		if leak != "" {
			return Verdict{
				Item: D13, Outcome: OutcomeFail, Source: KindAuto,
				Reason: "секрет сцены в прозе до заслуги: " + truncate(leak, 120),
			}
		}
		return Verdict{Item: D13, Outcome: OutcomePass, Source: KindAuto}
	}
}

// findLeak — ищет в MasterOutput фразу из SceneTruth или ClosedTiers, которой
// нет в Revealed этого хода. Возвращает первое совпадение или пустую строку.
//
// Match по токенам: режем ключ-ответ на фразы длиной не меньше minLeakPhraseLen;
// для каждой фразы проверяем вхождение в MasterOutput как подстроки (lowercased).
// Это намеренно грубо: AutoJudge ловит ДОСЛОВНЫЕ утечки и утечки через
// перефразирование ОБЩИХ слов («это человек», «это нечисть»). Мелкий
// пересказ своими словами остаётся на долю LLM-судьи.
func findLeak(rec TurnRecord) string {
	out := strings.ToLower(rec.MasterOutput)
	for _, phrase := range leakPhrases(rec.SceneTruth) {
		if phrase == "" {
			continue
		}
		if strings.Contains(out, phrase) && !revealedContains(rec.Revealed, phrase) {
			return phrase
		}
	}
	for _, tier := range rec.ClosedTiers {
		for _, phrase := range leakPhrases(tier) {
			if phrase == "" {
				continue
			}
			if strings.Contains(out, phrase) && !revealedContains(rec.Revealed, phrase) {
				return phrase
			}
		}
	}
	return ""
}

// leakPhrases — набор фраз-кандидатов на утечку из одного ключ-ответа.
// Берём само предложение целиком + все его подстроки длиннее minLeakPhraseLen
// по границам слов. Это пере-генерирует эвристику «всё, что звучит как
// ответ». Дубли шумные, но AutoJudge работает по O(N²) на коротких строках;
// на типичном ходе (<= 200 слов) это мгновенно.
func leakPhrases(key string) []string {
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" {
		return nil
	}
	var out []string
	// Целиком — самое длинное.
	out = append(out, key)
	// По знакам препинания.
	for _, sep := range []string{". ", "! ", "? ", "; "} {
		for _, p := range strings.Split(key, sep) {
			p = strings.TrimSpace(p)
			if len([]rune(p)) >= minLeakPhraseLen {
				out = append(out, p)
			}
		}
	}
	// По словам (>= 2 слов в окне).
	words := strings.Fields(key)
	for i := 0; i < len(words); i++ {
		for j := i + 2; j <= len(words); j++ {
			phrase := strings.Join(words[i:j], " ")
			if len([]rune(phrase)) >= minLeakPhraseLen {
				out = append(out, phrase)
			}
		}
	}
	return out
}

func revealedContains(revealed []string, phrase string) bool {
	for _, line := range revealed {
		if strings.Contains(strings.ToLower(line), phrase) {
			return true
		}
	}
	return false
}

// ─── C10. Мир тоже играет ─────────────────────────────────────────
//
// Эталон: «Противник играет по той же честной кости — со своими числами и
// своим весом. Ты видишь его бросок так же, как свой.» Провал: мир за спиной
// решает, бьёт он или нет.
//
// Структурно проверяемо: если в ходе был бросок мира (NpcRolled), его числа
// должны быть ВИДНЫ игроку — в прозе или в пересказе хода. Если бросока мира
// не было, пункт неприменим: судить не о чем.

func judgeWorldRolls() Judge {
	return func(rec TurnRecord) Verdict {
		if !rec.NpcRolled {
			return Verdict{Item: C10, Outcome: OutcomeNA, Reason: "мир в ходе не бросал — C10 не применим", Source: KindAuto}
		}
		text := strings.ToLower(rec.MasterOutput)
		// Бросок мира виден, если проза упоминает его d20 или само число.
		seen := false
		if rec.NpcRoll.D20 > 0 {
			needle := fmt.Sprintf("d20=%d", rec.NpcRoll.D20)
			if strings.Contains(text, strings.ToLower(needle)) {
				seen = true
			}
		}
		if !seen {
			// «оно: 20», «бросок мира: 20» и т. п.
			for _, marker := range []string{"оно:", "мир бросил", "бросок мира", "противник бросил", "оно бросает", "оно бросило"} {
				if strings.Contains(text, marker) {
					seen = true
					break
				}
			}
		}
		if seen {
			return Verdict{Item: C10, Outcome: OutcomePass, Source: KindAuto}
		}
		return Verdict{
			Item: C10, Outcome: OutcomeFail, Source: KindAuto,
			Reason: "бросок мира не показан игроку — мир решил за спиной",
		}
	}
}

// ─── Вспомогательное ─────────────────────────────────────────────

func matchAny(text string, words []string) bool {
	t := strings.ToLower(text)
	for _, w := range words {
		if strings.Contains(t, strings.ToLower(w)) {
			return true
		}
	}
	return false
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
