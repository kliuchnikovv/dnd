package scenegen

import "strings"

// Validate — жёсткие структурные инварианты. Пустой результат — сцена
// пригодна; иначе список ошибок, по которым вызывающий ретраит генерацию
// (≤3 раза). LLM даёт битые сцены — это обязательный барьер, не косметика.
func Validate(s *SceneSpec) []string {
	var e []string
	req := func(v, name string) {
		if strings.TrimSpace(v) == "" {
			e = append(e, "пустое поле "+name)
		}
	}
	req(s.Title, "title")
	req(s.Intro, "intro")
	req(s.Truth, "truth")
	req(s.Ambient, "ambient")
	req(s.SafeNote, "safe_note")
	req(s.WinText, "win_text")

	switch s.Mode {
	case "traverse":
		if s.Goal < 1 {
			e = append(e, "traverse: goal должен быть ≥1")
		}
		if s.Hazard == nil {
			e = append(e, "traverse: нужен hazard (сход с пути = гибель)")
		}
	case "disable":
		if strings.TrimSpace(s.WinTarget) == "" {
			e = append(e, "disable: пустой win_target")
		}
		if s.Hazard == nil {
			e = append(e, "disable: нужен hazard (опасная зона)")
		}
	case "hold":
		if s.Goal < 1 {
			e = append(e, "hold: goal (ходов до рассвета) ≥1")
		}
		if len(s.Beats) == 0 {
			e = append(e, "hold: нужны beats (эскалация по ходам)")
		}
		req(s.LoseText, "lose_text (hold: роковой шаг)")
	default:
		e = append(e, "mode должен быть traverse|disable|hold, получено: "+s.Mode)
	}

	if s.Hazard != nil {
		req(s.Hazard.EnterText, "hazard.enter_text")
		req(s.Hazard.LoseText, "hazard.lose_text")
		if len(s.Hazard.Depths) == 0 {
			e = append(e, "hazard.depths пуст (нужна хотя бы одна глубина)")
		}
	}

	if len(s.Objects) < 2 {
		e = append(e, "нужно ≥2 объектов")
	}
	ids := map[string]bool{}
	for _, o := range s.Objects {
		id := strings.TrimSpace(o.ID)
		if id == "" {
			e = append(e, "объект без id")
			continue
		}
		if ids[id] {
			e = append(e, "повтор id объекта: "+id)
		}
		ids[id] = true
		req(o.Surface, "surface объекта "+id)
		if len(o.Tiers) == 0 {
			e = append(e, "у объекта "+id+" нет тиров")
		}
	}
	if s.Mode == "disable" && s.WinTarget != "" && !ids[s.WinTarget] {
		e = append(e, "win_target ссылается на несуществующий объект: "+s.WinTarget)
	}
	return e
}
