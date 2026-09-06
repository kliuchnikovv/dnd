package vignette

import "github.com/kliuchnikovv/dnd/scenegen"

// FromSpec переводит валидированный DTO генератора в играбельную сцену движка.
// Правда (Truth) уходит В СЦЕНУ (ядро/страж), не Мастеру — анти-утечка цела.
// Вызывать ПОСЛЕ scenegen.Repair + scenegen.Validate: битую сцену не пускаем.
func FromSpec(s *scenegen.SceneSpec) *Scene {
	sc := &Scene{
		Title: s.Title, Intro: s.Intro, Truth: s.Truth,
		Mode: s.Mode, Goal: s.Goal, WinTarget: s.WinTarget,
		SafeNote: s.SafeNote, Ambient: s.Ambient,
		WinText: s.WinText, LoseText: s.LoseText,
		Beats:   s.Beats,
		Objects: map[string]*Object{},
	}
	if s.Hazard != nil {
		sc.Hazard = &Hazard{EnterText: s.Hazard.EnterText, Depths: s.Hazard.Depths, LoseText: s.Hazard.LoseText}
	}
	for _, o := range s.Objects {
		tiers := make([]Tier, 0, len(o.Tiers))
		for _, t := range o.Tiers {
			tiers = append(tiers, Tier{Gate: Gate{GateCheck, ""}, Text: t.Text, Grants: t.Grants, Passive: t.Passive})
		}
		sc.Objects[o.ID] = &Object{
			ID: o.ID, Name: o.Name, Surface: o.Surface, Hidden: o.Hidden,
			Aspects: map[string]Aspect{o.Aspect: {Tiers: tiers}},
		}
		if !o.Hidden { // скрытые — не на поверхности, открываются обыском
			sc.Order = append(sc.Order, o.ID)
		}
	}
	return sc
}
