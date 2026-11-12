package rubric

import (
	"context"
	"sync"
)

// Rubric — реестр судей по каждому пункту Эталона. Apply прогоняет запись
// через всех судей применимых к этому ходу и возвращает набор Verdict.
//
// Конструкция — намеренно «честный словарь»: одна и та же Verdict возвращается
// всегда (нет случайного порядка), и любой пункт можно опросить точечно
// (Rubric.Judge(Item, rec)). Это упрощает:
//   - выборочные проверки в репорте («покажи мне все D13 по этому корпусу»);
//   - сравнение моделей (дай под каждую модель свой LLMJudge — и Apply тот же);
//   - тесты на изоляцию (подменил StubJudge — и весь LLM-слой стоит на NA).
//
// Применимость: для auto-пунктов применимость зависит от структуры хода
// (HasRoll/Ended/ClosedTiers). Для LLM-пунктов применимость КАЖДОГО хода
// зависит от семантики (нужно знать, был ли на этом ходу бросок/секрет/etc.).
// Адаптер LLM-судьи сам решает применимость и возвращает OutcomeNA с пояс-
// нением, если пункт к ходу не применим.

type Rubric struct {
	// auto — статические судьи по Item. Один судья на пункт; nil = пункт
	// считается LLM-only (B5..C12 + A1).
	auto map[Item]Judge
	// llm — внешний нарративный судья. Используется для пунктов, где нет auto.
	llm LLMJudge
}

// NewRubric собирает рубрику с дефолтным набором auto-судей (A2/A3/A4/D13)
// и StubJudge для всего LLM-слоя. Подменить LLM-судью — Rubric.WithLLM.
func NewRubric() *Rubric {
	r := &Rubric{
		auto: map[Item]Judge{
			// A2 собирается из двух судей: первый про показ броска, второй
			// про честность исхода. Apply объединяет их в один Verdict с
			// правилом «оба pass → pass; один fail → fail».
			A2:  join(judgeRollShown(), judgeRollHonest()),
			A3:  judgeDCStated(),
			A4:  judgeFailForward(),
			C10: judgeWorldRolls(),
			D13: judgeNoEarlySecret(),
		},
		llm: NewStubJudge(),
	}
	return r
}

// WithLLM подменяет LLM-судью. Возвращает копию Rubric'а; оригинал не
// мутирует. Удобно для тестов и для сравнения моделей в отчёте.
func (r *Rubric) WithLLM(j LLMJudge) *Rubric {
	cp := *r
	cp.llm = j
	return &cp
}

// HasAuto говорит, есть ли в этой рубрике auto-судья для данного пункта.
// Полезно для адаптеров LLM, чтобы не звать модель зря.
func (r *Rubric) HasAuto(item Item) bool { _, ok := r.auto[item]; return ok }

// Apply прогоняет запись по ВСЕМ пунктам Эталона, для каждого пункта выбирая
// auto-судью (если есть) или LLM-судью. Возвращает срез Verdict в каноничном
// порядке Items(). Это форма выходного отчёта рубрики.
//
// Порядок стабильный: Items() пройдены ровно один раз, и каждый Verdict лежит
// в своей позиции. Это удобно для tabular-отчётов и для сравнения прогонов.
//
// LLM-пункты оцениваются ПОСЛЕДОВАТЕЛЬНО (один вызов на пункт). Для быстрых
// прогонов есть ApplyConcurrent — там soft-пункты идут параллельно.
func (r *Rubric) Apply(ctx context.Context, rec TurnRecord) []Verdict {
	out := make([]Verdict, 0, len(Items()))
	for _, item := range Items() {
		out = append(out, r.Judge(ctx, item, rec))
	}
	return out
}

// ApplyConcurrent — то же, что Apply, но LLM-пункты оцениваются ПАРАЛЛЕЛЬНО.
//
// Зачем: на ходе 9 мягких пунктов = 9 вызовов модели. Последовательно это
// девять сетевых задержек подряд; параллельно — одна. Экономия времени
// в разы при том же расходе токенов.
//
// Инварианты те же, что у Apply:
//   - результат в каноничном порядке Items() (не в порядке завершения);
//   - auto-пункты по-прежнему считаются последовательно (они чистые и дешёвые);
//   - LLM-судья должен быть безопасным для параллельного вызова. Это
//     контракт адаптера: GatewayJudge зовёт шлюз, шлюз потокобезопасен.
//
// Если LLM-судья НЕ готов к параллельному вызову — используйте Apply.
func (r *Rubric) ApplyConcurrent(ctx context.Context, rec TurnRecord) []Verdict {
	out := make([]Verdict, len(Items()))
	items := Items()

	// Сначала auto-пункты (дёшево, без сети).
	for i, item := range items {
		if _, ok := r.auto[item]; ok {
			out[i] = r.auto[item](rec)
		}
	}
	// Потом LLM-пункты — параллельно.
	var wg sync.WaitGroup
	for i, item := range items {
		if _, ok := r.auto[item]; ok {
			continue
		}
		wg.Add(1)
		go func(idx int, it Item) {
			defer wg.Done()
			out[idx] = r.Judge(ctx, it, rec)
		}(i, item)
	}
	wg.Wait()
	return out
}

// Judge — точечная оценка по ОДНОМУ пункту. Используется из Apply и из
// внешних тестов/отчётов. Для auto-судьи применимость/итог вычисляет сама
// (Verdict.Outcome=NA при «не применим»). Для LLM-судьи — адаптер решает.
//
// Если в рубрике нет auto-судьи для пункта и LLM-stub возвращает NA — это
// штатный режим «в офлайне мягкие пункты пропускаются», не поломка.
func (r *Rubric) Judge(ctx context.Context, item Item, rec TurnRecord) Verdict {
	if j, ok := r.auto[item]; ok {
		return j(rec)
	}
	if r.llm == nil {
		return Verdict{Item: item, Outcome: OutcomeNA, Reason: "нет ни auto-, ни LLM-судьи", Source: KindLLM}
	}
	return r.llm.Judge(ctx, item, rec)
}

// Summary — сводка по срезу вердиктов: сколько pass/fail/NA и список failed
// items по Reason. Используется репортом и assert'ами в тестах.
type Summary struct {
	Pass    int
	Fail    int
	NA      int
	Total   int
	Failed  []Item
	Reasons map[Item]string
}

// Summarize сворачивает срез Verdict'ов в Summary. Failed содержит список
// проваленных пунктов, Reasons — человеческое объяснение каждого провала (для
// репорта/диагностики). Порядок Failed по каноничному Items().
func Summarize(vs []Verdict) Summary {
	s := Summary{
		Total:   len(vs),
		Reasons: map[Item]string{},
	}
	for _, v := range vs {
		switch v.Outcome {
		case OutcomePass:
			s.Pass++
		case OutcomeFail:
			s.Fail++
			s.Failed = append(s.Failed, v.Item)
			if v.Reason != "" {
				s.Reasons[v.Item] = v.Reason
			}
		default:
			s.NA++
		}
	}
	return s
}

// join — комбинатор для A2: один auto-судья (Judge) склеивает два подпункта
// в один Verdict. Логика:
//
//   - оба pass → pass;
//   - один NA → отдаёт второго (его NA, его pass/fail) — пункту в целом
//     применимо то, что применимо;
//   - иначе fail с пояснением первого упавшего.
//
// Чтобы итог был один Verdict на пункт, а не два (для красоты отчёта и для
// устойчивого Apply).
func join(a, b Judge) Judge {
	return func(rec TurnRecord) Verdict {
		va, vb := a(rec), b(rec)
		// Один — NA, другой — pass/fail.
		if va.Outcome == OutcomeNA && vb.Outcome != OutcomeNA {
			vb.Item = va.Item
			return vb
		}
		if vb.Outcome == OutcomeNA && va.Outcome != OutcomeNA {
			return va
		}
		// Оба NA или оба pass/fail с одной оценкой — берём va как каноничного.
		if va.Outcome == OutcomeFail || vb.Outcome == OutcomeFail {
			first := va
			if va.Outcome != OutcomeFail {
				first = vb
			}
			first.Item = A2
			first.Source = KindAuto
			return first
		}
		va.Item = A2
		va.Source = KindAuto
		return va
	}
}
