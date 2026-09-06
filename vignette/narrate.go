package vignette

import (
	"context"
	"strings"
)

// Narrator рисует прозу хода. КЛЮЧЕВОЕ: реализация получает ТОЛЬКО revealed +
// нейтральные поверхности/положение — ни правды сцены (Scene.Truth), ни закрытых
// тиров (анти-лик конструкцией, ADR-0008). Абстракция, а не *master.Master:
// пакет vignette не тянет master — сервер даёт адаптер, CLI тоже.
type Narrator interface {
	Narrate(ctx context.Context, ambient string, surfaces, revealed []string, frame string) (string, error)
}

// Narrate — ОБЩИЙ шаг прозы хода для сервера и CLI (один код, не копия — иначе то,
// что «чувствуем» в CLI, разойдётся с сервером). Собирает outcome из того, что
// ядро ОТКРЫЛО (revealed + нейтральные положение/беат/развязка), зовёт нарратора
// с ТОЛЬКО revealed, откатывается на склейку outcome (нарратор==nil — офлайн без
// ключа, либо пустой ответ), и пропускает через стража с карваутом финала:
// на ended раскрытая развязка (EndText) идёт в allowed и НЕ режется.
//
// Возвращает готовую прозу (уже почищенную стражем) и ошибку нарратора, если она
// была — вызывающий решает, как её показать (сервер — errorFrame, CLI — stderr).
func Narrate(ctx context.Context, sc *Scene, st *State, res Result, n Narrator, g Guard) (string, error) {
	// outcome — только то, что ядро ОТКРЫЛО: revealed-строки, нейтральное
	// положение, беат-событие и (на конце) раскрытая развязка. Ни Scene.Truth,
	// ни закрытого тира здесь нет.
	outcome := append([]string{}, res.Revealed...)
	if res.StateNote != "" {
		outcome = append(outcome, res.StateNote)
	}
	if res.Beat != "" {
		outcome = append(outcome, res.Beat)
	}
	if res.Ended && res.EndText != "" {
		outcome = append(outcome, res.EndText)
	}

	var text string
	if n != nil {
		frame := firstNonEmptyStr(res.Beat, res.EndText, res.StateNote, "Опиши, чем кончился ход.")
		out, err := n.Narrate(ctx, sc.Ambient, sc.Surfaces(), res.Revealed, frame)
		if err != nil {
			return "", err
		}
		text = out
	}
	if strings.TrimSpace(text) == "" {
		text = strings.Join(outcome, " ")
	}

	// Страж-редактор (ADR-0008): бэкстоп против дословного эха защищённого факта.
	// Держит правду + закрытые тиры; на ended раскрытая развязка идёт в allowed
	// (карваут финала) и не режется.
	if g != nil {
		var allowed []string
		if res.Ended && res.EndText != "" {
			allowed = []string{res.EndText}
		}
		text = g.Check(text, ProtectedFacts(sc, st), StateFacts(sc, st), allowed).Clean
	}
	return strings.TrimSpace(text), nil
}

func firstNonEmptyStr(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
