package cases

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/naming"
	"github.com/kliuchnikovv/dnd/store"
)

// validateFile проверяет инварианты рукописного дела. Падает при загрузке, а не
// на сороковой минуте прогона. Возвращает все нарушения разом: чинить дело по
// одному сообщению за запуск — это часы вместо минут.
// namedIn — кто из людей дела назван в тексте. Совпадение с поправкой на падеж
// берётся из naming: второй его экземпляр здесь разъехался бы с первым.
//
// Сравниваются только РАЗЛИЧАЮЩИЕ слова имени. naming.Mentions по построению
// щедр — «Токе, писарь гильдии» он находит и по слову «гильдии», и для ввода
// игрока это правильно: двусмысленность разрешается дальше по цепочке. Здесь
// разрешать её нечем, а ложная тревога валит запуск дела, поэтому слово,
// которое встречается ещё в названии места или в имени другого, за имя не
// считается. Без этого честная подсказка «в конторе гильдии ведут книги»
// читалась как упоминание писаря.
func namedIn(text string, f File) map[store.EntityID]bool {
	lower := strings.ToLower(text)
	out := map[store.EntityID]bool{}
	for _, e := range f.Entities {
		if e.Kind != store.EntityNPC {
			continue
		}
		for _, w := range distinctiveWords(e, f) {
			if naming.Mentions(lower, w) {
				out[e.ID] = true
				break
			}
		}
	}
	return out
}

// distinctiveWords — слова имени, которые указывают именно на этого человека.
func distinctiveWords(who store.Entity, f File) []string {
	var out []string
	for _, w := range strings.Fields(strings.ToLower(who.Name)) {
		w = strings.Trim(w, ",.;:!?«»\"'()")
		if len([]rune(w)) < 4 || sharedWord(w, who.ID, f) {
			continue
		}
		out = append(out, w)
	}
	return out
}

// sharedWord — это слово встречается ещё где-то: в названии места или в имени
// другой сущности. Такое слово человека не опознаёт.
func sharedWord(word string, self store.EntityID, f File) bool {
	for _, l := range f.Locations {
		if naming.Mentions(strings.ToLower(l.Name), word) {
			return true
		}
	}
	for _, e := range f.Entities {
		if e.ID != self && naming.Mentions(strings.ToLower(e.Name), word) {
			return true
		}
	}
	return false
}

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
			if mandatory[h.FactID] {
				add("у факта %s больше одного mandatory-держателя: второй источник — "+
					"это подтверждение, и оно обязано стоить броска", h.FactID)
			}
			mandatory[h.FactID] = true
		}
	}

	// Единственная защита от «кубик убил дело».
	for _, fact := range f.Facts {
		if !mandatory[fact.ID] && !revealedByCompare(f, fact.ID) {
			add("у факта %s нет ни одного mandatory-держателя", fact.ID)
		}
	}

	// Пропы: инертны по построению, но ссылки и теги всё равно надо проверить.
	props := map[store.PropID]bool{}
	for _, p := range f.Props {
		props[p.ID] = true
		if !hasLocation(f, p.Node) {
			add("проп %s стоит в несуществующем узле %s", p.ID, p.Node)
		}
		if entities[store.EntityID(p.ID)] {
			add("идентификатор %s занят и пропом, и сущностью", p.ID)
		}
		if strings.TrimSpace(p.Text) == "" {
			add("у пропа %s нет текста — в выводе он станет заглушкой", p.ID)
		}
		for _, tag := range p.Tags {
			if !store.KnownPropTag(tag) {
				add("проп %s несёт тег %q: словарь тегов пуст, "+
					"инструментальность задаётся kind:%q, среда живёт на узле",
					p.ID, tag, store.ToolKind)
			}
		}
	}
	for _, h := range f.FactHolders {
		if props[store.PropID(h.HolderID)] {
			add("проп %s не может держать факт %s: пропы инертны", h.HolderID, h.FactID)
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
		// Говорящий персонаж без желания реактивен: он отвечает и никогда не
		// начинает. Разговор с таким читается как справка, а не как человек.
		if len(d.TalksAbout) > 0 && len(d.Wants) == 0 {
			add("у дневника %s нет ни одного wants — персонаж только отвечает", d.Entity)
		}
		for _, w := range d.Wants {
			if strings.TrimSpace(w) == "" {
				add("у дневника %s пустое желание", d.Entity)
			}
		}
		// Расположение вне разумных границ почти всегда опечатка: шкала
		// читается словами, и за пределами этого диапазона слов нет.
		if d.Disposition < -3 || d.Disposition > 3 {
			add("у дневника %s расположение %d вне диапазона -3..3", d.Entity, d.Disposition)
		}
	}

	// Общий ключ на глагол — последнее звено цепочки флейвора. Без него проба
	// по цели, у которой фактов не осталось, печатает служебный [ключ].
	usedVerbs := map[string]bool{}
	for _, h := range f.FactHolders {
		for _, v := range h.Gate.Verbs {
			usedVerbs[v] = true
		}
	}
	verbs := make([]string, 0, len(usedVerbs))
	for v := range usedVerbs {
		verbs = append(verbs, v)
	}
	sort.Strings(verbs)
	for _, v := range verbs {
		if strings.TrimSpace(f.Flavour[v]) == "" {
			add("у глагола %s нет общего текста — проба впустую напечатает [%s.цель]", v, v)
		}
	}

	// Развязка: у каждого факта, стоящего за токеном правильного ответа,
	// обязана быть клауза, иначе верное обвинение печатает пустоту.
	truthTokens := map[string]store.Token{
		"who": f.Truth.Who, "how": f.Truth.How, "when": f.Truth.When, "why": f.Truth.Why,
	}
	clause := map[store.FactID]string{}
	kind := map[store.FactID]store.FactKind{}
	for _, fact := range f.Facts {
		clause[fact.ID] = fact.SummationClause
		kind[fact.ID] = fact.Kind
	}
	for _, t := range f.Tokens {
		if truthTokens[t.Slot] != t.Token {
			continue
		}
		if strings.TrimSpace(clause[t.Fact]) == "" {
			add("у факта %s нет клаузы развязки, а он стоит за токеном %s", t.Fact, t.Token)
		}
		// Факт за токеном правды по определению объясняет, что произошло.
		if kind[t.Fact] != store.FactConcept {
			add("факт %s стоит за токеном правды, но размечен как %s, а должен быть concept",
				t.Fact, kind[t.Fact])
		}
	}
	// Подсказка чутья — единственный канал помощи застрявшему. Без неё игрок,
	// вставший в первой сессии, закрывает консоль молча. Говорящего подсказке
	// не нужно: она принадлежит игроку, а не персонажу.
	//
	// Требуется она там, где есть что добывать. Дело, все факты которого
	// выданы на старте, указывать может только на известное — а на известное
	// чутьё молчит по построению.
	// Брифинг обязателен. Без него игрок не знает, зачем он здесь, и всё, что
	// дальше опирается на место преступления или на бумагу в кармане, читается
	// как знание из ниоткуда — живой прогон встал ровно на этом.
	if strings.TrimSpace(f.Briefing) == "" {
		add("у дела нет брифинга — игрок не узнает, зачем он здесь")
	}
	holds := map[store.FactID]map[store.EntityID]bool{}
	for _, h := range f.FactHolders {
		if holds[h.FactID] == nil {
			holds[h.FactID] = map[store.EntityID]bool{}
		}
		holds[h.FactID][h.HolderID] = true
	}
	knownAtStart := map[store.FactID]bool{}
	for _, s := range f.StartFacts {
		knownAtStart[s.Fact] = true
	}
	findable := 0
	for _, fact := range f.Facts {
		if !knownAtStart[fact.ID] {
			findable++
		}
	}
	if findable > 0 && len(f.Hints) == 0 {
		add("у дела нет подсказок — застрявшему игроку неоткуда получить помощь")
	}
	for id, line := range f.Hints {
		if !facts[id] {
			add("подсказка ссылается на несуществующий факт %s", id)
		}
		// Подсказка на стартовый факт не сработает НИКОГДА: Hint пропускает
		// известное. Молча это выглядит как «чутьё сломано» — и однажды именно
		// так и выглядело, пока не выяснилось, что молчать оно обязано.
		if knownAtStart[id] {
			add("подсказка про %s указывает на факт, известный парти с начала: "+
				"чутьё пропускает известное и не сработает никогда", id)
		}
		if strings.TrimSpace(line) == "" {
			add("подсказка про %s пуста", id)
		}
		// Названный в подсказке человек обязан этот факт ДЕРЖАТЬ. Иначе
		// подсказка отправляет мимо цели, а выглядит уверенно: живой прогон
		// получил «Ивар не отходит от стойки, спросите его про деньги», хотя
		// держит f_ivar_debt вдова. Ошибка пережила и авторскую редактуру, и
		// правку вслед за ней — глазами она не ловится.
		for who := range namedIn(line, f) {
			if !holds[id][who] {
				add("подсказка про %s называет %s, который этого факта не держит", id, who)
			}
		}
	}

	if strings.TrimSpace(f.Aftermath) == "" {
		add("у дела нет aftermath — после речи игрока печатать нечего")
	}
	if strings.TrimSpace(f.ColdCase) == "" {
		add("у дела нет cold_case_text — висяк заканчивается молчанием")
	}

	if len(f.StartFacts) == 0 {
		add("у дела нет стартовых фактов — первый ход некуда сделать")
	}

	// Предмет, на который ссылается гейт, обязан быть ДОБЫВАЕМЫМ: лежать в
	// стартовом инвентаре или выдаваться каким-нибудь фактом. Иначе дело
	// выглядит проходимым, а факт закрыт навсегда — и увидит это игрок на
	// сороковой минуте прогона, а не автор на загрузке.
	items := map[store.ItemID]bool{}
	for _, item := range f.Items {
		items[item.ID] = true
		// Вид предмета — закрытый словарь: опечатка в нём делает предмет
		// молча бесполезным, и заметно это будет только в игре.
		if !store.KnownItemKind(item.Kind) {
			add("предмет %s имеет вид %q вне словаря %v", item.ID, item.Kind, store.ItemKinds)
		}
		if strings.TrimSpace(item.Name) == "" {
			add("у предмета %s нет имени — игрок не сможет его назвать", item.ID)
		}
	}
	obtainable := map[store.ItemID]bool{}
	for _, id := range f.StartInventory {
		obtainable[id] = true
	}
	for _, fact := range f.Facts {
		if fact.GrantsItem == "" {
			continue
		}
		if !items[fact.GrantsItem] {
			add("факт %s выдаёт предмет %s, которого нет в items", fact.ID, fact.GrantsItem)
			continue
		}
		obtainable[fact.GrantsItem] = true
	}
	for _, h := range f.FactHolders {
		for _, id := range h.Gate.RequiresItems {
			switch {
			case !items[id]:
				add("гейт факта %s требует предмет %s, которого нет в items", h.FactID, id)
			case !obtainable[id]:
				add("предмет %s недобываем: гейт факта %s требует его, "+
					"но его нет ни в start_inventory, ни в выдаче фактом", id, h.FactID)
			}
		}
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
