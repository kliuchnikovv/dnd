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
		out.Kind = world.Kind
		out.KnowsAbout = append(out.KnowsAbout, world.KnowsAbout...)
		out.TalksAbout = append(out.TalksAbout, world.TalksAbout...)
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

// TalksAbout — то, о чём сущность заговорит сама.
func (d *Dossiers) TalksAbout(e store.EntityID) []string {
	out := append([]string(nil), d.view(e).TalksAbout...)
	sort.Strings(out)
	return out
}

// Voice — голос из дневника либо из самой сущности. Дневник переопределяет:
// голос может меняться по ходу мира, сущность его лишь задаёт изначально.
func (d *Dossiers) Voice(e store.EntityID) string {
	if v := strings.TrimSpace(d.view(e).Voice); v != "" {
		return v
	}
	return d.db.Entities[e].Voice
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
