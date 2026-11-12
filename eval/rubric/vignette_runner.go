package rubric

// vignette_runner.go — harness для прогона рубрики по сценам vignette/.
//
// Почему именно vignette: у неё есть детерминированный adjudicate (State.
// Adjudicate + честный core.Dice), Recorder пишет шапку-ключ + per-turn снимок,
// а Narrate/Stраж уже разведены как интерфейсы. На dnd5e-сценариях
// зависимости сложнее (actor/affordances/locations), и эвал там — отдельная
// задача; vignette — это первая ласточка, по ней проверяем harness.
//
// Контракт:
//
//   - ScriptStep.Input — что вводит игрок; передаётся в judge.Rule.
//   - ScriptStep.DiceValues — последовательность d20 на этот ход (для dice.
//     Fixed); nil → kernel.Source (натурально случайный; не для эвала).
//   - Если задан narrator и/или guard — Narrate используется как в проде
//     (vignette.Narrate); если nil — MasterOutput остаётся пустым (auto-
//     пункты всё равно работают, LLM-пункты — нет).
//
// Возврат: по одному TurnRecord на ScriptStep. Маппинг 1:1 — если шагов 5,
// в срезе 5 записей. Это упрощает отчёты: «ход N — что зашло/не зашло».

import (
	"context"
	"strings"

	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/dice"
	"github.com/kliuchnikovv/dnd/vignette"
)

// ScriptStep — один ход сценария: ввод игрока и предсказанная кость (если нужна).
// DiceValues — кладётся в core.Dice через dice.Fixed; пустая последовательность
// → random (dice.Source) для хода.
type ScriptStep struct {
	Input      string
	DiceValues []int // nil/пусто → dice.Source (random); не годится для эвала
	// NarratorOutput — ОПЦИОНАЛЬНАЯ готовая проза Мастера. Если задана —
	//	используется как MasterOutput вместо вызова Narrate. Это для двух
	//	случаев: (1) прогон с реальной моделью, где runner не вызывает Narrate
	//	(модель зовётся отдельно), и результат подсовывается; (2) запись
	//	золотого прогона «Тихий Дол» с прозой человека-Мастера, чтобы
	//	рубрику можно было прогнать по той же записи, что и в Эталоне.
	NarratorOutput string
}

// RunOptions — что и как прогонять. Дефолты через zero-value (judge=nil →
// KeywordJudge, narrator/guard=nil → без прозы/без редактуры, DiceValues=
// пусто → random).
type RunOptions struct {
	Judge    vignette.Judge
	Narrator vignette.Narrator // nil → без Мастера
	Guard    vignette.Guard    // nil → NoGuard
}

// RunVignette прогоняет script по сцене, возвращая срез TurnRecord.
// Между шагами состояние State мутирует (как в проде). Если шаг завершил
// сцену (`Ended: true`), последующие ходы всё равно прогоятся — записи
// могут быть пустыми по смыслу, но нужны для подсчёта «после развязки».
//
// Если script пуст — возврат пустой []TurnRecord.
func RunVignette(ctx context.Context, sc *vignette.Scene, script []ScriptStep, opts RunOptions) ([]TurnRecord, error) {
	if sc == nil {
		return nil, errSceneNil
	}
	if len(script) == 0 {
		return nil, nil
	}
	j := opts.Judge
	if j == nil {
		j = vignette.KeywordJudge{}
	}
	g := opts.Guard
	if g == nil {
		g = vignette.NoGuard{}
	}

	// Стартовое состояние. На каждом шаге мы пересоздаём State с фиксированной
	// костью (dice.Fixed), либо с stream-костью — поэтому стартовое значение
	// здесь неважно (мы его тут же перезатрём).
	st := vignette.NewState(dice.Fixed(10))

	records := make([]TurnRecord, 0, len(script))
	for i, step := range script {
		// Кладём новый fixed-rng в state. core.Dice задаётся при инициализации
		// state, потом дёргается внутри Adjudicate; на следующем шаге —
		// снова свежий, не разделяем между ходами.
		if len(step.DiceValues) > 0 {
			st = restartWithFixed(st, step.DiceValues)
		} else {
			st = restartWithRandom(st)
		}

		// 1. Холодный судья → Ruling.
		ruling := j.Rule(step.Input, vignette.BuildJudgeView(sc, st))

		// 2. Ядро честно адъюдицирует.
		res := st.Adjudicate(sc, ruling)

		// 3. Проза. Если NarratorOutput подсунут — используем его (запись
		// живого прогона или золотого прогона «Тихий Дол»). Иначе зовём
		// vignette.Narrate (общий шаг сервера и CLI).
		var output string
		if step.NarratorOutput != "" {
			output = step.NarratorOutput
		} else if opts.Narrator != nil {
			var nerr error
			output, nerr = vignette.Narrate(ctx, sc, st, res, opts.Narrator, g)
			if nerr != nil {
				output = "" // намеренно пусто — ошибка Нарратора не подавляет ход
			}
		}

		// 4. Запись хода.
		rec := buildTurnRecord(sc, step, ruling, res, output)
		records = append(records, rec)

		// Ранний выход, если сцена закрылась: последующие ScriptStep'ы могут
		// быть «после развязки» — это полезно для корпуса «не должно
		// ничего меняться», но в типичном сценарии caller их не пишет.
		if res.Ended && i == len(script)-1 {
			break
		}
	}
	return records, nil
}

// buildTurnRecord собирает TurnRecord из структур vignette в формат рубрики.
// Ключ-ответ сцены (Scene.Truth + закрытые тиры) копируется один раз на
// ход — чтобы D13 мог сверять «секрет в прозе vs в revealed». Это сделано
// здесь, а не в Apply, чтобы TurnRecord был самодостаточным для отчётов.
func buildTurnRecord(sc *vignette.Scene, step ScriptStep, r vignette.Ruling, res vignette.Result, output string) TurnRecord {
	rec := TurnRecord{
		Input:        step.Input,
		RulingKind:   r.Kind,
		RollDC:       r.DC,
		RollMargin:   marginOf(res),
		Revealed:     append([]string{}, res.Revealed...),
		StateNote:    res.StateNote,
		Beat:         res.Beat,
		Ended:        res.Ended,
		EndText:      res.EndText,
		MasterOutput: output,
		SceneTruth:   sc.Truth,
	}
	if res.Roll != nil {
		rec.Roll.D20 = res.Roll.D20
		rec.Roll.Mod = res.Roll.Mod
		rec.Roll.DC = res.Roll.DC
		rec.Roll.Total = res.Roll.Total
		rec.Roll.Margin = res.Roll.Margin
		rec.Roll.Line = res.Roll.Line
		if res.Roll.D20 == 1 {
			rec.Roll.Natural = 1
		} else if res.Roll.D20 == 20 {
			rec.Roll.Natural = 20
		}
	}
	rec.ClosedTiers = closedTierTexts(sc)
	return rec
}

// closedTierTexts — закрытые (не-пассивные) тиры для D13. Используется редко:
// обычно автор дела сам вытаскивает нужные фразы в Scene.Truth. Здесь
// функция — чтобы Record был полон сам по себе и в других сценариях.
func closedTierTexts(sc *vignette.Scene) []string {
	var out []string
	for _, o := range sc.Objects {
		for _, a := range o.Aspects {
			for _, t := range a.Tiers {
				if t.Gate.Kind == vignette.GateAction || t.Gate.Kind == vignette.GateKnowledge {
					if strings.TrimSpace(t.Text) != "" {
						out = append(out, t.Text)
					}
				}
			}
		}
	}
	return out
}

// marginOf — маржа хода (Result.Roll не всегда есть; сюда передаём, чтобы
// не терять структуру).
func marginOf(res vignette.Result) int {
	if res.Roll == nil {
		return 0
	}
	return res.Roll.Margin
}

// restartWithFixed — без копирования internal-структуры виньетки-State у нас
// нет API «подменить dice на лету». Однако NewState берёт core.Dice в
// конструкторе, и State хранит его как unexported поле. Поэтому пересоздаём
// State, перенося видимые поля (Items, Progress и т. д.) — это дешевле, чем
// форкать vignette. Поля скопированы ровно те, что снаружи видимы в тестах.
func restartWithFixed(prev *vignette.State, values []int) *vignette.State {
	ns := vignette.NewState(dice.Fixed(values...))
	copyVignetteStateFields(prev, ns)
	return ns
}

func restartWithRandom(prev *vignette.State) *vignette.State {
	// dice.Source сам по себе не реализует core.Dice (только Stream). Для
	// «рандомного» хода используем stream-dice обёртку — повторяемо по seed
	// (хорошо для офлайн-тестов: если что-то странное, можно зафиксировать).
	ns := vignette.NewState(streamDice{s: dice.NewSource(0).Stream("rubric")})
	copyVignetteStateFields(prev, ns)
	return ns
}

// streamDice — адаптер от *dice.Stream к core.Dice. У Stream уже есть
// Roll(n, sides int) int; нам нужно только поднять сигнатуру до интерфейса.
type streamDice struct{ s *dice.Stream }

func (d streamDice) Roll(n, sides int) int { return d.s.Roll(n, sides) }

func copyVignetteStateFields(prev, dst *vignette.State) {
	if prev == nil || dst == nil {
		return
	}
	// Только публичные поля — внутренний rng у каждого свой.
	dst.Edge = prev.Edge
	dst.Body = prev.Body
	dst.Items = append([]string{}, prev.Items...)
	dst.OffPath = prev.OffPath
	dst.MireDepth = prev.MireDepth
	dst.Progress = prev.Progress
	dst.Turn = prev.Turn
	dst.Actions = cloneBoolMap(prev.Actions)
	dst.Facts = cloneBoolMap(prev.Facts)
	dst.Discovered = cloneBoolMap(prev.Discovered)
	dst.Tried = cloneBoolMap(prev.Tried)
}

func cloneBoolMap(m map[string]bool) map[string]bool {
	if m == nil {
		return map[string]bool{}
	}
	cp := make(map[string]bool, len(m))
	for k, v := range m {
		cp[k] = v
	}
	return cp
}

// Ошибки уровня пакета. Один — sentinel для «сцена не передана», на случай
// если кто-то позовёт RunVignette через reflect-y путь.
var errSceneNil = errRubric("rubric: scene is nil")

type errRubric string

func (e errRubric) Error() string { return string(e) }

// Чтобы core.Dice импорт не остался без дела при редактировании пакета.
var _ = core.Dice(nil)
