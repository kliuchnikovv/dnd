package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/store"
	"github.com/kliuchnikovv/dnd/view"
)

// Реф-конфигурация вида: одна честная пара (правило «Порог» × сценарий-детектив).
//
// Здесь и только здесь живёт осевой вокабуляр — слова «ранения»/«grit», слоты
// who/how/when/why, «Досье». Пакет view их не знает: он держит дескрипторы, а
// наполняет их эта конфигурация (view.Ruleset и view.Scenario). Появится второе
// правило или сценарий — появится второй такой файл, а форма вида не дрогнет.

// Реф-конфигурация обязана удовлетворять осевым интерфейсам вида: разойдись
// сигнатура — сборка вида перестанет компилироваться здесь, а не молча у
// вызова.
var (
	_ view.Ruleset  = RefRuleset{}
	_ view.Scenario = Detective{}
)

// init регистрирует view-фабрику Порога в общем реестре view. Реестр — точка
// выбора view-правила по имени сессии (server/CLI); сама реализация остаётся
// здесь, в презентации. Импорт cli тянет регистрацию side-effect'ом, поэтому
// «известность» правила равна «пакет-презентация в графе».
func init() {
	view.RegisterRuleset(core.RulesetThreshold, func() view.Ruleset { return RefRuleset{} })
}

// RefRuleset — ось ПРАВИЛА «Порог». Называет свои меры и решает по состоянию,
// какую сейчас поднять.
type RefRuleset struct{}

// harmMax вынесен в переменную: мере ран нужен УКАЗАТЕЛЬ на потолок, а брать
// адрес именованной константы нельзя.
var harmMax = core.HarmMax

// Meters — меры «Порога»: раны, grit и по мере на каждые часы. Surface — от
// состояния: рана всплывает, когда персонаж ранен; grit — пока есть, что
// тратить; часы — когда близки к срабатыванию. Здоровый персонаж с полными
// grit и далёкими часами не видит ни одной меры — и это минимализм, заданный
// правилом, а не эвристикой клиента.
func (RefRuleset) Meters(g *core.Game) []view.Meter {
	ch := g.DB.Characters[g.Actor]
	out := []view.Meter{
		{Label: "ранения", Kind: "harm", Value: ch.Harm, Max: &harmMax, Surface: ch.Harm > 0},
		{Label: "grit", Kind: "grit", Value: ch.Grit, Surface: ch.Grit > 0},
	}
	for _, c := range g.C.Snapshot() {
		max := c.Segments
		out = append(out, view.Meter{
			Label: c.Name, Kind: "clock", Value: c.Filled, Max: &max,
			Surface: clockNearFull(c),
		})
	}
	return out
}

// clockNearFull — часы в одном тике от срабатывания. «Близки» — это порог, а не
// любое движение: часы, тикнувшие раз из шести, ещё не повод занимать экран.
func clockNearFull(c store.Clock) bool {
	return c.Filled > 0 && c.Filled >= c.Segments-1
}

// Detective — ось СЦЕНАРИЯ: детектив. Даёт панель-досье как экземпляр архетипа
// «дедукция».
type Detective struct{}

// accusationSlotLabels — слова слотов обвинения. Порядок фиксирован: who → how
// → when → why, как в развязке.
var accusationSlotLabels = []struct{ name, label string }{
	{"who", "Кто"}, {"how", "Как"}, {"when", "Когда"}, {"why", "Почему"},
}

// Objective — панель цели детектива: факты с источниками и confidence, слоты
// обвинения, зафиксированные гипотезы. Скрыта по умолчанию: рабочее
// пространство вызывается жестом, а не висит на экране (Surface решает не эта
// сборка, а тот, кто покажет вид, — здесь панель отдаётся скрытой).
func (Detective) Objective(g *core.Game) *view.Panel {
	p := &view.Panel{Kind: "deduction", Title: "Досье", Surface: false}

	if facts := factItems(g); len(facts) > 0 {
		p.Sections = append(p.Sections, view.Section{Label: "Факты", Items: facts})
	}
	p.Sections = append(p.Sections, view.Section{Label: "Обвинение", Slots: accusationSlots4(g)})
	if len(g.Theories) > 0 {
		items := make([]view.PanelItem, len(g.Theories))
		for i, th := range g.Theories {
			items[i] = view.PanelItem{Label: th}
		}
		p.Sections = append(p.Sections, view.Section{Label: "Гипотезы", Items: items})
	}
	return p
}

// factItems — известные факты под своими словами. Sub несёт источник и
// confidence: по нему видно, зачем нужен третий независимый источник.
func factItems(g *core.Game) []view.PanelItem {
	bank := g.K.TopicBank()
	out := make([]view.PanelItem, 0, len(bank))
	for _, f := range bank {
		out = append(out, view.PanelItem{
			ID:    string(f),
			Label: g.DB.Facts[f].Key,
			Sub:   factSub(g, f),
		})
	}
	return out
}

// factSub — источники и confidence факта, со звёздочкой у подтверждённого.
// Звёздочка про крепость дела, а не про право обвинять: обвинять можно и без
// неё (см. легенду экрана facts).
func factSub(g *core.Game, f store.FactID) string {
	srcs := g.K.Sources(f)
	names := make([]string, len(srcs))
	for i, s := range srcs {
		names[i] = string(s)
	}
	sort.Strings(names)
	mark := ""
	if g.K.Corroborated(f) {
		mark = "* "
	}
	return fmt.Sprintf("%s%.3f ← %s", mark, g.K.Confidence(f), strings.Join(names, ", "))
}

// accusationSlots4 — четыре слота обвинения с их опциями из собранных фактов.
func accusationSlots4(g *core.Game) []view.Slot {
	out := make([]view.Slot, 0, len(accusationSlotLabels))
	for _, s := range accusationSlotLabels {
		var opts []view.PanelItem
		for _, tok := range g.AvailableTokens(s.name) {
			opts = append(opts, view.PanelItem{ID: string(tok), Label: string(tok)})
		}
		out = append(out, view.Slot{Name: s.name, Label: s.label, Options: opts})
	}
	return out
}
