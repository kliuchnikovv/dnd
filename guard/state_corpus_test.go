package guard

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/kliuchnikovv/dnd/llm"
)

// Порог второго вектора. Противоречие состоянию — это ложь про проверяемые
// данные, и ловиться оно обязано ВСЕГДА: асимметрия тут жёстче, чем у утечки,
// где в сомнении реплика проходит. Поэтому пропуск противоречия дороже, а
// порог ниже. Ложное срабатывание всё так же стоит живой фразы — дорого, но не
// смертельно, и ремонт его частично отыгрывает.
const (
	maxMissedContradictions = 0.10
	maxStateFalsePositives  = 0.20
)

// stateCase — реплика с известной разметкой по второму вектору. contradicts —
// утверждает ли она о состоянии иное, чем сказано в дайджесте.
type stateCase struct {
	line        string
	contradicts bool
	why         string
}

// stateMaterial — ambient и мир Берна: то, на что реплика вправе опираться.
// Отдельно от дайджеста: материал говорит, о чём можно, дайджест — что верно.
var stateMaterial = []string{
	"Берн, стражник",
	"сухой и служебный, докладывает как в протокол",
	"Место: Пристань",
	"Обстановка: дождь",
	"Гавань — посёлок в устье, живущий погрузкой. Гильдия держит склады и весы.",
	"Смена тянется вторые сутки: второй стражник не выходит.",
}

// stateDigest — доверенная сводка состояния. То же, что вернул бы
// core.StateDigest: чем владеет игрок, где он, чем ранен, что знает, часы, исход
// последнего хода. Против этого реплика врать не вправе.
var stateDigest = []string{
	"Несёт при себе: предписание",
	"Состояние: раны 0, стойкость 3",
	"Сейчас находится: Пристань",
	"Знает по делу: тело на складе",
	"Часы «подозрение»: 2 из 6",
	"Итог хода: ПРОВАЛ",
}

var stateCorpus = []stateCase{
	// Противоречие состоянию: реплика утверждает о владении, месте, знании,
	// исходе, ранах или часах иное, чем сводка. Резать обязано всегда.
	{"Раз ключ от весовой у тебя в руке, отпирай да заходи.", true, "владение: ключа нет, есть предписание"},
	{"Ты же уже в кузнице стоишь, вот и осмотрись тут.", true, "место: он на пристани, не в кузнице"},
	{"Дело, считай, раскрыто — виновный давно известен.", true, "исход: дело не раскрыто"},
	{"Часы вышли, времени у тебя больше не осталось.", true, "часы: 2 из 6, не полны"},
	{"Раз в этот раз получилось, дальше пойдёт легче.", true, "исход: ход провалился, не удался"},
	{"Ты и сам знаешь, кто его порешил, чего спрашивать.", true, "знание: виновный парти неизвестен"},
	{"Ты весь изранен, едва на ногах держишься.", true, "раны: их нет"},
	{"При себе у тебя пусто, нечем и предъявить.", true, "владение: предписание при нём есть"},
	{"Ты же смотрителя весов уже расспросил, забыл, что ли?", true, "знание/событие: такого хода не было"},

	// Ambient и верный пересказ состояния: резать это нельзя — иначе Мастер и
	// персонаж снова заговорят заглушками.
	{"Сыро на пристани, доски под ногами так и норовят уйти.", false, "погода и верное место"},
	{"Предписание при тебе — если что, предъявишь.", false, "верное владение"},
	{"Тело на складе, это ты и без меня знаешь.", false, "верный пересказ известного"},
	{"На этот раз не вышло. Бывает, чего уж.", false, "верный исход — провал"},
	{"Стоишь тут на пристани да мокнешь, как и я.", false, "верное место, ambient"},
	{"Раны тебя не мучают, и то ладно по такой погоде.", false, "верное состояние — цел"},
	{"Дождь третий день не унимается, сапоги не сохнут.", false, "чистый ambient"},
	{"Подозрение-то растёт помаленьку, да пока не через край.", false, "верный пересказ часов — не полны"},
	{"Пока держишься, стойкости на тебе хватает.", false, "верное состояние — стойкость есть"},
}

// Набор обязан оставаться сбалансированным: перекос делает частоты
// бессмысленными, а тест — зелёным по построению.
func TestStateCorpusStaysBalanced(t *testing.T) {
	var contra, colour int
	for _, c := range stateCorpus {
		if strings.TrimSpace(c.line) == "" || strings.TrimSpace(c.why) == "" {
			t.Errorf("реплика без текста или без разметки: %+v", c)
		}
		if c.contradicts {
			contra++
		} else {
			colour++
		}
	}
	if contra < 8 || colour < 8 {
		t.Errorf("набор перекошен: противоречий %d, краски %d", contra, colour)
	}
}

type stateRates struct {
	missed        float64 // доля пропущенных противоречий
	falsePositive float64 // доля зарубленной краски/верного пересказа
}

// stateRate прогоняет набор через судью «реплика прошла?» и считает частоты.
func stateRate(pass func(stateCase) bool) stateRates {
	var contra, missed, colour, cut int
	for _, c := range stateCorpus {
		ok := pass(c)
		if c.contradicts {
			contra++
			if ok {
				missed++
			}
			continue
		}
		colour++
		if !ok {
			cut++
		}
	}
	return stateRates{
		missed:        float64(missed) / float64(contra),
		falsePositive: float64(cut) / float64(colour),
	}
}

// Частоты считаются на разметке, и сама арифметика проверяется без сети:
// метрика, которая врёт, хуже отсутствующей.
func TestStateRatesAreCountedFromLabels(t *testing.T) {
	all := stateRate(func(stateCase) bool { return false }) // режет всё
	if all.missed != 0 || all.falsePositive != 1 {
		t.Errorf("режущий всё судья дал пропуски %.2f, ложные %.2f", all.missed, all.falsePositive)
	}
	none := stateRate(func(stateCase) bool { return true }) // пропускает всё
	if none.missed != 1 || none.falsePositive != 0 {
		t.Errorf("пропускающий всё судья дал пропуски %.2f, ложные %.2f", none.missed, none.falsePositive)
	}
	perfect := stateRate(func(c stateCase) bool { return !c.contradicts })
	if perfect.missed != 0 || perfect.falsePositive != 0 {
		t.Errorf("идеальный судья дал пропуски %.2f, ложные %.2f", perfect.missed, perfect.falsePositive)
	}
}

// TestStateGuardPrecisionOnLiveModel — метрика точности второго вектора, а не
// «на глаз». Требует живой модели: суждение о том, что противоречит состоянию,
// фейком не подменяется — подменённое проверяло бы наш стаб, а не промпт.
//
//	DND_GUARD_MODEL=anthropic/claude-3.5-haiku OPENROUTER_API_KEY=... \
//	  go test ./guard -run TestStateGuardPrecisionOnLiveModel -v
func TestStateGuardPrecisionOnLiveModel(t *testing.T) {
	model := os.Getenv("DND_GUARD_MODEL")
	if model == "" || os.Getenv("OPENROUTER_API_KEY") == "" {
		t.Skip("нужны DND_GUARD_MODEL и OPENROUTER_API_KEY: " +
			"частота ложных срабатываний меряется только на живой модели")
	}
	if !llm.HasPrice(model) {
		in, out := envPrice("DND_GUARD_PRICE_IN", 1), envPrice("DND_GUARD_PRICE_OUT", 5)
		t.Logf("цена %s принята за %.2f/%.2f $ за миллион токенов "+
			"(задайте DND_GUARD_PRICE_IN/OUT, если не так)", model, in, out)
		llm.SetPrice(model, llm.Price{
			InputMicroPerMTok:  int64(in * 1_000_000),
			OutputMicroPerMTok: int64(out * 1_000_000),
		})
	}
	gw := llm.NewGateway(
		llm.NewRouter().Route(llm.RoleCanonGuard,
			llm.Target{Provider: llm.NewOpenRouter(llm.ORWithTitle("dnd-state-corpus")), Model: model}),
		llm.NewLedger(llm.Caps{}))
	g := New(gw)

	var wrong []string
	var caughtByStateFlag int
	got := stateRate(func(c stateCase) bool {
		v, err := g.Check(context.Background(), c.line, stateMaterial, stateDigest, llm.Request{})
		if err != nil {
			t.Fatalf("проверка сорвалась на %q: %v", c.line, err)
		}
		if c.contradicts && v.ContradictsState {
			caughtByStateFlag++
		}
		if v.OK == c.contradicts {
			wrong = append(wrong, fmt.Sprintf("%q (%s) → ok=%v state=%v leak=%v what=%q",
				c.line, c.why, v.OK, v.ContradictsState, v.Leak, v.What))
		}
		return v.OK
	})

	t.Logf("пропущено противоречий: %.0f%%  зарублено краски: %.0f%%  "+
		"пойманы флагом состояния: %d  (расход %.4f $)",
		got.missed*100, got.falsePositive*100, caughtByStateFlag,
		float64(gw.Stats().SpentMicro)/1e6)
	for _, w := range wrong {
		t.Logf("  промах: %s", w)
	}
	if got.missed > maxMissedContradictions {
		t.Errorf("пропущено противоречий %.0f%%, порог %.0f%%",
			got.missed*100, maxMissedContradictions*100)
	}
	if got.falsePositive > maxStateFalsePositives {
		t.Errorf("зарублено краски/верного пересказа %.0f%%, порог %.0f%% — "+
			"Мастер и персонаж снова заговорят заглушками",
			got.falsePositive*100, maxStateFalsePositives*100)
	}
}
