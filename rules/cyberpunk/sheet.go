// Package cyberpunk — Cyberpunk RED (R. Talsorian, 2020) как система правил
// за швом core.RuleSystem. Фаза A: STAT+SKILL+1d10 vs DV, HP по формуле
// BODY/WILL, Humanity как EMP×10, боёвка с SP-аблацией. Кость — d10 через
// core.Dice.Roll(1,10). Лист персонажа непрозрачен для ядра: разбирает его
// ParseSheet, читая RED-специфику.
package cyberpunk

import (
	"encoding/json"
	"strings"
)

// STATS RED: 10 характеристик 1–8. Именование словами русских клиентов не
// касается: STAT-ключи — «внутренний» язык листа, показ идёт через view.
const (
	StatINT  = "int"
	StatREF  = "ref"
	StatDEX  = "dex"
	StatTECH = "tech"
	StatCOOL = "cool"
	StatWILL = "will"
	StatLUCK = "luck"
	StatMOVE = "move"
	StatBODY = "body"
	StatEMP  = "emp"
)

// Sheet — лист RED-персонажа. Внешне общий с dnd5e.Sheet (encoding/json), но
// поля другие: STATS, а не ability scores, и вместо AC — SP брони.
//
// Humanity хранится вычислимо (EMP×10 − сумма Loss киберварья); отдельного
// поля нет, чтобы не завести второе место правды. EMP роняется падением
// Humanity ниже кратной десятки: EMP = Humanity / 10 (округл. вниз).
type Sheet struct {
	INT  int `json:"int"`
	REF  int `json:"ref"`
	DEX  int `json:"dex"`
	TECH int `json:"tech"`
	COOL int `json:"cool"`
	WILL int `json:"will"`
	LUCK int `json:"luck"`
	MOVE int `json:"move"`
	BODY int `json:"body"`
	EMP  int `json:"emp"`

	// Skills — уровни навыков по имени (RED-подмножество). Отсутствие ключа
	// значит 0, а не «не знает как»: чистая проверка без навыка всё равно
	// возможна (STAT + 1d10 vs DV) — за это отвечает check.
	Skills map[string]int `json:"skills"`

	// Cyberware — установленные импланты с их Humanity Loss. Сумма HL
	// определяет текущую Humanity (см. Humanity()). Kind — свободный тег
	// категории (neural, cybereye, cyberarm...), в фазе A не разбирается.
	Cyberware []Cyberware `json:"cyberware"`

	// Armor — броня по локациям тела; в фазе A одна общая SP-цифра. Каждый
	// пробитый удар аблирует её на 1 через мутацию (см. attack).
	SP int `json:"sp"`

	// Weapons — арсенал. Первое — активное (в фазе A так же, как у dnd5e:
	// выбор оружия — авторство кейса).
	Weapons []Weapon `json:"weapons"`

	// LuckSpent — сколько LUCK уже потрачено в текущей сессии. Обнуляется на
	// следующей сессии; в фазе A UI траты нет, поле — задел для §2.
	LuckSpent int `json:"luck_spent,omitempty"`

	// Role — Solo | Netrunner | ... — сохранённый архетип для показа/панели.
	// Абилка роли в фазе A — минимальный бонус: например, Combat Awareness
	// для Solo даёт +1 к атакам (упрощение фазы A; полная механика — §2).
	Role string `json:"role,omitempty"`
}

// Cyberware — один установленный имплант.
type Cyberware struct {
	Name string `json:"name"`
	Kind string `json:"kind,omitempty"`
	// HL — Humanity Loss, конкретное число, снимаемое с Humanity установкой.
	HL int `json:"hl"`
}

// Weapon — одно оружие. Damage — dice-строка «NdM» (RED: 2d6/3d6/5d6).
// Skill — навык, по которому бьёт оружие («handgun», «shoulder_arms»,
// «brawling»). Attack — какая STAT: почти всегда REF (за исключением
// специальных, которых в фазе A нет), поэтому дефолт REF, если поле пусто.
type Weapon struct {
	Name   string `json:"name"`
	Skill  string `json:"skill"`
	Attack string `json:"attack,omitempty"`
	Damage string `json:"damage"`
}

// ParseSheet разбирает JSON-представление RED-листа. Пустой вход —
// нулевой лист (тесты и минимальные фикстуры): не ошибка, а «нет данных».
func ParseSheet(raw json.RawMessage) (Sheet, error) {
	var s Sheet
	if len(raw) == 0 {
		return s, nil
	}
	return s, json.Unmarshal(raw, &s)
}

// Stat возвращает значение STAT по ключу. Неизвестная строка — 0 (не 10,
// как в dnd5e: у RED STAT не ability-score, и «нейтральный» бросок — без
// STAT-бонуса).
func (s Sheet) Stat(key string) int {
	switch strings.ToLower(key) {
	case StatINT:
		return s.INT
	case StatREF:
		return s.REF
	case StatDEX:
		return s.DEX
	case StatTECH:
		return s.TECH
	case StatCOOL:
		return s.COOL
	case StatWILL:
		return s.WILL
	case StatLUCK:
		return s.LUCK
	case StatMOVE:
		return s.MOVE
	case StatBODY:
		return s.BODY
	case StatEMP:
		return s.EMP
	}
	return 0
}

// Skill возвращает уровень навыка по имени. Отсутствие — 0.
func (s Sheet) Skill(name string) int {
	if s.Skills == nil {
		return 0
	}
	return s.Skills[strings.ToLower(name)]
}

// TotalHL — суммарный Humanity Loss установленного киберварья.
func (s Sheet) TotalHL() int {
	sum := 0
	for _, c := range s.Cyberware {
		sum += c.HL
	}
	return sum
}

// Humanity — текущая Humanity. Формула: EMP×10 − ΣHL. Не может уйти ниже 0.
func (s Sheet) Humanity() int {
	h := s.EMP*10 - s.TotalHL()
	if h < 0 {
		return 0
	}
	return h
}

// CurrentEMP — эффективная EMP: Humanity ÷ 10 (округл. вниз). Падение
// Humanity роняет EMP — этого требует RED.
func (s Sheet) CurrentEMP() int {
	return s.Humanity() / 10
}

// MaxHP — HP по формуле RED: ((BODY+WILL)/2, округл. вверх) × 5 + 10.
func MaxHP(body, will int) int {
	avg := (body + will + 1) / 2
	return avg*5 + 10
}

// MaxHP листа.
func (s Sheet) MaxHP() int { return MaxHP(s.BODY, s.WILL) }

// SeriouslyWounded — HP ≤ ½ макс (штраф −2 ко всем проверкам).
func SeriouslyWounded(hp, maxHP int) bool {
	if maxHP <= 0 {
		return false
	}
	return hp <= maxHP/2
}

// MortallyWounded — HP < 0. По RED смерть проверяется Death Save; статус
// сам по себе даёт штраф −4 к проверкам и −6 к MOVE.
func MortallyWounded(hp int) bool { return hp < 0 }

// DV — Difficulty Value по RED. Именно DV, а не «DC»: номенклатура правил
// сохраняется, чтобы в тестах и логах читалось честно.
const (
	DVEveryday     = 13
	DVDifficult    = 15
	DVProfessional = 17
	DVHeroic       = 21
	DVIncredible   = 24
)

// DefaultDV — сложность по умолчанию, если ни SceneView.DC, ни CheckDC не
// проставлены. Держим на «Difficult»: обыденная сложность вне гейта.
const DefaultDV = DVDifficult
