package rubric

// LLMJudge — интерфейс нарративной оценки. Пункты A1, B5, B6, B7, C8, C9,
// C10, C11, C12 требуют оценки «по смыслу», а не «по структуре». Здесь
// фиксируется контракт — реализация под конкретную модель живёт в адаптере
// (за рамками этого файла).
//
// Контракт:
//
//   - Один вызов — один Verdict по ОДНОМУ пункту. Несколько пунктов = несколько
//     вызовов. Это намеренно: формулировки пункта конкретны, и пакетная оценка
//     размывает рубрику.
//
//   - Детерминизм не требуется (модель шумит), но результат должен быть
//     воспроизводим ВНУТРИ сессии (тот же вход → та же оценка). Адаптер
//     ставит temperature=0 для судей.
//
//   - Возврат Verdict: Outcome/Reason обязательны; Verdict.Source должен быть
//     KindLLM.
//
//   - Ошибка адаптера (сетевая/прочие) возвращается как Verdict с
//     Reason="llm error: …" и Outcome=NA — чтобы падение модели не записывалось
//     как «прошёл» или «упал» (это отдельный сигнал: «не оценено»).

import "context"

type LLMJudge interface {
	Judge(ctx context.Context, item Item, rec TurnRecord) Verdict
}

// StubJudge — офлайн-судья для тестов/корпуса: возвращает N/A с пометкой.
// Подключение к реальной модели — отдельным адаптером.
//
// Нужен, чтобы test infra работала без ключа и без сети: harness применяет
// рубрику, видит «NA — нужен LLM-судья», и не считает пункт пройденным.
type StubJudge struct{}

// NewStubJudge — пустой судья. Полезен в тестах, которые проверяют «NA для
// мягких пунктов», и как zero-value по умолчанию.
func NewStubJudge() StubJudge { return StubJudge{} }

func (StubJudge) Judge(ctx context.Context, item Item, rec TurnRecord) Verdict {
	return Verdict{
		Item: item, Outcome: OutcomeNA,
		Reason: "нужен LLM-судья (StubJudge: офлайн-заглушка)",
		Source: KindLLM,
	}
}
