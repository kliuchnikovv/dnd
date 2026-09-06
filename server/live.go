package server

import "context"

// liveSession — общий контракт живой сессии для WS-слоя: и M1a (sessionRuntime),
// и виньетка (vignetteRuntime) его удовлетворяют, поэтому handleWS/readLoop
// работают против интерфейса, не зная, какой под ним движок. Методы совпадают с
// уже существующими у sessionRuntime — интерфейс лишь даёт им общее имя; сам
// sessionRuntime (server/runtime.go) не меняется.
type liveSession interface {
	attach() *subscriber
	unsubscribe(*subscriber)
	applyInput(ctx context.Context, frameID int, in inputPayload) (bool, string)
	stopProse()
	ChatID() string
	UserID() string
	nextOutID() int // берёт свой мьютекс сам
}

// ── адаптеры sessionRuntime (M1a) под liveSession. Живут ЗДЕСЬ, а не в
//    runtime.go, чтобы измерительная поверхность M1a осталась побайтово равна
//    pivot (ADR-0009). Семантика ровно та же, что у существующих методов. ──

func (rt *sessionRuntime) ChatID() string { return rt.chatID }
func (rt *sessionRuntime) UserID() string { return rt.userID }
func (rt *sessionRuntime) nextOutID() int {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.nextOutIDLocked()
}

// ── адаптеры vignetteRuntime под liveSession. ──

func (rt *vignetteRuntime) ChatID() string { return rt.chatID }
func (rt *vignetteRuntime) UserID() string { return rt.userID }
func (rt *vignetteRuntime) nextOutID() int {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.nextOutIDLocked()
}

// GetLive возвращает живую сессию любого трека по chat_id: сперва M1a-сессии,
// затем виньетка-сессии. Для WS-слоя, которому движок под сессией безразличен.
func (m *Manager) GetLive(chatID string) (liveSession, bool) {
	if rt, ok := m.Get(chatID); ok {
		return rt, true
	}
	if rt, ok := m.GetVignette(chatID); ok {
		return rt, true
	}
	return nil, false
}
