// Package server — тонкий транспорт над доменом. Он не считает состояние
// игры: только грузит дело, строит core.Game, разворачивает интент и отдаёт
// turn-view. Пакет живёт НАД чистыми слоями (core/rules/store/dice/cases) и
// вправе знать о сети и моделях — граница держится тем, что чистые слои о нём
// не знают (см. e2e/architecture_test.go).
package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/kliuchnikovv/dnd/cases"
	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/dice"
	"github.com/kliuchnikovv/dnd/intent"
	"github.com/kliuchnikovv/dnd/llm"
	"github.com/kliuchnikovv/dnd/master"
	"github.com/kliuchnikovv/dnd/rules/threshold"
	"github.com/kliuchnikovv/dnd/scenegen"
	"github.com/kliuchnikovv/dnd/store"
	"github.com/kliuchnikovv/dnd/view"
	"github.com/kliuchnikovv/dnd/vignette"
)

// SceneGenerator — продуктовый путь «тема → SceneSpec» (core-способность
// scenegen, а не приложение к виньеточному треку). Manager принимает его
// колбэком, чтобы не тянуть llm.Gateway в свой конструктор — проводку из
// шлюза даёт cmd/server (см. WithSceneGenerator). Виньетка — первый (и пока
// единственный) потребитель через CreateVignetteFromTheme.
type SceneGenerator func(ctx context.Context, theme, mode string) (*scenegen.SceneSpec, error)

// WithVignetteJudge ставит судью виньетка-трека (обычно LLMJudge на шлюзе). Без
// него виньетка играет офлайн-судьёй KeywordJudge.
func (m *Manager) WithVignetteJudge(j vignette.Judge) *Manager {
	m.vignetteJudge = j
	return m
}

// sessionRuntime — живая игра одной сессии в памяти вместе с её сокет-
// подписчиками. Генерация хода не привязана к живому сокету: механику применяет
// ядро, результат рассылается всем подписчикам chat_id, поэтому второе
// устройство и реконнект видят один и тот же ход (буфер прозы придёт в фазе 4).
type sessionRuntime struct {
	chatID   string
	caseID   store.CaseID
	seed     int64
	snapshot string
	store    Store
	// userID — владелец сессии (Subject из access-токена на момент Create).
	// Пусто у легаси-сессий, созданных до auth-гейта.
	userID string

	// narrator — Мастер для стрима прозы. nil означает механику без прозы
	// (сервер без сконфигурированного LLM): ходы применяются, session_state
	// уходит, прозы просто нет.
	narrator *master.Master
	// interp — разбор свободного ввода в интент/пробу/уточнение. nil означает,
	// что связный текст не разбирается: принимаются только варианты и номера.
	interp chatInterpreter

	mu   sync.Mutex
	game *core.Game
	// viewRuleset — презентация правила сессии для view.Build. Резолвится один
	// раз при создании/реконструкции по имени правила дела через реестр
	// view.RegisterRuleset (ADR-0010). Хранится готовым инстансом, чтобы turn-
	// путь не звал реестр на каждом кадре и не логировал фолбэк по 3 раза за ход.
	viewRuleset view.Ruleset
	// turn — номер применённого хода. Входит в DiceCtx и ключ идемпотентности
	// журнала; растёт на каждый состоявшийся ход.
	turn int
	// Стрим прозы текущего хода: буфер дельт ТЕКУЩЕГО сегмента под досстрим при
	// возобновлении, признак «идёт генерация» и отмена (op:stop и суперсессия
	// новым ходом). Ход может состоять из нескольких сегментов (обрамление gm +
	// реплика npc): завершённые сегменты уже в ленте, в буфере — только текущий.
	// narrateRole/narrateSpeaker — роль и говорящий текущего сегмента: их несёт
	// OpStart, в т.ч. при реконнекте в середине сегмента.
	narration      []string
	narrating      bool
	narrateGen     int
	narrateRole    string
	narrateSpeaker string
	genCancel      context.CancelFunc
	// spokenTo — последний адресат: разговор продолжается с тем же NPC, пока
	// игрок не обратится к другому. Определяет набор аффордансов хода.
	spokenTo store.EntityID
	// pending — заданный Мастером встречный вопрос (уточнение): следующий
	// свободный ввод разбирается как ответ на него. Пусто — вопроса нет.
	pending string
	// lastAppliedID — наибольший applied Frame.ID: клиент шлёт монотонный
	// счётчик, переживающий реконнект, поэтому повтор (id <= lastApplied) —
	// это дубль доставки, а не новый ход. Идемпотентность хода.
	lastAppliedID int
	// subs — открытые сокеты этой сессии. Рассылка session_state идёт всем.
	subs map[*subscriber]struct{}
	// outSeq — счётчик id серверных кадров (session_state, error): id входящих
	// принадлежит клиенту, исходящие нумеруются сервером.
	outSeq int
}

// subscriber — один подключённый сокет. out буферизован: медленный клиент не
// держит горутину хода; переполнение означает отставшего читателя (фаза 5
// решит его реконнектом с досстримом).
type subscriber struct {
	out chan Frame
}

// Manager — кэш живых сессий над долговечным журналом. Сессии, которых нет в
// кэше (после рестарта), восстанавливаются реплеем журнала при первом
// обращении. Доступ сериализован мьютексом: сокеты приходят из разных горутин.
type Manager struct {
	casesRoot string
	store     Store
	// narrator — общий Мастер для стрима прозы, один на все сессии (шлюз и
	// ledger потокобезопасны). nil означает сервер без прозы.
	narrator *master.Master
	// parser — общий интерпретатор свободного ввода. nil — только варианты/номера.
	parser *intent.Parser

	mu       sync.Mutex
	sessions map[string]*sessionRuntime
	// vignettes — живые сессии виньетка-трека (ADR-0009), рядом с sessions.
	// Отдельная карта, потому что у виньетки свой движок (не core.Game).
	vignettes map[string]*vignetteRuntime
	// vignetteJudge — судья виньетки. nil → рантайм берёт офлайн KeywordJudge;
	// с ключом cmd/server ставит LLMJudge (тот сам откатывается на keyword при сбое).
	vignetteJudge vignette.Judge
	// sceneGen — генератор сцен из темы (scenegen поверх llm.Gateway); nil в
	// офлайн-сервере без ключа. Продуктовый путь «тема → сцена → виньетка»
	// доступен только если он задан (см. CreateVignetteFromTheme и
	// server.handleCreateVignetteFromTheme).
	sceneGen SceneGenerator
}

// WithSceneGenerator подключает scenegen-путь: сервер сможет принять тему,
// сгенерировать сцену и поднять виньетка-сессию из неё (POST
// /vignette/from-theme). Без него POST /vignette/from-theme отвечает 501:
// генератор — core-способность и доступен только когда шлюз с ключом собран.
func (m *Manager) WithSceneGenerator(g SceneGenerator) *Manager {
	m.sceneGen = g
	return m
}

// SceneGeneratorAvailable — есть ли подключённый scenegen. Транспорту нужно,
// чтобы отдать 501 без похода в Manager (см. handleCreateVignetteFromTheme).
func (m *Manager) SceneGeneratorAvailable() bool { return m.sceneGen != nil }

// CreateVignetteFromTheme — продуктовый core-путь: тема → scenegen.Generate
// (через колбэк) → Repair+Validate → CreateVignette. Ошибка отделяет «нет
// генератора» (клиент увидит 501) от «генерация сорвалась/сцена невалидна»
// (400). Именно этот путь — то, что просит §1.4 хендоффа
// 2026-09-09-vignette-into-mvp: генератор доступен из продукта, а не только из
// CLI cmd/vignette.
func (m *Manager) CreateVignetteFromTheme(ctx context.Context, theme, mode string, seed int64, userID string) (string, error) {
	if m.sceneGen == nil {
		return "", fmt.Errorf("scenegen не подключён")
	}
	spec, err := m.sceneGen(ctx, theme, mode)
	if err != nil {
		return "", fmt.Errorf("генерация сцены: %w", err)
	}
	return m.CreateVignette(spec, seed, userID)
}

// WithNarrator включает стрим прозы: сессии получат Мастера. Без него сервер
// отдаёт только механику (session_state), как в фазах до LLM.
func (m *Manager) WithNarrator(ms *master.Master) *Manager {
	m.narrator = ms
	return m
}

// WithInterpreter включает разбор свободного ввода: связный текст игрока пойдёт
// через парсер в интент/пробу/уточнение. Без него принимаются только варианты и
// номера (как в фазах до чат-режима).
func (m *Manager) WithInterpreter(p *intent.Parser) *Manager {
	m.parser = p
	return m
}

// newInterp строит интерпретатор свободного ввода для одной сессии: разбору
// нужна её игра. nil, если парсер не задан (сервер без свободного ввода). Явный
// nil, а не типизированный: interpretFreeLocked проверяет interp == nil.
func (m *Manager) newInterp(game *core.Game) chatInterpreter {
	if m.parser == nil {
		return nil
	}
	// MaxTokens обязателен: без него провайдер резервирует дефолтный максимум
	// модели (напр. 64k), и OpenRouter отбивает запрос по кредитам (402). Разбор
	// + короткая реплика в 1024 укладываются с запасом.
	return &intent.GameInterpreter{Parser: m.parser, Game: game, Req: llm.Request{MaxTokens: 1024}}
}

// NewManager строит менеджер с журналом в памяти — MVP без БД и основа тестов.
// Дела ищутся в casesRoot: дело "harbour" — это casesRoot/harbour/case.json.
func NewManager(casesRoot string) *Manager {
	return NewManagerWithStore(casesRoot, NewMemStore())
}

// NewManagerWithStore — менеджер над заданным журналом (Postgres в проде).
func NewManagerWithStore(casesRoot string, st Store) *Manager {
	return &Manager{
		casesRoot: casesRoot,
		store:     st,
		sessions:  make(map[string]*sessionRuntime),
		vignettes: make(map[string]*vignetteRuntime),
	}
}

// Create грузит дело, строит игру на данном seed, записывает паспорт сессии в
// журнал и регистрирует её, возвращая chat_id. Ошибка — про дело (не нашли/не
// разобрали) или про журнал. Система правил — threshold, персонаж —
// минимальный дефолт (низкоуровневый путь без character_id: тесты и
// реконструкция после рестарта; см. CreateWithRuleset для сессий с явным
// character_id и настоящим листом).
func (m *Manager) Create(caseName string, seed int64, userID string) (string, error) {
	return m.create(caseName, seed, userID, core.RulesetThreshold, threshold.New(), nil)
}

// CreateWithRuleset — как Create, но система правил и персонаж задаются
// вызывающим явно. Сервер сверяет ruleset персонажа с полем "rules" дела ДО
// вызова (см. server.handleCreateSession) и передаёт сюда уже проверенную
// систему правил вместе с самим персонажем — иначе его лист (Sheet) неоткуда
// взять внутри buildGame, и игра стартовала бы на дефолтной заглушке вместо
// настоящего листа (найдено ревью Task 6: character.Ruleset сверялся, но сам
// character в игру не попадал).
func (m *Manager) CreateWithRuleset(caseName string, seed int64, userID string, kind core.RulesetKind, rules core.RuleSystem, character *store.Character) (string, error) {
	return m.create(caseName, seed, userID, kind, rules, character)
}

func (m *Manager) create(caseName string, seed int64, userID string, kind core.RulesetKind, rules core.RuleSystem, character *store.Character) (string, error) {
	game, caseID, snapshot, err := m.buildGame(caseName, seed, rules, character)
	if err != nil {
		return "", err
	}
	chatID := fmt.Sprintf("%s-%s", snapshot, randToken())

	if err := m.store.SaveSession(context.Background(), SessionRecord{
		ChatID:      chatID,
		CaseID:      caseName,
		Seed:        seed,
		Snapshot:    snapshot,
		CoreVersion: core.Version,
		UserID:      userID,
	}); err != nil {
		return "", fmt.Errorf("журнал: %w", err)
	}

	rt := &sessionRuntime{
		chatID:      chatID,
		caseID:      caseID,
		seed:        seed,
		snapshot:    snapshot,
		store:       m.store,
		userID:      userID,
		narrator:    m.narrator,
		interp:      m.newInterp(game),
		game:        game,
		viewRuleset: resolveViewRuleset(kind),
		subs:        make(map[*subscriber]struct{}),
	}
	m.mu.Lock()
	m.sessions[chatID] = rt
	m.mu.Unlock()

	// Опенинг: вводная проза места стартует сразу при рождении сессии.
	// Подписчиков ещё нет — дельты копятся в буфере, первый attach() отдаёт их
	// (в полёте — дельтами, завершённые — кадром history).
	rt.mu.Lock()
	rt.startOpeningProseLocked()
	rt.mu.Unlock()
	return chatID, nil
}

// Get возвращает живую сессию по chat_id, восстанавливая её из журнала, если в
// кэше нет (первое обращение после рестарта). Второй результат — нашлась ли
// сессия вообще.
func (m *Manager) Get(chatID string) (*sessionRuntime, bool) {
	m.mu.Lock()
	rt, ok := m.sessions[chatID]
	m.mu.Unlock()
	if ok {
		return rt, true
	}
	return m.reconstruct(chatID)
}

// reconstruct поднимает сессию из журнала: паспорт → свежая игра из (case,
// seed) → реплей команд тем же путём, что живой ход. Незавершённый хвост
// (pending после падения между «записал» и «применил») доигрывается и
// помечается applied. Двойного применения нет: реплей журнал не дописывает.
func (m *Manager) reconstruct(chatID string) (*sessionRuntime, bool) {
	ctx := context.Background()
	recs, err := m.store.Sessions(ctx)
	if err != nil {
		return nil, false
	}
	var rec SessionRecord
	found := false
	for _, r := range recs {
		if r.ChatID == chatID {
			rec, found = r, true
			break
		}
	}
	if !found {
		return nil, false
	}

	// Реконструкция не знает, какой ruleset и какого персонажа выбрала сессия
	// при создании: SessionRecord их не хранит (персистентность character_id —
	// вне scope этого плана, придёт с login-flow). Пока — threshold и
	// дефолтный персонаж, как и было до character-owned ruleset; не регрессия,
	// а сохранение старого поведения для восстановленных после рестарта сессий.
	game, caseID, snapshot, err := m.buildGame(rec.CaseID, rec.Seed, threshold.New(), nil)
	if err != nil {
		return nil, false
	}
	rt := &sessionRuntime{
		chatID:      chatID,
		caseID:      caseID,
		seed:        rec.Seed,
		snapshot:    snapshot,
		store:       m.store,
		userID:      rec.UserID,
		narrator:    m.narrator,
		interp:      m.newInterp(game),
		game:        game,
		viewRuleset: resolveViewRuleset(core.RulesetThreshold),
		subs:        make(map[*subscriber]struct{}),
	}

	cmds, err := m.store.Commands(ctx, store.SessionID(chatID))
	if err != nil {
		return nil, false
	}
	for _, e := range cmds {
		if e.CoreVersion != core.Version {
			// Правка ядра меняет исход прошлых бросков: реплей обязан сказать
			// это, а не выдать другую сессию за ту же. Молча продолжить нельзя.
			return nil, false
		}
		var intent core.Intent
		if err := json.Unmarshal(e.Intent, &intent); err != nil {
			return nil, false
		}
		rt.advance(intent)
		rt.turn = e.DiceCtx.Turn
		if e.Status == store.CommandPending {
			_ = m.store.MarkApplied(ctx, e.SessionID, e.Seq)
		}
	}

	m.mu.Lock()
	// Гонка: пока восстанавливали, другой мог уже поднять эту сессию. Первый
	// победитель остаётся, чтобы не расщепить состояние на два объекта.
	if existing, ok := m.sessions[chatID]; ok {
		m.mu.Unlock()
		return existing, true
	}
	m.sessions[chatID] = rt
	m.mu.Unlock()
	return rt, true
}

// WarmCache поднимает все сессии журнала в кэш. cmd/server зовёт её на старте:
// клиент, цепляющийся сразу после рестарта, не ждёт ленивого восстановления.
func (m *Manager) WarmCache(ctx context.Context) error {
	recs, err := m.store.Sessions(ctx)
	if err != nil {
		return err
	}
	for _, r := range recs {
		m.Get(r.ChatID)
	}
	return nil
}

// buildGame собирает игру из (дело, seed, система правил, персонаж) — общий
// путь для Create/CreateWithRuleset и реплея. Возвращает игру, её caseID и
// снепшот начального состояния.
//
// character — настоящий лист персонажа-актёра (character_id пришёл через
// server.handleCreateSession и был провалидирован там на совместимость
// ruleset ДО этого вызова). nil — низкоуровневый путь без character_id
// (Manager.Create, реконструкция после рестарта): заводим минимального
// персонажа по умолчанию, как раньше делал загрузчик кейсов, чтобы игра
// оставалась играбельной без обязательной завязки на HTTP-слой.
//
// Персонаж кладётся в базу ПОД КЛЮЧОМ cfg.Actor, а не под своим собственным
// ID: cfg.Actor — это ключ, под которым дело ждёт актёра в остальных таблицах
// (в т.ч. db.Entities для сценария adventure — движок хода и Defeat() читают
// db.Entities[EntityID(g.Actor)], и подмена g.Actor на произвольный
// character.ID без парного Entity увела бы actor-сущность в нулевой HP,
// то есть в мгновенное поражение). ID персонажа в собственном CharacterStore
// остаётся его настоящим паспортом; здесь берётся только его Sheet/Ruleset/
// Grit/Harm.
func (m *Manager) buildGame(caseName string, seed int64, rules core.RuleSystem, character *store.Character) (*core.Game, store.CaseID, string, error) {
	if caseName == "" {
		return nil, "", "", fmt.Errorf("дело не указано")
	}
	path := filepath.Join(m.casesRoot, caseName, "case.json")
	// Сырьё читаем сами: снепшот хешируется от СОДЕРЖИМОГО дела, а не от пути
	// — правка дела обязана делать журнал непереигрываемым (как в cmd/dnd).
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, "", "", fmt.Errorf("дело %q: %w", caseName, err)
	}
	cfg, err := cases.Parse(raw)
	if err != nil {
		return nil, "", "", fmt.Errorf("дело %q: %w", caseName, err)
	}
	cfg.Rules = rules
	cfg.Dice = dice.NewSource(seed).Stream("resolve")
	if character != nil {
		actor := *character
		actor.ID = cfg.Actor
		cfg.DB.SaveCharacter(&actor)
	} else if _, ok := cfg.DB.CharacterByID(cfg.Actor); !ok {
		cfg.DB.SaveCharacter(&store.Character{ID: cfg.Actor, Grit: 3})
	}
	return core.NewGame(*cfg), cfg.CaseID, snapshotID(cfg.CaseID, raw, seed), nil
}

// CaseRulesKind читает поле "rules" из case.json дела caseName, не разбирая
// дело целиком — сверке ruleset персонажа (см. server.handleCreateSession)
// нужно только имя системы правил. Правило по умолчанию то же, что у
// LoadCatalog: пустое поле — "threshold" (обратная совместимость с делами без
// явного rules).
func (m *Manager) CaseRulesKind(caseName string) (string, error) {
	path := filepath.Join(m.casesRoot, caseName, "case.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("дело %q: %w", caseName, err)
	}
	var rr rawRules
	if err := json.Unmarshal(raw, &rr); err != nil {
		return "", fmt.Errorf("дело %q: %w", caseName, err)
	}
	if rr.Rules == "" {
		return defaultRulesKind, nil
	}
	return rr.Rules, nil
}

// randToken — короткий случайный суффикс chat_id. Разводит две сессии одного
// дела на одном seed и переживает рестарт: счётчик процесса дал бы после
// перезапуска тот же id и столкновение в журнале.
func randToken() string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// snapshotID — паспорт начального состояния: дело плюс seed. Хеш от
// содержимого, а не от пути, чтобы правка дела ломала переигрываемость явно.
func snapshotID(caseID store.CaseID, content []byte, seed int64) string {
	h := sha256.New()
	h.Write(content)
	fmt.Fprintf(h, "|%d", seed)
	return fmt.Sprintf("%s@%s", caseID, hex.EncodeToString(h.Sum(nil))[:8])
}
