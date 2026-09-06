package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/kliuchnikovv/dnd/cli"
	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/store"
	"github.com/kliuchnikovv/dnd/view"
)

// chatInterpreter — разбор свободного ввода игрока. За ним intent.GameInterpreter
// (LLM), в тестах — фейк. Отдаёт интент (обычный ход), пробу (мир отвечает без
// хода), уточнение (встречный вопрос) — ровно то, чем ветвится interpretFreeLocked.
type chatInterpreter interface {
	InterpretChat(ctx context.Context, text string, with store.EntityID, pending string) (*core.Intent, string, core.Probe, string, bool, error)
}

// turnViewVersion — версия формы turn-view, которую отдаёт сервер. Совпадает с
// той, что проставляет view.Build; сервер лишь пересылает.

// Ruleset/Scenario серверного вида — референсные реализации из cli: меры от
// правила порога, панель цели детективного досье. Гейм-логики в них нет, это
// проекция состояния ядра; сервер берёт их как есть, чтобы вид совпадал с CLI.
var (
	serverRuleset  view.Ruleset  = cli.RefRuleset{}
	serverScenario view.Scenario = cli.Detective{}
)

// subscribe регистрирует сокет и возвращает его. Вызывается при (ре)коннекте.
func (rt *sessionRuntime) subscribe() *subscriber {
	sub := &subscriber{out: make(chan Frame, 64)}
	rt.mu.Lock()
	rt.subs[sub] = struct{}{}
	rt.mu.Unlock()
	return sub
}

// unsubscribe снимает сокет с рассылки. Идемпотентна.
func (rt *sessionRuntime) unsubscribe(sub *subscriber) {
	rt.mu.Lock()
	delete(rt.subs, sub)
	rt.mu.Unlock()
}

// broadcast рассылает кадр всем подписчикам. Отставший читатель (полный буфер)
// пропускается, а не блокирует ход: его вылечит реконнект с досстримом.
// Держит rt.mu, поэтому кадр собирается заранее.
func (rt *sessionRuntime) broadcastLocked(f Frame) {
	for sub := range rt.subs {
		select {
		case sub.out <- f:
		default:
		}
	}
}

// nextOutID выдаёт id для серверного кадра. Под rt.mu.
func (rt *sessionRuntime) nextOutIDLocked() int {
	rt.outSeq++
	return rt.outSeq
}

// snapshotView собирает session_state текущего состояния — без применения хода.
// Нужен при (ре)коннекте: подписчик сразу видит, где игра.
func (rt *sessionRuntime) snapshotView() Frame {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	tv := view.Build(rt.game, core.TurnResult{}, nil, serverRuleset, serverScenario, rt.spokenTo)
	return newFrame(rt.nextOutIDLocked(), rt.chatID, ChannelChat,
		KindData, OpSessionState, tv)
}

// applyInput разворачивает ввод игрока в интент показанного набора, применяет
// его ядром и рассылает получившийся session_state. Возвращает false и текст
// ошибки, если ввод не разворачивается в валидный вариант.
//
// Модели здесь нет: и токен, и номер ссылаются на аффорданс, у которого интент
// уже собран ядром. Свободный NL-текст — отдельный путь фазы LLM.
//
// frameID — id входящего кадра: повтор (id <= lastApplied) не применяет ход
// дважды, но всё равно пере-отдаёт текущий session_state, чтобы дубль доставки
// оставлял клиента в согласованном состоянии.
func (rt *sessionRuntime) applyInput(ctx context.Context, frameID int, in inputPayload) (bool, string) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	if frameID != 0 && frameID <= rt.lastAppliedID {
		// Дубль доставки: ход уже применён. Пере-отдаём состояние, не трогая
		// ни ядро, ни журнал.
		rt.broadcastLocked(rt.snapshotViewLocked())
		return true, ""
	}

	offered := rt.game.Affordances(rt.spokenTo)
	if aff, ok := expand(offered, in); ok {
		intent := aff.Intent
		intent.Actor = rt.game.Actor
		// Лейбл действия для ленты — тот же, что игрок видел на кнопке. Подводки
		// нет: у хода-варианта chat-reply не спрашивается.
		return rt.applyIntentLocked(ctx, frameID, intent, view.AffordanceLabel(rt.game, aff), "")
	}
	// Связный текст: разбираем интерпретатором (интент / проба / уточнение).
	return rt.interpretFreeLocked(frameID, strings.TrimSpace(in.Text))
}

// applyIntentLocked проводит один интент через журнал и ядро, рассылает
// session_state, пишет эхо действия в ленту и запускает прозу. Общий низ для
// хода-варианта и хода из разобранного текста. actionLabel — подпись действия в
// ленте; leadIn — подводка из chat-reply (для разговорного хода из свободного
// текста; "" у хода-варианта). frameID==0 означает, что дедуп уже сделан
// вызывающим. Под rt.mu.
func (rt *sessionRuntime) applyIntentLocked(ctx context.Context, frameID int, intent core.Intent, actionLabel, leadIn string) (bool, string) {
	// Журнал ДО обработки (ADR-0002): команда ложится как pending раньше, чем
	// ядро тронуло состояние. Падение между записью и применением лечит реплей
	// хвоста — команда получит applied, ход не повторится.
	turn := rt.turn + 1
	payload, err := json.Marshal(intent)
	if err != nil {
		return false, "интент не сериализуется"
	}
	dctx := store.DiceCtx{Seed: rt.seed, Turn: turn}
	entry := store.CommandLogEntry{
		SessionID:      store.SessionID(rt.chatID),
		SnapshotID:     rt.snapshot,
		CoreVersion:    core.Version,
		Intent:         payload,
		DiceCtx:        dctx,
		IdempotencyKey: idempotencyKey(store.SessionID(rt.chatID), dctx, payload),
	}
	saved, isNew, err := rt.store.AppendCommand(ctx, entry)
	if err != nil {
		return false, "журнал: " + err.Error()
	}
	if !isNew {
		// Ключ идемпотентности уже известен: ход применён в прошлой жизни
		// сессии. Пере-отдаём состояние, ядро не трогаем.
		if frameID != 0 {
			rt.lastAppliedID = frameID
		}
		rt.broadcastLocked(rt.snapshotViewLocked())
		return true, ""
	}

	res := rt.advance(intent)
	rt.turn = turn
	if err := rt.store.MarkApplied(ctx, saved.SessionID, saved.Seq); err != nil {
		// Ход применён, но отметка не легла: реплей хвоста доиграет и пометит.
		// Молчать нельзя — рассинхрон журнала с состоянием должен быть слышен.
		return false, "журнал (applied): " + err.Error()
	}
	if frameID != 0 {
		rt.lastAppliedID = frameID
	}

	// Эхо действия в ленту. Не канон (источник правды — command_log), поэтому
	// ошибку записи не эскалируем в отказ применённого хода, но делаем слышимой.
	rt.appendTranscriptLocked(TranscriptEntry{Role: RolePlayer, Text: actionLabel})

	tv := view.Build(rt.game, res, nil, serverRuleset, serverScenario, rt.spokenTo)
	rt.broadcastLocked(newFrame(rt.nextOutIDLocked(), rt.chatID, ChannelChat,
		KindData, OpSessionState, tv))

	// Механика ушла мгновенно; проза доезжает отдельной горутиной. Под rt.mu:
	// аргументы Мастеру снимаются со свежего состояния.
	rt.startProseLocked(intent, res, leadIn)
	return true, ""
}

// interpretFreeLocked разбирает связный текст интерпретатором и разводит по трём
// исходам: интент (обычный ход), проба (мир отвечает, канон не меняется) и
// уточнение (Мастер задаёт встречный вопрос). Интерпретация идёт LLM-вызовом ПОД
// rt.mu — сессия однопользовательская, а лоадер уходит клиенту раньше вызова
// (broadcast неблокирующий), поэтому «Мастер печатает» виден, пока разбираем.
// Разбор на detached-контексте: обрыв сокета его не роняет. Под rt.mu.
func (rt *sessionRuntime) interpretFreeLocked(frameID int, text string) (bool, string) {
	if rt.interp == nil || text == "" {
		return false, "ход не разворачивается в показанный вариант"
	}
	if frameID != 0 {
		rt.lastAppliedID = frameID
	}
	// Лоадер до разбора: клиент видит «Мастер печатает», пока идёт LLM.
	if rt.genCancel != nil {
		rt.genCancel()
		rt.genCancel = nil
	}
	rt.narrateGen++
	rt.narrating = true
	rt.narrateRole = RoleGM
	rt.narrateSpeaker = ""
	rt.narration = nil
	rt.broadcastLocked(rt.startFrameLocked(RoleGM, ""))

	pending := rt.pending
	rt.pending = "" // вопрос задан один раз: ответ на него уже пришёл
	// reply — подводка Мастера (безоценочная, заземлена на слова игрока): станет
	// leadIn'ом разговорного хода вместо выдумывающего обрамления.
	inp, reply, probe, clarify, idle, err := rt.interp.InterpretChat(context.Background(), text, rt.spokenTo, pending)

	switch {
	case err != nil:
		// Сбой переводчика — не отказ мира: сообщаем ошибкой-кадром, лоадер гасим.
		rt.narrating = false
		rt.broadcastLocked(errorFrame(rt.nextOutIDLocked(), rt.chatID,
			"переводчик недоступен: "+err.Error()))
		return true, ""
	case idle:
		// Ввод не действие персонажа (мета/инъекция/мусор): состояние не меняется,
		// ход не тратится. Эхо игрока + диегетическая строка «медлит», без реплики
		// Мастера (пересказывать нечего) и без броска.
		rt.narrating = false
		rt.appendTranscriptLocked(TranscriptEntry{Role: RolePlayer, Text: text})
		rt.appendTranscriptLocked(TranscriptEntry{Role: RoleGM, Text: "Ты медлишь, ничего не предпринимая."})
		rt.broadcastLocked(rt.snapshotViewLocked())
		return true, ""
	case probe.Text != "":
		// Авторский контент достижим словами: если проба назвала цель, за которой
		// что-то положено, это обычный ход (канон меняется валидируемо ядром).
		if in, ok := rt.game.MatchProbeAs(probe); ok {
			in.Actor = rt.game.Actor
			return rt.applyIntentLocked(context.Background(), 0, in, text, reply)
		}
		// Иначе чистое повествование: мир отвечает, состояние не меняется, ход
		// не тратится (журнала нет). Эхо действия + отклик пробы в ленту.
		rt.appendTranscriptLocked(TranscriptEntry{Role: RolePlayer, Text: text})
		rt.broadcastLocked(rt.snapshotViewLocked())
		rt.launchSegmentsLocked([]proseSegment{{pr: cli.ProbeProse(rt.game, probe), role: RoleGM}})
		return true, ""
	case inp == nil:
		// Неясное обращение: ход не состоялся. Подсказку интерпретатора держим
		// внутренним pending (для следующего разбора), а игроку показываем
		// диегетическую прозу — мир не понял и ждёт, пока скажет яснее.
		q := clarify
		if strings.TrimSpace(q) == "" {
			q = "Уточните, к кому вы обращаетесь и что делаете."
		}
		rt.pending = q
		rt.appendTranscriptLocked(TranscriptEntry{Role: RolePlayer, Text: text})
		rt.broadcastLocked(rt.snapshotViewLocked())
		rt.launchSegmentsLocked([]proseSegment{{pr: cli.ClarifyProse(rt.game), role: RoleGM}})
		return true, ""
	default:
		intent := *inp
		intent.Actor = rt.game.Actor
		return rt.applyIntentLocked(context.Background(), 0, intent, text, reply)
	}
}

// appendTranscriptLocked пишет запись ленты best-effort: лента не канон, ошибку
// не эскалируем в отказ хода, но делаем слышимой. Под rt.mu.
func (rt *sessionRuntime) appendTranscriptLocked(e TranscriptEntry) {
	if err := rt.store.AppendTranscript(context.Background(), store.SessionID(rt.chatID), e); err != nil {
		log.Printf("лента: запись не легла: %v", err)
	}
}

// advance применяет интент к ядру и доводит состояние до полного: резолв броска
// ядром плюс шов перемещения (узел меняет слой над ядром). Живой ход и реплей
// зовут ОДНО это место — состояние симметрично по построению. Ни журнала, ни
// рассылки здесь нет: их добавляет вызывающий. Требует взятого rt.mu.
func (rt *sessionRuntime) advance(intent core.Intent) core.TurnResult {
	if intent.Actor == "" {
		intent.Actor = rt.game.Actor
	}
	res := rt.game.Apply(intent)
	// Перемещение — известный шов: ядро резолвит бросок, узел меняет слой над
	// ним (в CLI это afterAction). Без него успешный move_zone крутит кость и
	// оставляет игрока на месте.
	if intent.Verb == "move_zone" && res.Res != nil && res.Res.Class >= core.OutcomePartial {
		rt.game.MoveTo(intent.Args.Node)
	}
	// Адресат хода продолжает разговор: следующий набор аффордансов строится
	// от него. Пусто — вышли из разговора.
	if intent.Args.Target != "" {
		rt.spokenTo = intent.Args.Target
	}
	return res
}

// idempotencyKey — ключ повторной доставки команды: сессия, кость и интент. Та
// же формула, что в cli.Journal (ADR-0002): одинаковый ход в одном контексте
// кости не применяется дважды.
func idempotencyKey(s store.SessionID, d store.DiceCtx, payload []byte) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s|%d|%d|", s, d.Seed, d.Turn)
	h.Write(payload)
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// snapshotViewLocked — как snapshotView, но под уже взятым rt.mu.
func (rt *sessionRuntime) snapshotViewLocked() Frame {
	tv := view.Build(rt.game, core.TurnResult{}, nil, serverRuleset, serverScenario, rt.spokenTo)
	return newFrame(rt.nextOutIDLocked(), rt.chatID, ChannelChat,
		KindData, OpSessionState, tv)
}

// expand сопоставляет ввод с показанным набором: токен — по стабильному
// OptionToken, текст — по номеру варианта (1..N). Один путь на оба входа, как
// в CLI: токен и номер разворачиваются в тот же интент.
func expand(offered []core.Affordance, in inputPayload) (core.Affordance, bool) {
	if in.Token != "" {
		for _, a := range offered {
			if view.OptionToken(a) == in.Token {
				return a, true
			}
		}
		return core.Affordance{}, false
	}
	if n, ok := parseChoice(in.Text); ok {
		if n >= 1 && n <= len(offered) {
			return offered[n-1], true
		}
	}
	return core.Affordance{}, false
}

// parseChoice читает номер варианта из текста. Свободный NL сюда не попадает:
// он уходит в LLM-парсер (фаза 4). Пустая строка и не-число — не выбор.
func parseChoice(text string) (int, bool) {
	n := 0
	seen := false
	for _, r := range text {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			if seen {
				break
			}
			continue
		}
		if r < '0' || r > '9' {
			return 0, false
		}
		n = n*10 + int(r-'0')
		seen = true
	}
	if !seen {
		return 0, false
	}
	return n, true
}
