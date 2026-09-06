package vignette

import "strings"

// Страж-редактор (leak-editor, ADR-0008): держит правду сцены и вычитает её
// утечку из черновика Мастера — умеет ТОЛЬКО вычитать (never add). Асимметрия:
// Мастер слеп к тайне и свободен, страж тайну видит, но лишь режет. Анти-лик и
// так держится конструкцией (Мастеру не дают правду) — страж это бэкстоп против
// дословного эха защищённого факта (напр. инъекция-payload в черновике).
//
// На `ended` раскрытая развязка приходит в allowed и НЕ режется (карваут финала).

type GuardResult struct {
	Clean   string
	Flagged []string // какие защищённые факты вырезаны (для лога/эвала)
	Changed bool
}

// Guard проверяет черновик против защищённых фактов (protected), положения
// (state — противоречить нельзя) и разрешённого (allowed — раскрывается намеренно).
type Guard interface {
	Check(draft string, protected, state, allowed []string) GuardResult
}

// NoGuard — пропуск как есть (офлайн без ключа / когда страж не нужен). Анти-лик
// от этого не страдает: он конструкцией, а не стражем.
type NoGuard struct{}

func (NoGuard) Check(draft string, _, _, _ []string) GuardResult {
	return GuardResult{Clean: draft}
}

// KeywordGuard — детерминированный офлайн-редактор: вырезает из черновика
// дословные вхождения защищённых фактов, кроме тех, что в allowed (раскрытая
// развязка). Настоящий LLM-страж (ловит и перифраз) — второй заход; но
// дословное эхо payload'а этот барьер снимает без ключа.
type KeywordGuard struct{}

func (KeywordGuard) Check(draft string, protected, _, allowed []string) GuardResult {
	clean := draft
	var flagged []string
	for _, p := range protected {
		p = strings.TrimSpace(p)
		if p == "" || allowedContains(allowed, p) {
			continue // разрешённое (напр. развязка) не режем
		}
		if strings.Contains(clean, p) {
			clean = strings.ReplaceAll(clean, p, "…")
			flagged = append(flagged, p)
		}
	}
	clean = strings.TrimSpace(clean)
	return GuardResult{Clean: clean, Flagged: flagged, Changed: clean != strings.TrimSpace(draft)}
}

// allowedContains — защищённый факт разрешён, если он целиком содержится в любой
// из разрешённых строк (развязка может дословно включать «правду», и это ок).
func allowedContains(allowed []string, fact string) bool {
	for _, a := range allowed {
		if strings.Contains(a, fact) {
			return true
		}
	}
	return false
}
