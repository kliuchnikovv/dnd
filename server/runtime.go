package server

import (
	"github.com/kliuchnikovv/dnd/cli"
	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/view"
)

// turnViewVersion — версия формы turn-view, которую отдаёт сервер. Совпадает с
// той, что проставляет view.Build; сервер лишь пересылает.

// Ruleset/Scenario серверного вида — референсные реализации из cli: меры от
// правила порога, панель цели детективного досье. Гейм-логики в них нет, это
// проекция состояния ядра; сервер берёт их как есть, чтобы вид совпадал с CLI.
var (
	serverRuleset  view.Ruleset  = cli.RefRuleset{}
	serverScenario view.Scenario = cli.Detective{}
)

// subscribe регистрирует сокет и возвращает его. Вызывается при (ре)коннекте.
func (rt *sessionRuntime) subscribe() *subscriber {
	sub := &subscriber{out: make(chan Frame, 64)}
	rt.mu.Lock()
	rt.subs[sub] = struct{}{}
	rt.mu.Unlock()
	return sub
}

// unsubscribe снимает сокет с рассылки. Идемпотентна.
func (rt *sessionRuntime) unsubscribe(sub *subscriber) {
	rt.mu.Lock()
	delete(rt.subs, sub)
	rt.mu.Unlock()
}

// broadcast рассылает кадр всем подписчикам. Отставший читатель (полный буфер)
// пропускается, а не блокирует ход: его вылечит реконнект с досстримом.
// Держит rt.mu, поэтому кадр собирается заранее.
func (rt *sessionRuntime) broadcastLocked(f Frame) {
	for sub := range rt.subs {
		select {
		case sub.out <- f:
		default:
		}
	}
}

// nextOutID выдаёт id для серверного кадра. Под rt.mu.
func (rt *sessionRuntime) nextOutIDLocked() int {
	rt.outSeq++
	return rt.outSeq
}

// snapshotView собирает session_state текущего состояния — без применения хода.
// Нужен при (ре)коннекте: подписчик сразу видит, где игра.
func (rt *sessionRuntime) snapshotView() Frame {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	tv := view.Build(rt.game, core.TurnResult{}, nil, serverRuleset, serverScenario, rt.spokenTo)
	return newFrame(rt.nextOutIDLocked(), rt.chatID, ChannelChat,
		KindData, OpSessionState, tv)
}

// applyInput разворачивает ввод игрока в интент показанного набора, применяет
// его ядром и рассылает получившийся session_state. Возвращает false и текст
// ошибки, если ввод не разворачивается в валидный вариант.
//
// Модели здесь нет: и токен, и номер ссылаются на аффорданс, у которого интент
// уже собран ядром. Свободный NL-текст — отдельный путь фазы LLM.
//
// frameID — id входящего кадра: повтор (id <= lastApplied) не применяет ход
// дважды, но всё равно пере-отдаёт текущий session_state, чтобы дубль доставки
// оставлял клиента в согласованном состоянии.
func (rt *sessionRuntime) applyInput(frameID int, in inputPayload) (bool, string) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	if frameID != 0 && frameID <= rt.lastAppliedID {
		// Дубль доставки: ход уже применён. Пере-отдаём состояние, не трогая
		// ядро.
		rt.broadcastLocked(rt.snapshotViewLocked())
		return true, ""
	}

	offered := rt.game.Affordances(rt.spokenTo)
	intent, ok := expand(offered, in)
	if !ok {
		return false, "ход не разворачивается в показанный вариант"
	}
	intent.Actor = rt.game.Actor

	res := rt.game.Apply(intent)
	// Адресат хода продолжает разговор: следующий набор аффордансов строится
	// от него. Пусто — вышли из разговора.
	if intent.Args.Target != "" {
		rt.spokenTo = intent.Args.Target
	}
	if frameID != 0 {
		rt.lastAppliedID = frameID
	}

	tv := view.Build(rt.game, res, nil, serverRuleset, serverScenario, rt.spokenTo)
	rt.broadcastLocked(newFrame(rt.nextOutIDLocked(), rt.chatID, ChannelChat,
		KindData, OpSessionState, tv))
	return true, ""
}

// snapshotViewLocked — как snapshotView, но под уже взятым rt.mu.
func (rt *sessionRuntime) snapshotViewLocked() Frame {
	tv := view.Build(rt.game, core.TurnResult{}, nil, serverRuleset, serverScenario, rt.spokenTo)
	return newFrame(rt.nextOutIDLocked(), rt.chatID, ChannelChat,
		KindData, OpSessionState, tv)
}

// expand сопоставляет ввод с показанным набором: токен — по стабильному
// OptionToken, текст — по номеру варианта (1..N). Один путь на оба входа, как
// в CLI: токен и номер разворачиваются в тот же интент.
func expand(offered []core.Affordance, in inputPayload) (core.Intent, bool) {
	if in.Token != "" {
		for _, a := range offered {
			if view.OptionToken(a) == in.Token {
				return a.Intent, true
			}
		}
		return core.Intent{}, false
	}
	if n, ok := parseChoice(in.Text); ok {
		if n >= 1 && n <= len(offered) {
			return offered[n-1].Intent, true
		}
	}
	return core.Intent{}, false
}

// parseChoice читает номер варианта из текста. Свободный NL сюда не попадает:
// он уходит в LLM-парсер (фаза 4). Пустая строка и не-число — не выбор.
func parseChoice(text string) (int, bool) {
	n := 0
	seen := false
	for _, r := range text {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			if seen {
				break
			}
			continue
		}
		if r < '0' || r > '9' {
			return 0, false
		}
		n = n*10 + int(r-'0')
		seen = true
	}
	if !seen {
		return 0, false
	}
	return n, true
}
