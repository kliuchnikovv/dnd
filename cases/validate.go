package cases

import (
	"errors"
	"fmt"
	"strings"

	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/store"
)

// validateFile проверяет инварианты рукописного дела. Падает при загрузке, а не
// на сороковой минуте прогона. Возвращает все нарушения разом: чинить дело по
// одному сообщению за запуск — это часы вместо минут.
func validateFile(f File) error {
	var bad []string
	add := func(format string, args ...any) { bad = append(bad, fmt.Sprintf(format, args...)) }

	entities := map[store.EntityID]bool{}
	for _, e := range f.Entities {
		entities[e.ID] = true
		if e.Node != "" && !hasLocation(f, e.Node) {
			add("сущность %s стоит в несуществующем узле %s", e.ID, e.Node)
		}
	}
	facts := map[store.FactID]bool{}
	for _, fact := range f.Facts {
		facts[fact.ID] = true
	}

	// Смежность симметрична: проход в одну сторону — почти всегда опечатка.
	for _, l := range f.Locations {
		for _, n := range l.Adjacent {
			if !hasLocation(f, n) {
				add("узел %s ведёт в несуществующий %s", l.ID, n)
				continue
			}
			if !adjacent(f, n, l.ID) {
				add("нарушена смежность: %s -> %s есть, обратного нет", l.ID, n)
			}
		}
	}

	mandatory := map[store.FactID]bool{}
	for _, h := range f.FactHolders {
		if !facts[h.FactID] {
			add("держатель ссылается на несуществующий факт %s", h.FactID)
		}
		if !entities[h.HolderID] {
			add("факт %s держит несуществующая сущность %s", h.FactID, h.HolderID)
		}
		if len(h.Gate.Verbs) == 0 {
			add("у держателя факта %s пустой список глаголов", h.FactID)
		}
		for _, v := range h.Gate.Verbs {
			if _, ok := core.LookupVerb(v); !ok {
				add("gate факта %s ссылается на несуществующий глагол %q", h.FactID, v)
			}
		}
		if req := h.Gate.Requires; req != nil && len(req.Of) > 0 {
			for _, r := range req.Of {
				if !facts[r] {
					add("gate факта %s требует несуществующий факт %s", h.FactID, r)
				}
			}
			if req.N < 1 || req.N > len(req.Of) {
				add("gate факта %s задаёт порог %d при %d предпосылках",
					h.FactID, req.N, len(req.Of))
			}
		}
		if h.Mandatory {
			mandatory[h.FactID] = true
		}
	}

	// Единственная защита от «кубик убил дело».
	for _, fact := range f.Facts {
		if !mandatory[fact.ID] && !revealedByCompare(f, fact.ID) {
			add("у факта %s нет ни одного mandatory-держателя", fact.ID)
		}
	}

	// Каждый слот правильного ответа должен быть достижим токеном под фактом.
	tokens := map[string]map[store.Token]bool{}
	for _, t := range f.Tokens {
		if !facts[t.Fact] {
			add("токен %s опирается на несуществующий факт %s", t.Token, t.Fact)
		}
		if tokens[t.Slot] == nil {
			tokens[t.Slot] = map[store.Token]bool{}
		}
		tokens[t.Slot][t.Token] = true
	}
	for slot, want := range map[string]store.Token{
		"who": f.Truth.Who, "how": f.Truth.How, "when": f.Truth.When, "why": f.Truth.Why,
	} {
		if !tokens[slot][want] {
			add("слот %s правильного ответа не покрыт ни одним токеном", slot)
		}
	}

	for _, c := range f.Contradictions {
		for _, id := range []store.FactID{c.A, c.B, c.Reveals} {
			if !facts[id] {
				add("противоречие ссылается на несуществующий факт %s", id)
			}
		}
		if f.Flavour[c.FlavourKey] == "" {
			add("у противоречия нет текста по ключу %s", c.FlavourKey)
		}
	}

	for _, cl := range f.Clocks {
		if cl.Segments <= 0 {
			add("у часов %s неположительное число сегментов", cl.ID)
		}
		if cl.OnFill.FlavourKey != "" && f.Flavour[cl.OnFill.FlavourKey] == "" {
			add("у часов %s нет текста по ключу %s", cl.ID, cl.OnFill.FlavourKey)
		}
	}

	for _, u := range f.FactUnlocks {
		if !facts[u.FactID] {
			add("разблокировка исходит из несуществующего факта %s", u.FactID)
		}
		switch u.UnlocksKind {
		case "topic":
			if !facts[store.FactID(u.UnlocksID)] {
				add("разблокировка открывает несуществующую тему %s", u.UnlocksID)
			}
		case "node":
			if !hasLocation(f, store.NodeID(u.UnlocksID)) {
				add("разблокировка открывает несуществующий узел %s", u.UnlocksID)
			}
		case "entity":
			if !entities[store.EntityID(u.UnlocksID)] {
				add("разблокировка открывает несуществующую сущность %s", u.UnlocksID)
			}
		default:
			add("неизвестный вид разблокировки %q", u.UnlocksKind)
		}
	}

	for _, d := range f.Dossiers {
		if !entities[d.Entity] {
			add("дневник заведён на несуществующую сущность %s", d.Entity)
		}
		for _, k := range d.KnowsAbout {
			if !facts[k] {
				add("дневник %s знает несуществующий факт %s", d.Entity, k)
			}
		}
		// Расположение вне разумных границ почти всегда опечатка: шкала
		// читается словами, и за пределами этого диапазона слов нет.
		if d.Disposition < -3 || d.Disposition > 3 {
			add("у дневника %s расположение %d вне диапазона -3..3", d.Entity, d.Disposition)
		}
	}

	if len(f.StartFacts) == 0 {
		add("у дела нет стартовых фактов — первый ход некуда сделать")
	}

	if len(bad) > 0 {
		return errors.New("дело не прошло валидацию:\n  - " + strings.Join(bad, "\n  - "))
	}
	return nil
}

func hasLocation(f File, id store.NodeID) bool {
	for _, l := range f.Locations {
		if l.ID == id {
			return true
		}
	}
	return false
}

func adjacent(f File, from, to store.NodeID) bool {
	for _, l := range f.Locations {
		if l.ID != from {
			continue
		}
		for _, n := range l.Adjacent {
			if n == to {
				return true
			}
		}
	}
	return false
}

// revealedByCompare — факт-вывод не нуждается в держателе: его открывает
// сопоставление, а оно броска не требует и потому кубиком не блокируется.
// Стартовый факт без единого держателя тоже не нуждается в mandatory-держателе
// — он и так известен с начала расследования. Но если у факта уже есть
// держатели (как у f_ligature в эталонном деле), он обязан остаться
// достижимым и через них: заглянуть в party_knowledge и не тронуть держатели
// — самый простой способ спрятать баг.
func revealedByCompare(f File, id store.FactID) bool {
	for _, c := range f.Contradictions {
		if c.Reveals == id {
			return true
		}
	}
	isStart := false
	for _, s := range f.StartFacts {
		if s.Fact == id {
			isStart = true
			break
		}
	}
	if !isStart {
		return false
	}
	for _, h := range f.FactHolders {
		if h.FactID == id {
			return false
		}
	}
	return true
}
