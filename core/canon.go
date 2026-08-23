package core

import (
	"strings"

	"github.com/kliuchnikovv/dnd/store"
)

// Канон мира — ambient-детали, которых в авторской затравке не было. Их
// решает Мастер по запросу, движок только запоминает.
//
// Ядро здесь ничего не решает и ни к кому не звонит: оно держит слово. Один
// раз решённая деталь возвращается той же, и потому «расширение мира» не
// становится генератором противоречий. Факты дела сюда не попадают никогда —
// их гейтит fact_holders, и импровизацией они не канонизируются.

// canonPut канонизирует деталь и возвращает действующий канон темы.
//
// Неэкспортирован намеренно: снаружи домена канон пишется только через
// ProposeMutation, где тема проверяется на пространство имён фактов дела, а
// конфликт с решённым получает отказ. Экспортированный писатель был бы второй
// дверью — без обеих проверок.
//
// Первый
// ответ выигрывает: перезапись означала бы, что мир меняется под каждый
// вопрос, а именно от этого канон и защищает.
func (g *Game) canonPut(topic, text string, turn int) string {
	key := canonTopic(topic)
	text = strings.TrimSpace(text)
	if key == "" || text == "" {
		return ""
	}
	if have, ok := g.DB.Canon[store.CanonKey{CaseID: g.CaseID, Topic: key}]; ok {
		return have.Text
	}
	g.DB.Canon[store.CanonKey{CaseID: g.CaseID, Topic: key}] = store.CanonFact{
		CaseID: g.CaseID, Topic: key, Text: text, Turn: turn,
	}
	return text
}

// CanonGet читает канон темы. Чтение точное: совпадение по нормализованной
// теме, а не поиск по смыслу.
func (g *Game) CanonGet(topic string) (string, bool) {
	f, ok := g.DB.Canon[store.CanonKey{CaseID: g.CaseID, Topic: canonTopic(topic)}]
	return f.Text, ok
}

// Canon — весь канон этого дела в стабильном порядке.
func (g *Game) Canon() []store.CanonFact { return g.DB.CanonOf(g.CaseID) }

// canonTopic нормализует тему: регистр и лишние пробелы — это один и тот же
// вопрос, и второй ответ на него был бы вторым каноном.
func canonTopic(topic string) string {
	return strings.Join(strings.Fields(strings.ToLower(topic)), " ")
}
