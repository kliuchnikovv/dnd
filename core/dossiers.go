package core

import (
	"sort"
	"strings"

	"github.com/kliuchnikovv/dnd/store"
)

// Dossiers — доступ к дневникам сущностей. Чтение детерминированное: join по
// участникам сцены, а не поиск по векторам. Дневник ограничен по размеру и
// инжектится целиком, когда сущность в сцене.
type Dossiers struct {
	db    *store.DB
	party string
}

func NewDossiers(db *store.DB, party string) *Dossiers {
	return &Dossiers{db: db, party: party}
}

// view — слитый взгляд: мировой слой плюс отношенческий поверх него.
// Отношенческий сильнее, потому что он про эту парти.
func (d *Dossiers) view(e store.EntityID) store.Dossier {
	out := store.Dossier{EntityID: e, PartyID: d.party, Kind: "npc"}
	if world, ok := d.db.WorldDossier(e); ok {
		out.Voice = world.Voice
		out.Life = world.Life
		out.Kind = world.Kind
		out.KnowsAbout = append(out.KnowsAbout, world.KnowsAbout...)
		out.TalksAbout = append(out.TalksAbout, world.TalksAbout...)
		out.Wants = append(out.Wants, world.Wants...)
	}
	if rel, ok := d.db.Dossiers[store.DossierKey{EntityID: e, PartyID: d.party}]; ok {
		out.Disposition = rel.Disposition
		out.Summary = rel.Summary
		out.OpenThreads = append(out.OpenThreads, rel.OpenThreads...)
		out.KnowsAbout = append(out.KnowsAbout, rel.KnowsAbout...)
		out.TalksAbout = append(out.TalksAbout, rel.TalksAbout...)
		if rel.Voice != "" {
			out.Voice = rel.Voice
		}
	}
	return out
}

// View — публичный слитый дневник сущности.
func (d *Dossiers) View(e store.EntityID) store.Dossier { return d.view(e) }

// Disposition — расположение к парти. Единственный источник: дневник.
// Держать его ещё и в Game значило бы иметь два места правды, а такую ошибку
// схема уже однажды допускала с ранениями.
func (d *Dossiers) Disposition(e store.EntityID) int {
	return d.db.DossierFor(e, d.party).Disposition
}

// Adjust меняет расположение. Возвращает новое значение.
func (d *Dossiers) Adjust(e store.EntityID, delta int) int {
	row := d.db.DossierFor(e, d.party)
	row.Disposition += delta
	row.Version++
	return row.Disposition
}

// Knows отвечает на механический вопрос: знает ли эта сущность этот факт.
// Знать не значит рассказать — условия выдачи задаёт fact_holders.gate.
func (d *Dossiers) Knows(e store.EntityID, f store.FactID) bool {
	for _, k := range d.view(e).KnowsAbout {
		if k == f {
			return true
		}
	}
	return false
}

// OpenThreads — незакрытое с парти, в стабильном порядке.
func (d *Dossiers) OpenThreads(e store.EntityID) []string {
	out := append([]string(nil), d.view(e).OpenThreads...)
	sort.Strings(out)
	return out
}

// Wants — чего персонаж хочет от парти, в стабильном порядке. Желание не
// вычитается по мере рассказа: человек не перестаёт хотеть оттого, что
// однажды попросил.
func (d *Dossiers) Wants(e store.EntityID) []string {
	out := append([]string(nil), d.view(e).Wants...)
	sort.Strings(out)
	return out
}

// TalksAbout — то, что сущность знает и ещё не рассказывала этой парти.
// Рассказанное вычитается: повтор звучит как заклинивший автомат.
func (d *Dossiers) TalksAbout(e store.EntityID) []string {
	told := map[string]bool{}
	for _, t := range d.db.DossierFor(e, d.party).Told {
		told[t] = true
	}
	var out []string
	for _, t := range d.view(e).TalksAbout {
		if !told[t] {
			out = append(out, t)
		}
	}
	sort.Strings(out)
	return out
}

// MarkTold помечает тему рассказанной этой парти.
func (d *Dossiers) MarkTold(e store.EntityID, note string) {
	row := d.db.DossierFor(e, d.party)
	for _, t := range row.Told {
		if t == note {
			return
		}
	}
	row.Told = append(row.Told, note)
	row.Version++
}

// Voice — голос из дневника либо из самой сущности. Дневник переопределяет:
// голос может меняться по ходу мира, сущность его лишь задаёт изначально.
func (d *Dossiers) Voice(e store.EntityID) string {
	if v := strings.TrimSpace(d.view(e).Voice); v != "" {
		return v
	}
	return d.db.Entities[e].Voice
}

// Life — быт персонажа из мирового слоя: авторская затравка, за которую
// разговор цепляется, не выдумывая. Пустая строка означает, что автор быта
// не написал.
func (d *Dossiers) Life(e store.EntityID) string {
	return strings.TrimSpace(d.view(e).Life)
}

// CloseThread снимает незакрытое дело: обещание выполнено, долг отдан.
func (d *Dossiers) CloseThread(e store.EntityID, thread string) bool {
	row := d.db.DossierFor(e, d.party)
	for i, t := range row.OpenThreads {
		if t == thread {
			row.OpenThreads = append(row.OpenThreads[:i], row.OpenThreads[i+1:]...)
			row.Version++
			return true
		}
	}
	return false
}

// historyCap — сколько кругов разговора персонаж помнит дословно. Память
// растит промпт каждым ходом, поэтому потолок обязателен; вытесненное не
// теряется, а сворачивается во впечатления.
const historyCap = 12

// summaryCap — потолок впечатлений в рунах. Свёрнутое тоже копится, и без
// предела долгий разговор переполнил бы промпт с другой стороны.
const summaryCap = 400

// foldSep разделяет свёрнутые круги внутри впечатлений. Резать впечатления
// по нему — единственный способ обрезать прозу, не рубя её посреди слова.
const foldSep = "; "

// Remember кладёт круг разговора в отношенческий слой. Дословность
// ограничена historyCap: самое старое сворачивается во впечатления.
func (d *Dossiers) Remember(e store.EntityID, player, reply string, turn int) {
	row := d.db.DossierFor(e, d.party)
	row.History = append(row.History, store.Exchange{
		Player: strings.TrimSpace(player),
		Reply:  strings.TrimSpace(reply),
		Turn:   turn,
	})
	if cut := len(row.History) - historyCap; cut > 0 {
		row.Summary = fold(row.Summary, row.History[:cut])
		row.History = append([]store.Exchange(nil), row.History[cut:]...)
	}
	row.Version++
}

// Recent — последние круги разговора в порядке, в котором они прозвучали.
func (d *Dossiers) Recent(e store.EntityID) []store.Exchange {
	row := d.db.DossierFor(e, d.party)
	return append([]store.Exchange(nil), row.History...)
}

// fold дописывает вытесненное во впечатления и держит их в пределах потолка.
//
// Переполнение вытесняет самое старое, включая авторские впечатления: у
// долгого разговора приоритет над первым знакомством, потому что игрок помнит
// именно его.
func fold(summary string, dropped []store.Exchange) string {
	parts := make([]string, 0, len(dropped)+1)
	if s := strings.TrimSpace(summary); s != "" {
		parts = append(parts, s)
	}
	for _, e := range dropped {
		if line := foldOne(e); line != "" {
			parts = append(parts, line)
		}
	}
	for len(parts) > 1 && len([]rune(strings.Join(parts, foldSep))) > summaryCap {
		parts = parts[1:]
	}
	out := strings.Join(parts, foldSep)
	if r := []rune(out); len(r) > summaryCap {
		out = string(r[len(r)-summaryCap:])
	}
	return out
}

// foldOne — один круг разговора в одну строку. Дословность здесь уже не
// нужна: впечатления это проза, а не истина.
func foldOne(e store.Exchange) string {
	player, reply := clip(e.Player), clip(e.Reply)
	switch {
	case player == "" && reply == "":
		return ""
	case player == "":
		return "он сказал: " + reply
	case reply == "":
		return "спрашивали: " + player
	}
	return "спрашивали: " + player + " — он: " + reply
}

// clip обрезает фразу до обозримой длины по границе слова.
func clip(s string) string {
	const limit = 48
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= limit {
		return s
	}
	cut := string(r[:limit])
	if i := strings.LastIndex(cut, " "); i > limit/2 {
		cut = cut[:i]
	}
	return strings.TrimSpace(cut) + "…"
}
