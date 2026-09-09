import { Block } from '../turnview/types';
import { Frame, Kind, Op } from './frame';
import { WebSocketLike, wsUrlFrom } from './netSource';
import { VignetteView, VIGNETTE_PLACEHOLDER } from '../vignette/types';

// VignetteNetSource — сетевой источник вида виньетки поверх того же WS-канала,
// что и NetSource (client/src/net/netSource.ts). НЕ подкласс и не правка
// NetSource: OpSessionState для виньетки несёт vignetteView, а не TurnView
// (см. ../vignette/types.ts), и сервер это НИКАК не тегирует на уровне Frame —
// клиент обязан УЖЕ ЗНАТЬ, что chatId ведёт в виньетку, ДО коннекта (см.
// заметку внизу файла «как узнать тип сессии» и screens/session/sessionKind.ts).
//
// Дублирует ~40 строк WS-обвязки NetSource (connect/reconnect/backoff/onFrame-
// стрим прозы) — не рефакторинг в общий класс, чтобы не трогать существующий
// файл (см. хендофф задачи: «маршрутизацию не переписывать»). Если позже
// решите завести общий транспорт — кандидаты на выделение: connect()/close()/
// backoff-таймер, Op.start/Op.message/Op.done обработка (идентична побайтово),
// toBlock() (тоже идентична). Op.sessionState — единственное место, что
// РЕАЛЬНО разное (TurnView vs VignetteView).

export interface VignetteSnapshot {
    mechanics: VignetteView;
    /** Та же лента, что NarrationFeed уже умеет рисовать без изменений —
     *  проза Мастера/реплики NPC/эхо игрока/системные строки. Бросков
     *  (kind:'resolution') виньетка не производит — судья виньетки не отдаёт
     *  роль наружу (см. хендофф §3, строка «resolution»), поэтому такого
     *  Block здесь не появится. */
    narration: Block[];
    /** true, когда баннер развязки МОЖНО показывать. mechanics.ended сам по
     *  себе становится true РАНЬШЕ, чем дочитана финальная проза хода (сервер
     *  шлёт session_state{ended:true} ДО narrateLocked — см.
     *  server/vignette_runtime.go: applyInput). Рендерить баннер по
     *  mechanics.ended напрямую — обрезать игроку последнюю реплику Мастера. */
    showEnded: boolean;
}

export interface VignetteNetSourceDeps {
    chatId: string;
    wsUrl: string;
    getToken(): Promise<string>;
    onAuthLost(): void;
    makeSocket?: (url: string) => WebSocketLike;
}

// Без реального прогона: если финальный ход даёт ПУСТУЮ прозу (revealed/guard
// свели narrateLocked к ''), сервер вообще не шлёт Op.start для этого хода
// (см. vignette_runtime.go: narrateLocked — `if strings.TrimSpace(text) == ""
// { return }`). Тогда ждать Op.done вечно нельзя — короткий таймаут раскрывает
// баннер сам, если Op.start за это время не пришёл. Если Op.start пришёл —
// таймаут отменяется, ждём настоящий Op.done. Значение НЕ тюнено под реальную
// сеть/устройство — эвристика, отметить как непроверенную.
const END_REVEAL_FALLBACK_MS = 600;

export class VignetteNetSource {
    private mechanics: VignetteView = VIGNETTE_PLACEHOLDER;
    private transcript: Block[] = [];
    private live: Block | null = null;
    private pendingEnded = false; // ended:true пришёл, финальная проза ещё не дочитана
    private endFallbackTimer: ReturnType<typeof setTimeout> | null = null;
    private listeners = new Set<(v: VignetteSnapshot) => void>();
    private ws: WebSocketLike | null = null;
    private outId = 0;
    private pending: ((v: VignetteSnapshot) => void) | null = null;
    private closed = false;
    private backoff = 1000;
    private schedule: (fn: () => void, ms: number) => void = (fn, ms) => {
        setTimeout(fn, ms);
    };

    constructor(private deps: VignetteNetSourceDeps) {}

    current(): VignetteSnapshot {
        return this.buildSnapshot();
    }

    private buildSnapshot(): VignetteSnapshot {
        const narration = [...this.transcript, ...(this.live ? [this.live] : [])];
        return { mechanics: this.mechanics, narration, showEnded: this.mechanics.ended && !this.pendingEnded };
    }

    private toBlock(e: { role: string; text: string; speaker?: string }): Block {
        if (e.role === 'npc') {
            return { kind: 'npc', text: e.text, speaker: e.speaker ? { id: '', name: e.speaker, disposition: 0 } : undefined };
        }
        return { kind: e.role === 'player' ? 'player' : 'gm', text: e.text };
    }

    subscribe(listener: (v: VignetteSnapshot) => void): () => void {
        this.listeners.add(listener);
        return () => {
            this.listeners.delete(listener);
        };
    }

    // Идентично NetSource.connect() — см. комментарий там же про auth/reconnect
    // семантику онерр/онклоуз.
    async connect(): Promise<void> {
        if (this.closed) return;
        let token: string;
        try {
            token = await this.deps.getToken();
        } catch {
            this.deps.onAuthLost();
            this.closed = true;
            return;
        }
        if (this.closed) return;
        const url = `${this.deps.wsUrl}?token=${encodeURIComponent(token)}&chat_id=${encodeURIComponent(this.deps.chatId)}`;
        const make = this.deps.makeSocket ?? ((u: string) => new WebSocket(u) as unknown as WebSocketLike);
        const ws = make(url);
        this.ws = ws;
        let handled = false;
        const handleUnexpectedDisconnect = () => {
            if (handled) return;
            handled = true;
            if (this.ws === ws) this.ws = null;
            if (this.closed) return;
            const wait = this.backoff;
            this.backoff = Math.min(this.backoff * 2, 30000);
            this.schedule(() => {
                if (this.closed) return;
                this.connect().catch(() => {});
            }, wait);
        };
        ws.onmessage = (e) => this.onFrame(JSON.parse(e.data) as Frame);
        ws.onopen = () => {
            this.backoff = 1000;
        };
        ws.onclose = handleUnexpectedDisconnect;
        ws.onerror = handleUnexpectedDisconnect;
    }

    close(): void {
        this.closed = true;
        if (this.endFallbackTimer) clearTimeout(this.endFallbackTimer);
        const ws = this.ws;
        this.ws = null;
        if (ws) {
            ws.onclose = null;
            ws.onmessage = null;
            ws.onopen = null;
            ws.onerror = null;
            ws.close();
        }
    }

    // send — виньетка не знает токенов-опций (Option.token): свободный ввод —
    // ЕДИНСТВЕННЫЙ канал (см. хендофф §2: «виньетка — чистый свободный ввод»).
    // Поэтому сигнатура — просто string, не Intent; вызывающий экран сам решает,
    // откуда взялся текст (Composer шлёт freeIntent — адаптер в
    // VignetteScreen.tsx распаковывает {kind:'free',text} → сюда).
    send(text: string): Promise<VignetteSnapshot> {
        if (!this.ws) return Promise.reject(new Error('vignetteNetSource: не подключено'));
        const trimmed = text.trim();
        if (trimmed === '') return Promise.resolve(this.buildSnapshot());
        this.transcript = [...this.transcript, { kind: 'player', text: trimmed }];
        this.emit();
        this.outId += 1;
        const frame: Frame = {
            id: this.outId,
            chat_id: this.deps.chatId,
            channel: 'chat',
            kind: Kind.data,
            op: Op.message,
            payload: { text: trimmed },
        };
        this.ws.send(JSON.stringify(frame));
        return new Promise<VignetteSnapshot>((resolve) => {
            this.pending = resolve;
        });
    }

    private onFrame(f: Frame): void {
        if (f.kind === Kind.error) {
            this.live = null;
            const msg: string = f.error?.message ?? 'что-то пошло не так';
            this.transcript = [...this.transcript, { kind: 'system', text: msg }];
            this.emit();
            return;
        }
        switch (f.op) {
            case Op.sessionState: {
                const v = f.payload as VignetteView;
                this.mechanics = v;
                if (v.ended) {
                    this.pendingEnded = true;
                    if (this.endFallbackTimer) clearTimeout(this.endFallbackTimer);
                    // См. END_REVEAL_FALLBACK_MS выше — страховка на «финальный
                    // ход без прозы».
                    this.endFallbackTimer = setTimeout(() => {
                        if (this.pendingEnded) {
                            this.pendingEnded = false;
                            this.emit();
                        }
                    }, END_REVEAL_FALLBACK_MS);
                }
                this.emit();
                if (this.pending) {
                    const r = this.pending;
                    this.pending = null;
                    r(this.buildSnapshot());
                }
                break;
            }
            case Op.transcript:
                if (f.payload?.type === 'transcript' && Array.isArray(f.payload.entries)) {
                    this.transcript = f.payload.entries.map((e: { role: string; text: string; speaker?: string }) =>
                        this.toBlock(e),
                    );
                    this.live = null;
                    this.emit();
                }
                break;
            case Op.start: {
                // Op.start реально пришёл для этого хода — таймаут-страховка не
                // нужна, ждём настоящий Op.done ниже.
                if (this.endFallbackTimer) {
                    clearTimeout(this.endFallbackTimer);
                    this.endFallbackTimer = null;
                }
                if (this.live && this.live.text.trim() !== '') {
                    this.transcript = [...this.transcript, { ...this.live, streaming: false }];
                }
                const role: string = f.payload?.role ?? 'gm';
                const speaker: string | undefined = f.payload?.speaker;
                this.live =
                    role === 'npc'
                        ? { kind: 'npc', text: '', streaming: true, speaker: speaker ? { id: '', name: speaker, disposition: 0 } : undefined }
                        : { kind: 'gm', text: '', streaming: true };
                this.emit();
                break;
            }
            case Op.message:
                if (f.kind === Kind.data && f.payload?.type === 'text') {
                    const base: Block = this.live ?? { kind: 'gm', text: '', streaming: true };
                    this.live = { ...base, text: (base.text ?? '') + (f.payload.delta ?? ''), streaming: true };
                    this.emit();
                }
                break;
            case Op.done:
                if (this.live && this.live.text.trim() !== '') {
                    this.transcript = [...this.transcript, { ...this.live, streaming: false }];
                }
                this.live = null;
                if (this.pendingEnded) {
                    this.pendingEnded = false; // финальная проза дочитана — теперь можно баннер
                }
                this.emit();
                break;
            case Op.ping:
                this.ws?.send(JSON.stringify({ id: 0, chat_id: this.deps.chatId, channel: 'chat', kind: Kind.signal, op: Op.ping }));
                break;
            default:
                break;
        }
    }

    private emit(): void {
        const v = this.buildSnapshot();
        this.listeners.forEach((l) => l(v));
    }
}

export { wsUrlFrom };

// ── Как узнать, что chatId ведёт в виньетку, ДО коннекта ──
//
// WS-протокол этого не тегирует (server/ws.go: handleWS отдаёт liveSession
// одинаково для sessionRuntime и vignetteRuntime — см. аудит-отчёт). Но
// клиент к моменту коннекта уже видел рулсет:
//   - НОВАЯ сессия: SessionSelectScreen уже фильтрует дела по
//     pickedCase.rules === pickedCharacter.ruleset (net/cases.ts:
//     CaseSummary.rules). Значение "vignette" (server: VignetteRulesKind,
//     server/vignette_manager.go:17) уже долетает клиенту — просто сегодня
//     нигде не читается для выбора ЭКРАНА, только для фильтра совместимости.
//   - RESUME сессии: net/sessionsClient.ts: SessionSummary НЕ несёт rules —
//     но SessionSelectScreen и так грузит listCases() параллельно с
//     listSessions() (Promise.all), так что join по caseId → CaseSummary.rules
//     возможен БЕЗ похода на сервер. См. screens/session/sessionKind.ts.
