// Package propose — капабилити-гейт над доменом. Тонкий слой между теми, кто
// предлагает, и ядром, которое располагает (ADR-0001).
//
// Живёт он отдельным пакетом, а не в ядре, по одной причине: ядро не знает об
// LLM и знать не должно. Роли, полномочия и реестр капабилити — материал слоя
// над доменом, и проверка «эта роль вправе такое предлагать» обязана стоять
// здесь. Ядро проверяет другое — само изменение.
//
// Порядок жёсткий: сначала капабилити, потом инвариант. Пройти обязаны оба, и
// собраны они в одну функцию именно поэтому — два вызова в вызывающем коде
// когда-нибудь разошлись бы, и разошлись бы молча.
package propose

import (
	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/llm"
)

// RefusalNotAllowed — отказ капабилити-гейта. Одна строка на все случаи
// нарочно: предложителю незачем знать, чем именно он не вышел, а разбору
// хватает роли и вида из самой записи отказа.
const RefusalNotAllowed = "роль не вправе предлагать такое изменение"

// needs — какое полномочие требует вид мутации. Виды, которых здесь нет,
// не предлагает никто: числовые мутации — прерогатива системы правил, и
// отсутствие записи это и означает.
var needs = map[core.MutationKind]llm.ProposalKind{
	core.MutCanonAmbient: llm.ProposeCanonAmbient,
	core.MutWorldEvent:   llm.ProposeWorldMutation,
}

// Allowed — вправе ли роль предлагать такой вид изменения. Ответ выводится из
// реестра капабилити, а не из второго списка ролей: второй список отставал бы
// от полномочий.
//
// Незаявленная роль получает пустые полномочия — молчание реестра означает
// «ничего не предлагает», а не «можно всё».
func Allowed(role llm.Role, kind core.MutationKind) bool {
	need, ok := needs[kind]
	if !ok {
		return false
	}
	for _, have := range llm.Capabilities[role].Proposes {
		if have == need {
			return true
		}
	}
	return false
}

// Mutation проводит предложение роли через оба гейта и возвращает вердикт
// ядра. Отказ капабилити-гейта до ядра не доходит: непозволенное предложение
// не должно даже иметь шанса пройти инвариант.
func Mutation(g *core.Game, role llm.Role, m core.Mutation) (core.Applied, core.Refusal) {
	if !Allowed(role, m.Kind) {
		return core.Applied{}, core.Refusal{Kind: m.Kind, Reason: RefusalNotAllowed}
	}
	return g.ProposeMutation(m)
}
