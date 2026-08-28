package guard

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/kliuchnikovv/dnd/llm"
)

// Порог проверки — не «на глаз». Guard обязан резать утечку дела и пропускать
// бытовую краску, и обе частоты меряются на размеченном наборе.
//
// Цена ошибок не симметрична, поэтому и пороги разные. Пропущенная утечка
// ломает дело: она пишет в канон намёком то, что должен был выдать гейт.
// Ложное срабатывание стоит одной живой фразы — дорого, но не смертельно, и
// ремонт его частично отыгрывает.
const (
	maxMissedLeaks    = 0.10
	maxFalsePositives = 0.20
)

// guardCase — реплика с известной разметкой. Разметка авторская: это тот же
// приём, что в тесте факторов, только шкала не бросок, а суждение.
type guardCase struct {
	line string
	leak bool
	why  string
}

// guardMaterial — материал Берна: быт, обстановка, один известный факт.
// Один и тот же для всех реплик, иначе частоты не сравнимы.
var guardMaterial = []string{
	"Берн, стражник",
	"сухой и служебный, докладывает как в протокол",
	"Место: Пристань",
	"Обстановка: дождь",
	"Рядом: Нильс, мальчишка-посыльный",
	"Смена с темноты и потом снова с рассветом: второй стражник не выходит, и тянешь ты обе.",
	"Гавань — посёлок в устье, живущий погрузкой. Гильдия держит склады и весы.",
	"ключи от весовой: у смотрителя весов, он же запирает на ночь",
	"Тело сборщика податей Халдена найдено на складе у пристани",
}

var guardCorpus = []guardCase{
	// Утечка: конкретика про дело, которой в материале нет.
	{"Так это Токе его и порешил, писарь из конторы.", true, "имя виновного"},
	{"У кузнеца в кузне нашли шнур от печати, я сам видел.", true, "улика и место"},
	{"В гроссбухе за прошлый месяц три записи подскоблены.", true, "чужая бумага и число"},
	{"Халдена задушили около полуночи, до отлива.", true, "время преступления"},
	{"Вдова его в ту ночь из дому выходила, соседи видели.", true, "кто где был"},
	{"У нас в посёлке магистратский следователь этим уже занимался.", true, "учреждение, которого мир не предполагает"},
	{"Склад отпирал человек с ключом, а ключ один — у писаря.", true, "вывод по делу"},
	{"В книге приливов помечено, что засов задвинули до заката.", true, "содержание чужой записи"},
	{"Фермер Олсен с той стороны залива в ту ночь скот перегонял.", true, "человек, которого в мире нет"},
	{"Долг у кузнеца был двенадцать марок, срок вышел в пятницу.", true, "число вокруг преступления"},

	// Краска: быт, эмоция, уклонение. Резать это нельзя — именно из-за этого
	// персонаж говорил заглушками.
	{"Сыро. Третью неделю сыро, и сапоги уже не сохнут.", false, "погода и быт"},
	{"Спину сорвал ещё в прошлую смену, а сменщика всё нет.", false, "быт вне материала"},
	{"Чай в караулке остыл вчера, и никому до того дела нет.", false, "быт из затравки"},
	{"А вам это зачем? Предписание сперва покажите.", false, "встречный вопрос"},
	{"Кто ж его знает. Не моё это дело — я тут стою и мокну.", false, "уклонение"},
	{"Тело на складе нашли, это верно. Больше ничего не скажу.", false, "пересказ известного факта"},
	{"Ключи у смотрителя весов, к нему и идите.", false, "пересказ канона"},
	{"Мальчишка вон рядом крутится, у него и спросите, он говорун.", false, "тот, кто в обстановке"},
	{"Не хворать. Хотя в такую погоду это пожелание пустое.", false, "любезность"},
	{"Стою тут один за двоих, а начальству и не доложишь толком.", false, "ворчание на начальство"},
	{"Гильдия своё возьмёт, она всегда своё берёт.", false, "уклад из затравки"},
	{"Да что вы всё спрашиваете. Идите в контору, там и спросят.", false, "раздражение и отсылка"},
}

// Набор обязан оставаться сбалансированным: перекос в любую сторону делает
// частоты бессмысленными, а тест — зелёным по построению.
func TestGuardCorpusStaysBalanced(t *testing.T) {
	var leaks, colour int
	for _, c := range guardCorpus {
		if strings.TrimSpace(c.line) == "" || strings.TrimSpace(c.why) == "" {
			t.Errorf("реплика без текста или без разметки: %+v", c)
		}
		if c.leak {
			leaks++
		} else {
			colour++
		}
	}
	if leaks < 8 || colour < 8 {
		t.Errorf("набор перекошен: утечек %d, краски %d", leaks, colour)
	}
}

// Частоты считаются на разметке, и сама эта арифметика проверяется без сети:
// метрика, которая врёт, хуже отсутствующей.
func TestGuardRatesAreCountedFromLabels(t *testing.T) {
	// Судья, который режет всё: обязан дать нулевые пропуски и полные
	// ложные срабатывания.
	all := rate(t, func(guardCase) bool { return false })
	if all.missed != 0 || all.falsePositive != 1 {
		t.Errorf("режущий всё судья дал пропуски %.2f, ложные %.2f",
			all.missed, all.falsePositive)
	}
	// Судья, который пропускает всё: наоборот.
	none := rate(t, func(guardCase) bool { return true })
	if none.missed != 1 || none.falsePositive != 0 {
		t.Errorf("пропускающий всё судья дал пропуски %.2f, ложные %.2f",
			none.missed, none.falsePositive)
	}
	// Идеальный судья по разметке.
	perfect := rate(t, func(c guardCase) bool { return !c.leak })
	if perfect.missed != 0 || perfect.falsePositive != 0 {
		t.Errorf("идеальный судья дал пропуски %.2f, ложные %.2f",
			perfect.missed, perfect.falsePositive)
	}
}

// envPrice читает цену в долларах за миллион токенов.
func envPrice(name string, def float64) float64 {
	v, err := strconv.ParseFloat(os.Getenv(name), 64)
	if err != nil || v <= 0 {
		return def
	}
	return v
}

type rates struct {
	missed        float64 // доля пропущенных утечек
	falsePositive float64 // доля зарубленной краски
}

// rate прогоняет набор через судью «реплика прошла?» и считает частоты.
func rate(t *testing.T, pass func(guardCase) bool) rates {
	t.Helper()
	var leaks, missed, colour, cut int
	for _, c := range guardCorpus {
		ok := pass(c)
		if c.leak {
			leaks++
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
	return rates{
		missed:        float64(missed) / float64(leaks),
		falsePositive: float64(cut) / float64(colour),
	}
}

// TestGuardFrequencyOnLiveModel — та самая метрика, а не «на глаз». Требует
// живой модели: суждение о том, что похоже на дело, фейком не подменяется —
// подменённое проверяло бы наш стаб, а не промпт.
//
//	DND_GUARD_MODEL=anthropic/claude-3.5-haiku OPENROUTER_API_KEY=... \
//	  go test ./actor -run TestGuardFrequencyOnLiveModel -v
func TestGuardFrequencyOnLiveModel(t *testing.T) {
	model := os.Getenv("DND_GUARD_MODEL")
	if model == "" || os.Getenv("OPENROUTER_API_KEY") == "" {
		t.Skip("нужны DND_GUARD_MODEL и OPENROUTER_API_KEY: " +
			"частота ложных срабатываний меряется только на живой модели")
	}
	if !llm.HasPrice(model) {
		// Без цены шлюз не пропустит вызов, и это правильно: расход должен
		// быть виден. Цену задаёт тот, кто запускает: слаги у OpenRouter свои,
		// и подставлять их цены по памяти значит печатать неверный расход.
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
			llm.Target{Provider: llm.NewOpenRouter(llm.ORWithTitle("dnd-guard-corpus")), Model: model}),
		llm.NewLedger(llm.Caps{}))
	g := New(gw)

	var wrong []string
	got := rate(t, func(c guardCase) bool {
		v, err := g.Check(context.Background(), c.line, guardMaterial, nil, llm.Request{})
		if err != nil {
			t.Fatalf("проверка сорвалась на %q: %v", c.line, err)
		}
		if v.OK == c.leak {
			wrong = append(wrong, fmt.Sprintf("%q (%s) → invented=%v, what=%q",
				c.line, c.why, !v.OK, v.What))
		}
		return v.OK
	})

	t.Logf("пропущено утечек: %.0f%%  зарублено краски: %.0f%%  (расход %.4f $)",
		got.missed*100, got.falsePositive*100, float64(gw.Stats().SpentMicro)/1e6)
	for _, w := range wrong {
		t.Logf("  промах: %s", w)
	}
	if got.missed > maxMissedLeaks {
		t.Errorf("пропущено утечек %.0f%%, порог %.0f%%", got.missed*100, maxMissedLeaks*100)
	}
	if got.falsePositive > maxFalsePositives {
		t.Errorf("зарублено краски %.0f%%, порог %.0f%% — персонаж снова заговорит заглушками",
			got.falsePositive*100, maxFalsePositives*100)
	}
}
