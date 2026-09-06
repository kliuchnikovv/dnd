package vignette

import (
	"strings"

	"github.com/kliuchnikovv/dnd/core"
)

// State — состояние прохождения виньетки. Инвентарь и позиция — правда движка,
// не выдумка Мастера. Dice — честный d20 из ядра (RNG вне Мастера).
type State struct {
	Edge, Body int
	Items      []string
	OffPath    bool
	MireDepth  int // 0 нет · 1..3 глубина опасной зоны
	Progress   int // traverse: шаги к краю
	Turn       int // сколько ходов сделано (hold: достоять до Goal)
	Actions    map[string]bool
	Facts      map[string]bool
	Discovered map[string]bool
	Tried      map[string]bool // аспекты, уже успешно проверенные (лок-на-успехе)
	shown      map[string]bool // тексты, уже показанные (дедуп)
	dice       core.Dice
}

// NewState — стартовое состояние. dice — честная кость ядра (dice.Source в
// проде, dice.Fixed в тестах).
func NewState(d core.Dice) *State {
	return &State{
		Edge: 2, Body: 0,
		Items:      []string{"охотничий нож", "моток верёвки"},
		Actions:    map[string]bool{},
		Facts:      map[string]bool{},
		Discovered: map[string]bool{},
		Tried:      map[string]bool{},
		shown:      map[string]bool{},
		dice:       d,
	}
}

func hasItem(st *State, item string) bool {
	for _, x := range st.Items {
		if strings.EqualFold(x, item) {
			return true
		}
	}
	return false
}

// isWritingItem — предмет-носитель текста (записка/письмо/книга…): вносится
// пустым, чтобы игрок не мог внести СВЕДЕНИЯ, только вещь.
func isWritingItem(item string) bool {
	s := strings.ToLower(item)
	for _, k := range []string{"записк", "письм", "книг", "свит", "грамот", "надпис",
		"табличк", "дневник", "бумаг", "страниц", "заметк", "послани", "листок", "лист бумаг"} {
		if strings.Contains(s, k) {
			return true
		}
	}
	return false
}

// looksLikeMeta — ввод похож на служебное/мета/инъекцию (не действие в мире).
// Страховка движка: мета-текст НЕ может стать роковым шагом (прото §2.12).
func looksLikeMeta(text string) bool {
	s := strings.ToLower(text)
	for _, k := range []string{"ignore previous", "ignore all", "previous instruction",
		"забудь", "инструкц", "system", "verdict", "kind=", "admit=", "\"kind\"",
		"переписыв", "ты ассистент", "ты обычный", "prompt"} {
		if strings.Contains(s, k) {
			return true
		}
	}
	return false
}

func inventoryNote(st *State) string {
	if len(st.Items) == 0 {
		return "При себе у тебя ничего нет."
	}
	return "При себе (точный список, отвечай строго по нему): " +
		strings.Join(st.Items, ", ") + " — и больше ничего."
}

func physicalNote(sc *Scene, st *State) string {
	if sc.Hazard != nil {
		if st.MireDepth >= 1 && st.MireDepth <= len(sc.Hazard.Depths) {
			return sc.Hazard.Depths[st.MireDepth-1]
		}
		if st.OffPath {
			return sc.Hazard.EnterText
		}
	}
	return sc.SafeNote
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
