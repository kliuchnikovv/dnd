package server

import (
	"sync"

	"github.com/kliuchnikovv/dnd/store"
)

// CharacterStore — in-memory хранилище персонажей уровня сервера (не дела).
// MVP до login-flow: персонаж, сохранённый через Save, живёт до перезапуска
// процесса. Персистентность — отдельная задача вне этого плана.
type CharacterStore struct {
	mu   sync.Mutex
	byID map[string]*store.Character
}

// NewCharacterStore — пустое хранилище персонажей.
func NewCharacterStore() *CharacterStore {
	return &CharacterStore{byID: make(map[string]*store.Character)}
}

// Save кладёт персонажа в хранилище по его ID, перезаписывая прежнюю запись.
func (cs *CharacterStore) Save(c *store.Character) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.byID[string(c.ID)] = c
}

// ByID возвращает персонажа по ID. Второй результат ложен, если такого
// персонажа не сохраняли.
func (cs *CharacterStore) ByID(id string) (*store.Character, bool) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	c, ok := cs.byID[id]
	return c, ok
}
