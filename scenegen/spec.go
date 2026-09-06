// Package scenegen — пре-пасс генерации виньетки: тема → LLM → SceneSpec(DTO) →
// Repair → Validate → сцена ядра. Здесь живут DTO и детерминированные Repair/
// Validate; сам вызов LLM и маппинг в модель сцены виньетки — отдельно (LLM даёт
// битые сцены, поэтому валидатор обязателен, а починка гарантирует достижимый
// tell и вменяемые пороги). Правду генератор рождает как автор — она уходит
// ядру и стражу, не Мастеру (ADR-0008).
package scenegen

// TierSpec — один тир аспекта в DTO. Passive: 0 — активный (по броску), >0 —
// пассивный порог внимания (10+edge≈12).
type TierSpec struct {
	Text    string `json:"text"`
	Passive int    `json:"passive"`
	Grants  string `json:"grants"`
}

// ObjectSpec — видимый объект сцены: поверхность (без тайны) + упорядоченные
// тиры (shallow→deep). Aspect: look | listen | watch.
type ObjectSpec struct {
	ID      string     `json:"id"`
	Name    string     `json:"name"`
	Surface string     `json:"surface"`
	Hidden  bool       `json:"hidden"`
	Aspect  string     `json:"aspect"`
	Tiers   []TierSpec `json:"tiers"`
}

// HazardSpec — смертельная зона (топь/жернова): вход, глубины по нарастанию,
// гибель. nil, если в сцене её нет.
type HazardSpec struct {
	EnterText string   `json:"enter_text"`
	Depths    []string `json:"depths"`
	LoseText  string   `json:"lose_text"`
}

// SceneSpec — DTO сцены-виньетки, как её отдаёт генератор-LLM.
type SceneSpec struct {
	Title     string       `json:"title"`
	Intro     string       `json:"intro"`
	Truth     string       `json:"truth"`
	Mode      string       `json:"mode"` // traverse | disable | hold
	Ambient   string       `json:"ambient"`
	SafeNote  string       `json:"safe_note"`
	WinText   string       `json:"win_text"`
	LoseText  string       `json:"lose_text"`
	Goal      int          `json:"goal"`
	WinTarget string       `json:"win_target"`
	Hazard    *HazardSpec  `json:"hazard"`
	Beats     []string     `json:"beats"`
	Objects   []ObjectSpec `json:"objects"`
}
