package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kliuchnikovv/dnd/dice"
	"github.com/kliuchnikovv/dnd/scenegen"
	"github.com/kliuchnikovv/dnd/vignette"
)

// VignetteRulesKind — значение поля "rules" дела и Character.Ruleset, по которому
// сервер маршрутизирует сессию на виньетка-трек (ADR-0009).
const VignetteRulesKind = "vignette"

// CreateVignette поднимает сессию виньетки из спека сцены: repair+validate DTO →
// vignette.FromSpec → NewState на честной кости → живой рантайм в sibling-карте.
// Битую сцену не пускаем (валидатор обязателен). Персистентность минимальна
// (in-memory), см. vignetteRuntime.
func (m *Manager) CreateVignette(spec *scenegen.SceneSpec, seed int64, userID string) (string, error) {
	scenegen.Repair(spec)
	if errs := scenegen.Validate(spec); len(errs) > 0 {
		return "", fmt.Errorf("сцена невалидна: %s", strings.Join(errs, "; "))
	}
	sc := vignette.FromSpec(spec)
	st := vignette.NewState(dice.NewSource(seed).Stream("resolve"))

	chatID := fmt.Sprintf("vig-%s", randToken())
	rt := newVignetteRuntime(chatID, userID, seed, m.store, m.narrator, sc, st)
	if m.vignetteJudge != nil {
		rt.judge = m.vignetteJudge // с ключом — LLMJudge (сам откатится на keyword)
	}

	m.mu.Lock()
	m.vignettes[chatID] = rt
	m.mu.Unlock()

	// Пре-считать вступительную прозу СИНХРОННО на общем контексте: подписчиков
	// ещё нет, broadcast был бы потерян; первый attach отдаст введение из
	// буфера. Синхронно — чтобы к моменту возврата chat_id клиенту интро уже
	// было. Для fake/offline это мгновенно (см. renderIntroLocked); в живом
	// режиме — один LLM-вызов на Create (см. §2.1 хендоффа).
	rt.precomputeIntro(context.Background())
	return chatID, nil
}

// CreateVignetteFromCase читает сцену виньетки из дела (<casesRoot>/<case>/
// scene.json — DTO scenegen.SceneSpec) и поднимает сессию. Виньетка-дело
// объявляет себя полем "rules":"vignette" в case.json (для CaseRulesKind), а сама
// сцена лежит рядом в scene.json — её модель не M1a-дело, через cases.Parse она
// не идёт.
func (m *Manager) CreateVignetteFromCase(caseName string, seed int64, userID string) (string, error) {
	path := filepath.Join(m.casesRoot, caseName, "scene.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("сцена дела %q: %w", caseName, err)
	}
	var spec scenegen.SceneSpec
	if err := json.Unmarshal(raw, &spec); err != nil {
		return "", fmt.Errorf("сцена дела %q не разобралась: %w", caseName, err)
	}
	return m.CreateVignette(&spec, seed, userID)
}

// GetVignette возвращает живую виньетка-сессию по chat_id. Реконструкции из
// журнала пока нет (in-memory-срез, TODO second-pass) — только кэш.
func (m *Manager) GetVignette(chatID string) (*vignetteRuntime, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rt, ok := m.vignettes[chatID]
	return rt, ok
}
