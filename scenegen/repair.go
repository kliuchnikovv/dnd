package scenegen

import "strings"

// maxReachablePassive — верхняя граница «достижимого» пассивного порога: средний
// персонаж (10+edge) должен tell брать. Выше — притягиваем к достижимому.
const maxReachablePassive = 12

// Repair — мягкая починка спека (пороги, аспекты, гарантия достижимого tell).
// Не роняет сцену: правит то, что можно, оставляя жёсткие нарушения валидатору.
func Repair(s *SceneSpec) {
	s.Mode = strings.ToLower(strings.TrimSpace(s.Mode))
	for i := range s.Objects {
		o := &s.Objects[i]
		o.Aspect = strings.ToLower(strings.TrimSpace(o.Aspect))
		if o.Aspect != "look" && o.Aspect != "listen" && o.Aspect != "watch" {
			o.Aspect = "look"
		}
		reachable := false
		for j := range o.Tiers {
			t := &o.Tiers[j]
			if t.Passive < 0 {
				t.Passive = 0
			}
			if t.Passive > 20 {
				t.Passive = maxReachablePassive // недостижимый порог → притянуть
			}
			if t.Passive > 0 && t.Passive <= maxReachablePassive {
				reachable = true
			}
		}
		// Инвариант «осмотр всегда что-то даёт»: у видимого объекта хотя бы один
		// достижимый пассивный tell. Нет — делаем первый тир пассивным.
		if !reachable && !o.Hidden && len(o.Tiers) > 0 {
			o.Tiers[0].Passive = 11
		}
	}
}
