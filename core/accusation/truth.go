// Package accusation изолирует cases.truth. Правильный ответ не покидает этот
// пакет иначе как одним булевым значением из Check.
package accusation

import (
	"errors"

	"github.com/kliuchnikovv/dnd/store"
)

var ErrTruthNotSerializable = errors.New("accusation: truth не подлежит сериализации")

// Truth хранит правильный ответ. Поля приватны, String редактирован,
// MarshalJSON отказывает — напечатать truth случайно нельзя.
type Truth struct {
	who, how, when, why store.Token
}

func NewTruth(who, how, when, why store.Token) Truth {
	return Truth{who: who, how: how, when: when, why: why}
}

func (Truth) String() string { return "<redacted>" }

func (Truth) MarshalJSON() ([]byte, error) { return nil, ErrTruthNotSerializable }

// Form — заполненная игроком форма обвинения.
type Form struct {
	Who  store.Token `json:"who"`
	How  store.Token `json:"how"`
	When store.Token `json:"when"`
	Why  store.Token `json:"why"`
}

func (f Form) Complete() bool {
	return f.Who != "" && f.How != "" && f.When != "" && f.Why != ""
}

// Check — целиком и символьно. Возврат один булев: игрок не узнаёт, какой
// слот ошибочен, иначе форма брутфорсится по слоту за раз.
func (t Truth) Check(f Form) bool {
	return f.Who == t.who && f.How == t.how && f.When == t.when && f.Why == t.why
}
