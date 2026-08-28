package core

import (
	"github.com/kliuchnikovv/dnd/naming"
	"github.com/kliuchnikovv/dnd/store"
)

// Маршрут свободной пробы к авторскому содержимому.
//
// Проба — это ввод, которого словарь не выразил. Приземляется она всегда, но
// сначала ядро отвечает на один вопрос: не назвал ли игрок своими словами ту
// самую цель, за которой автор что-то положил? Если назвал — это обычный ход,
// и авторский контент оказывается достижим словами, а не только командой.
//
// Не назвал — чистое повествование, и оно живёт над доменом: ядру описывать
// нечего.

// probeVerbs — глаголы, которыми проба может лечь на авторскую цель, в
// стабильном порядке. Порядок здесь не приоритет замысла, а лишь
// детерминированность перебора: решает всё равно гейт автора, а он у цели
// обычно один.
var probeVerbs = []Verb{"examine", "search", "stake_out", "tail"}

// MatchProbe сообщает, легла ли проба на авторскую цель этого узла, и каким
// ходом её резолвить.
//
// Глагол берётся ИЗ ГЕЙТА АВТОРА, а не угадывается по тексту пробы. Угадывание
// было бы подменой: «принюхиваюсь к бочкам» стало бы осмотром бочек, игра
// сделала бы не то, что заявлено, и не сказала бы об этом. Гейт же отвечает на
// честный вопрос — что вообще даёт возня с этой целью.
//
// Функция только читает. Ход из неё применяет вызывающий, обычным Apply: второй
// двери в состояние здесь не появляется.
func (g *Game) MatchProbe(text string) (Intent, bool) {
	id, ok := naming.Resolve(text, g.probeTargets())
	if !ok {
		return Intent{}, false
	}
	for _, v := range probeVerbs {
		in := Intent{Verb: v, Actor: g.Actor, Args: Args{Target: store.EntityID(id)}}
		if _, found := g.holderFor(in); !found {
			continue
		}
		if g.Check(in).Refused {
			continue
		}
		return in, true
	}
	return Intent{}, false
}

// probeTargets — то, на что проба вправе лечь: детали места и неживые
// сущности узла.
//
// Людей здесь нет намеренно. К людям обращаются, а не пробуют: гейт человека
// стоит на вопросе, и превратить «заглядываю Берну в глаза» в допрос значило бы
// выбрать за игрока механику, которой он не заявлял. Речь и обращение — работа
// разбора, и у них свои исходы.
func (g *Game) probeTargets() []naming.Candidate {
	var out []naming.Candidate
	for _, e := range g.DB.EntitiesAt(g.Node) {
		if e.Kind == store.EntityNPC {
			continue
		}
		out = append(out, naming.Candidate{ID: string(e.ID), Name: e.Name})
	}
	for _, p := range g.DB.Props[g.Node] {
		out = append(out, naming.Candidate{ID: string(p.ID), Name: p.Name})
	}
	return out
}
