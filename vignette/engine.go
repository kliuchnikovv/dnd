package vignette

import (
	"fmt"
	"strings"
)

// Result — что ядро отдаёт наружу за ход. Мастеру уходят только Revealed +
// нейтральные строки; ни правды, ни закрытых тиров, ни числа-до-броска.
type Result struct {
	Roll      *RollInfo
	RollLine  string
	Revealed  []string
	StateNote string
	Ended     bool
	EndText   string
	Beat      string // hold: беат эскалации этого хода — событие для Мастера
}

// RollInfo — бросок хода (числа — для лога/эвала, не для Мастера).
type RollInfo struct {
	D20, Mod, DC, Total, Margin int
	Line                        string
}

// rollMode — честный d20 с преимуществом/помехой (2d20 бери больше/меньше);
// строка броска нейтральна: не называет, ОТ ЧЕГО защищает проверка.
func (st *State) rollMode(mod, dc, adv int) RollInfo {
	a := st.dice.Roll(1, 20)
	pick := a
	other := 0
	if adv != 0 {
		other = st.dice.Roll(1, 20)
		if adv > 0 && other > pick {
			pick = other
		}
		if adv < 0 && other < pick {
			pick = other
		}
	}
	total := pick + mod
	m := total - dc
	line := fmt.Sprintf("бросок: d20=%d%+d = %d против %d · %+d", pick, mod, total, dc, m)
	switch {
	case adv > 0:
		line = fmt.Sprintf("бросок (преимущество): d20=%d/%d→%d%+d = %d против %d · %+d", a, other, pick, mod, total, dc, m)
	case adv < 0:
		line = fmt.Sprintf("бросок (помеха): d20=%d/%d→%d%+d = %d против %d · %+d", a, other, pick, mod, total, dc, m)
	}
	return RollInfo{D20: pick, Mod: mod, DC: dc, Total: total, Margin: m, Line: line}
}

func (st *State) roll(mod, dc int) RollInfo { return st.rollMode(mod, dc, 0) }

// depthFromMargin — bandwidth: сколько активных тиров открыть по марже.
func depthFromMargin(m int) int {
	switch {
	case m < 0:
		return 0
	case m < 5:
		return 1
	default:
		return 2
	}
}

// passiveScore — пассивное внимание (D&D passive check): 10 + edge + 5*adv.
func (st *State) passiveScore(adv int) int { return 10 + st.Edge + 5*adv }

func (st *State) gateOK(g Gate) bool {
	switch g.Kind {
	case GateOpen, GateCheck:
		return true // Check отгейчен bandwidth'ом
	case GateAction:
		return st.Actions[g.Key]
	case GateKnowledge:
		return st.Facts[g.Key]
	}
	return false
}

func (st *State) take(t Tier, out *[]string) {
	if st.shown[t.Text] {
		return
	}
	st.shown[t.Text] = true
	*out = append(*out, t.Text)
	if t.Grants != "" {
		st.Facts[t.Grants] = true
	}
}

func aspectHasActive(obj *Object, aspect string) bool {
	a, ok := obj.Aspects[aspect]
	if !ok {
		return false
	}
	for _, t := range a.Tiers {
		if t.Passive == 0 {
			return true
		}
	}
	return false
}

// revealPerception — пассивные тиры открываются вниманием без кости и лока;
// активные — по броску (bandwidth-глубина от маржи), и только если rolled.
func (st *State) revealPerception(obj *Object, aspect string, rolled bool, margin, adv int, out *[]string) {
	a, ok := obj.Aspects[aspect]
	if !ok {
		return
	}
	depth := depthFromMargin(margin)
	passive := st.passiveScore(adv)
	activeIdx := 0
	for _, t := range a.Tiers {
		if !st.gateOK(t.Gate) {
			continue
		}
		if t.Passive > 0 {
			if passive >= t.Passive {
				st.take(t, out)
			}
			continue
		}
		if rolled && activeIdx < depth {
			st.take(t, out)
		}
		activeIdx++
	}
}

// revealUnlocked — тиры за действием/знанием, ставшие доступными (правда
// последствием). Bandwidth к ним не применяется.
func (st *State) revealUnlocked(sc *Scene, out *[]string) {
	for _, id := range allObjectIDs(sc) {
		for _, a := range sc.Objects[id].Aspects {
			for _, t := range a.Tiers {
				if (t.Gate.Kind == GateAction || t.Gate.Kind == GateKnowledge) && st.gateOK(t.Gate) {
					st.take(t, out)
				}
			}
		}
	}
}

func perceptionAspect(obj *Object, kind string) string {
	if kind == "listen" {
		if _, ok := obj.Aspects["listen"]; ok {
			return "listen"
		}
	}
	for _, name := range []string{"look", "watch", "listen"} {
		if _, ok := obj.Aspects[name]; ok {
			return name
		}
	}
	for name := range obj.Aspects {
		if name != "chase" {
			return name
		}
	}
	return ""
}

// climbDC — сложность выбраться, ФУНКЦИЯ ПОЗИЦИИ (своя каноничная шкала):
// по колено easy(10), по пояс medium(15), по грудь hard(20).
func climbDC(mire int) int {
	switch mire {
	case 1:
		return DCEasy
	case 2:
		return DCMedium
	default:
		return DCHard
	}
}

// maybeFumble — сильный промах КУСАЕТ (fail-forward по марже, не магия единицы):
// margin ≤ -10 в сцене с опасной зоной — оступаешься и соскальзываешь к беде.
func (st *State) maybeFumble(sc *Scene, ri RollInfo, res *Result) {
	if ri.Margin <= -10 && sc.Hazard != nil && !st.OffPath && st.MireDepth == 0 {
		st.OffPath = true
		st.MireDepth = 1
		res.StateNote = "Ты оступаешься и соскальзываешь с твёрдого. " + physicalNote(sc, st)
	}
}

// attemptClimb — попытка выбраться из топи телом. Успех — на гать; провал
// глубже; провал с грудью — тонешь (ход всегда что-то решает).
func (st *State) attemptClimb(sc *Scene, res *Result) {
	ri := st.roll(st.Body, climbDC(st.MireDepth))
	res.Roll, res.RollLine = &ri, ri.Line
	switch {
	case ri.Margin >= 0:
		st.OffPath = false
		st.MireDepth = 0
	case st.MireDepth < 3:
		st.MireDepth++
	default:
		res.Ended = true
		res.EndText = sc.Hazard.LoseText
	}
}

// Adjudicate исполняет вердикт судьи детерминированно: катит кость по заданным
// DC/стату, применяет гейты/bandwidth/пассив, двигает состояние, решает исход по
// режиму и пушит беат hold'а. Само не решает «что за действие» — это работа судьи.
func (st *State) Adjudicate(sc *Scene, r Ruling) Result {
	var res Result
	var out []string

	mod := st.Edge
	if r.Stat == "body" {
		mod = st.Body
	}
	inHazard := sc.Hazard != nil && (st.OffPath || st.MireDepth > 0)

	switch r.Kind {

	case "look", "listen", "search":
		if r.Kind == "search" {
			for id, o := range sc.Objects {
				if o.Hidden && !st.Discovered[id] {
					st.Discovered[id] = true
				}
			}
		}
		obj := sc.Objects[r.Target]
		if obj == nil && len(sc.Order) > 0 {
			obj = sc.Objects[sc.Order[0]]
		}
		if obj != nil {
			aspect := perceptionAspect(obj, r.Kind)
			key := obj.ID + "|" + aspect
			switch {
			case st.Tried[key]:
				st.revealPerception(obj, aspect, false, 0, r.Adv, &out)
				if len(out) == 0 {
					res.StateNote = "Ты уже разобрал тут всё, что мог."
				}
			case aspectHasActive(obj, aspect):
				ri := st.rollMode(mod, r.DC, r.Adv)
				res.Roll, res.RollLine = &ri, ri.Line
				st.revealPerception(obj, aspect, true, ri.Margin, r.Adv, &out)
				if ri.Margin >= 0 {
					st.Tried[key] = true // лок ТОЛЬКО на успехе; провал — можно позже
				}
				st.maybeFumble(sc, ri, &res)
			default:
				st.revealPerception(obj, aspect, false, 0, r.Adv, &out)
				if len(out) == 0 {
					res.StateNote = "Ты уже разобрал тут всё, что мог."
				}
			}
		}

	case "moveoff":
		if sc.Hazard == nil {
			// нет опасной зоны (hold): «сойти/открыть/впустить» — роковой шаг
			res.Ended, res.EndText = true, sc.LoseText
			return res
		}
		st.OffPath = true
		st.Actions["step_off"] = true
		ri := st.rollMode(st.Body, r.DC, r.Adv)
		res.Roll, res.RollLine = &ri, ri.Line
		switch m := ri.Margin; {
		case m >= 5:
			st.MireDepth = max(st.MireDepth, 1)
		case m >= 0:
			st.MireDepth = max(st.MireDepth, 2)
		default:
			st.MireDepth = min(3, st.MireDepth+2)
		}
		st.revealUnlocked(sc, &out)

	case "moveon":
		if inHazard {
			st.attemptClimb(sc, &res)
			if res.Ended {
				return res
			}
		} else if sc.Mode == ModeTraverse {
			st.Progress++
		}

	case "recall":
		res.StateNote = inventoryNote(st) + " " + physicalNote(sc, st)

	case "improvise":
		// Игрок вносит ВЕЩЬ обстановки (щедро), но БЕЗ содержания/фактов: знание —
		// только осмотром авторских тиров. Грант становится правдой ядра (в
		// инвентарь). Письменный предмет приходит пустым.
		item := strings.TrimSpace(r.Item)
		switch {
		case r.Admit == "deny" || item == "":
			res.StateNote = "Ничего похожего под рукой нет. " + physicalNote(sc, st)
		default:
			if !hasItem(st, item) {
				st.Items = append(st.Items, item)
			}
			if isWritingItem(item) {
				res.StateNote = "Ты берёшь: " + item + " — но разобрать на нём нечего, пусто. " + physicalNote(sc, st)
			} else {
				res.StateNote = "Ты берёшь: " + item + ". " + physicalNote(sc, st)
			}
		}

	case "idle":
		// Мета/служебное/бессмыслица — персонаж ничего не предпринимает. Никаких
		// мутаций и уж точно не роковой шаг (страховка против «инъекция→проигрыш»).
		res.StateNote = "Ты медлишь, ничего не предпринимая. " + physicalNote(sc, st)

	case "call":
		res.StateNote = "Твои слова уходят в темноту без ответа."

	default: // interact / probe — свободное физическое действие; НЕ тупик
		switch {
		case inHazard:
			st.attemptClimb(sc, &res)
			if res.Ended {
				return res
			}
		case sc.Mode == ModeDisable && r.Target == sc.WinTarget && len(st.Items) > 0:
			st.Facts["disabled"] = true
		default:
			ri := st.rollMode(mod, r.DC, r.Adv)
			res.Roll, res.RollLine = &ri, ri.Line
			res.StateNote = "Ты пробуешь — мир отзывается, но ничего не сдвигается."
			st.maybeFumble(sc, ri, &res)
		}
	}

	// победа/провал по режиму (если ещё не случилось выше)
	if !res.Ended {
		switch {
		case sc.Mode == ModeTraverse && st.Progress >= sc.Goal:
			res.Ended, res.EndText = true, sc.WinText
		case sc.Mode == ModeDisable && st.Facts["disabled"]:
			res.Ended, res.EndText = true, sc.WinText
		case sc.Mode == ModeHold && st.Turn >= sc.Goal:
			res.Ended, res.EndText = true, sc.WinText
		}
	}

	// hold: пушим беат эскалации этого хода как СОБЫТИЕ (не за проверкой)
	if sc.Mode == ModeHold && !res.Ended && len(sc.Beats) > 0 {
		idx := st.Turn - 1
		if idx < 0 {
			idx = 0
		}
		if idx >= len(sc.Beats) {
			idx = len(sc.Beats) - 1
		}
		res.Beat = sc.Beats[idx]
	}

	res.Revealed = out
	res.StateNote = firstNonEmpty(res.StateNote, physicalNote(sc, st))
	return res
}

// ProtectedFacts — что игрок ещё НЕ заслужил: скрытая правда + все ещё НЕ
// показанные тиры. Для стража-редактора (ADR-0008): держит правду, режет утечку.
func ProtectedFacts(sc *Scene, st *State) []string {
	out := []string{}
	if sc.Truth != "" {
		out = append(out, sc.Truth)
	}
	for _, id := range allObjectIDs(sc) {
		for _, a := range sc.Objects[id].Aspects {
			for _, t := range a.Tiers {
				if !st.shown[t.Text] {
					out = append(out, t.Text)
				}
			}
		}
	}
	return out
}

// StateFacts — текущее положение (чтобы страж ловил противоречия состоянию).
func StateFacts(sc *Scene, st *State) []string {
	return []string{physicalNote(sc, st)}
}
