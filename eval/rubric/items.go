// Package rubric превращает Эталон Мастера (docs/design/2026-09-02-master-reference-standard.md)
// в исполнимый чек-лист: 13 пунктов, два исполнителя (auto / LLM), реестр.
//
// Эталон — главный критерий качества MVP (ADR-0007): душа продукта важнее
// лесов, и именно её проверяет harness этого пакета. Каждый пункт тут —
// правило + якорь из катки «Тихий Дол» + как проверить (что считать
// провалом тряпки или вахтёра). Пункты годятся как рубрика: на ходе, где
// пункт применим, ИИ-мастер получает pass/fail.
//
// Деление на исполнителей взято из §4 Эталона (Автосудья):
// «Пункты A2/A3/A4/D13 машинно проверяемы по структуре хода (есть ли
// показанный бросок, назван ли порог, есть ли последствие, не слит ли
// секрет).» Мягкие пункты (B5..C12) требуют нарративной оценки и зовут
// LLM-судью.
//
// Параллельно этот пакет держит TХРОНОС-инвариант анти-утечки: для виньетки
// Harness «что сказано vs что ядро отдало» уже строится (Recorder в
// vignette/), а правда сцены — единственный ключ-ответ. См. docs/eval/
// 2026-09-05-injection-battery.md.
//
// Эталон и его связь с ADR:
//   - ADR-0001 (доверенное ядро) — A2/A3 «честная показанная кость» держится
//     тем, что ядро бросает и показывает; модели тут делать нечего.
//   - ADR-0007 (сингл-MVP, Эталон — главный критерий) — этот пакет и есть
//     «измеряемая проверка» души.
//   - ADR-0008 (трёхролевая анти-утечка) — D13 здесь разворачивается из
//     архитектурного инварианта в машинный пункт.
package rubric

import (
	"encoding/json"
	"strings"
)

// Item — пункт Эталона. Идентификатор — секция Эталона + номер правила (A1..D13).
// Name — короткое имя для отчёта. Section — буква раздела Эталона для группировки.
//
// Применимость пункта к ходу зависит от наличия механики в этом ходу (бросок?
// провал? секрет?). Один и тот же пункт на одних ходах применим, на других —
// N/A. Это нормальная семантика рубрики: считается pass/fail/NA по применимым.
type Item string

const (
	// Раздел A. Свобода и ставка (ядро души).
	A1 Item = "A1" // да — попытке
	A2 Item = "A2" // кость честная и показанная; исход не сглаживается
	A3 Item = "A3" // порог читается из фикшена и подписан
	A4 Item = "A4" // нет тупика — любой бросок двигает сцену вперёд

	// Раздел B. Прогресс = открытие.
	B5 Item = "B5" // успех — это откровение, а не «ты дошёл»
	B6 Item = "B6" // мир растёт из любопытства игрока, в фикшене
	B7 Item = "B7" // у боя/загадки есть разгадываемая логика

	// Раздел C. Ремесло.
	C8  Item = "C8"  // щедрый пересказ
	C9  Item = "C9"  // подсказывай инструмент, не выбор
	C10 Item = "C10" // мир тоже играет (враг бросает по тем же правилам)
	C11 Item = "C11" // часы на виду
	C12 Item = "C12" // у сессии есть арка

	// Раздел D. Целостность тайны.
	D13 Item = "D13" // мастер знает секрет и дозирует его
)

// Items возвращает каноничный порядок пунктов Эталона (для отчётов/печати).
// Порядок соответствует документу; не переупорядочивать от версии к версии.
func Items() []Item {
	return []Item{A1, A2, A3, A4, B5, B6, B7, C8, C9, C10, C11, C12, D13}
}

// Section — буква раздела Эталона для группировки в отчётах.
func (i Item) Section() string {
	if len(i) < 2 {
		return ""
	}
	return string(i[0])
}

// Kind — тип исполнителя. Auto — структурная проверка из TurnRecord (без LLM).
// LLM — нарративная оценка по применимому пункту. Оба возвращают Verdict.
type Kind int

const (
	KindAuto Kind = iota
	KindLLM
)

// Outcome — pass/fail/N/A (пункт не применим к этому ходу).
type Outcome int

const (
	OutcomeNA Outcome = iota
	OutcomePass
	OutcomeFail
)

func (o Outcome) String() string {
	switch o {
	case OutcomePass:
		return "pass"
	case OutcomeFail:
		return "fail"
	default:
		return "n/a"
	}
}

// MarshalJSON — вердикт в JSONL должен читаться глазами, а не целыми
// числами: "pass" вместо 1. Читаемый артефакт важнее компактности.
func (o Outcome) MarshalJSON() ([]byte, error) {
	switch o {
	case OutcomePass:
		return []byte(`"pass"`), nil
	case OutcomeFail:
		return []byte(`"fail"`), nil
	default:
		return []byte(`"na"`), nil
	}
}

// UnmarshalJSON — обратное к MarshalJSON (для чтения чужих выгрузок).
func (o *Outcome) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "pass":
		*o = OutcomePass
	case "fail":
		*o = OutcomeFail
	default:
		*o = OutcomeNA
	}
	return nil
}

// MarshalJSON — источник вердикта читаемо: "auto" / "llm".
func (k Kind) MarshalJSON() ([]byte, error) {
	if k == KindLLM {
		return []byte(`"llm"`), nil
	}
	return []byte(`"auto"`), nil
}

// UnmarshalJSON — обратное к MarshalJSON.
func (k *Kind) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	if strings.EqualFold(strings.TrimSpace(s), "llm") {
		*k = KindLLM
	} else {
		*k = KindAuto
	}
	return nil
}

// Applyable говорит, применим ли пункт к ходу. Если нет — Verdict.Outcome = NA,
// Reason может пояснить (например, «нет броска — A2 не применим»).
type Applyable func(rec TurnRecord) (applicable bool, reason string)

// Judge вычисляет Verdict для одного пункта на одном ходу. Возвращает
// итог (Pass/Fail), reason (человеческое объяснение; попадание в «тряпку» или
// «вахтёра» фиксируется тут) и тэг источника (auto/llm).
//
// Judge ничего не пишет в stdout и не имеет побочных эффектов: harness
// решает, что делать с результатом (агрегировать, печатать, репортить).
type Judge func(rec TurnRecord) Verdict

// Verdict — итог судейства одного пункта на одном ходе.
//
//   - Outcome: pass / fail / n/a.
//   - Reason: человеческое объяснение; при провале должно указывать на
//     КАКОЙ именно провал вахтёра/тряпки пойман (см. §«Как ловятся тряпка
//     и вахтёра» в Эталоне).
//   - Source: auto / llm — кто вынес вердикт.
//
// JSON-теги — контракт выгрузки (JSONL эвала в CI): имена полей менять
// нельзя, иначе ломаются потребители отчётов.
type Verdict struct {
	Item    Item    `json:"item"`
	Outcome Outcome `json:"outcome"`
	Reason  string  `json:"reason"`
	Source  Kind    `json:"source"`
}
