import { TurnView } from '../turnview/types';
import { Intent } from '../turnview/intents';
import { TurnViewSource } from '../turnview/source';
import { Frame, Kind, Op } from './frame';

// NetSource — сетевой TurnViewSource поверх WS. Это ЯДРО (Task 3): connect, session_state,
// current, subscribe. send()/накопление прозы токенами/reconnect+auth-refresh — Tasks 4–5.

export interface WebSocketLike {
  onopen: (() => void) | null;
  onmessage: ((e: { data: string }) => void) | null;
  onclose: (() => void) | null;
  onerror: (() => void) | null;
  send(data: string): void;
  close(): void;
}

export interface NetSourceDeps {
  chatId: string;
  wsUrl: string;
  getToken(): Promise<string>;
  onAuthLost(): void;
  makeSocket?: (url: string) => WebSocketLike;
}

const PLACEHOLDER: TurnView = { version: 0, scene: { node: '', title: '' }, options: [] };

// wsUrlFrom — apiBaseUrl() (http/https) → адрес WS-эндпоинта чата. Отдельная
// чистая функция ради детерминированного юнит-теста без RN.
export function wsUrlFrom(apiBase: string): string {
  const base = apiBase.replace(/\/+$/, '');
  const ws = base.startsWith('https://')
    ? base.replace('https://', 'wss://')
    : base.startsWith('http://')
      ? base.replace('http://', 'ws://')
      : base;
  return ws + '/chat/ws';
}

export class NetSource implements TurnViewSource {
  private view: TurnView = PLACEHOLDER;
  private listeners = new Set<(v: TurnView) => void>();
  private ws: WebSocketLike | null = null;
  private prose = ''; // накопитель прозы текущего хода
  private outId = 0;
  private pending: ((v: TurnView) => void) | null = null;
  private closed = false;
  private backoff = 1000;
  private schedule: (fn: () => void, ms: number) => void = (fn, ms) => {
    setTimeout(fn, ms);
  };

  constructor(private deps: NetSourceDeps) {}

  current(): TurnView {
    return this.view;
  }

  subscribe(listener: (v: TurnView) => void): () => void {
    this.listeners.add(listener);
    return () => {
      this.listeners.delete(listener);
    };
  }

  // Сервер отклоняет подключения без токена (server/ws.go) — токен обязателен в URL.
  // getToken() бросает ТОЛЬКО когда токена реально нет (refresh не удался и токены
  // очищены — см. SessionScreen); обычный обрыв сети токен не трогает, и getToken
  // по-прежнему отдаёт валидное значение — тогда просто продолжаем connect() как обычно.
  // WS-события (onerror/onclose) не несут HTTP-статус и не могут значить «401» — это
  // сигнал утраты соединения, а не авторизации, поэтому оба ведут в один и тот же
  // backoff-реконнект (handleUnexpectedDisconnect), а не в onAuthLost().
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
    if (this.closed) return; // close() могли вызвать во время ожидания getToken()
    const url = `${this.deps.wsUrl}?token=${encodeURIComponent(token)}&chat_id=${encodeURIComponent(this.deps.chatId)}`;
    const make = this.deps.makeSocket ?? ((u: string) => new WebSocket(u) as unknown as WebSocketLike);
    const ws = make(url);
    this.ws = ws;
    let handled = false; // не планируем реконнект дважды, если onerror и onclose оба сработают для одного сокета
    const handleUnexpectedDisconnect = () => {
      if (handled) return;
      handled = true;
      if (this.ws === ws) this.ws = null;
      if (this.closed) return;
      const wait = this.backoff;
      this.backoff = Math.min(this.backoff * 2, 30000);
      this.schedule(() => {
        if (this.closed) return; // могли close() успеть между планированием и срабатыванием
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
    this.closed = true; // сначала: connect() и таймер реконнекта должны увидеть его первыми
    const ws = this.ws;
    this.ws = null;
    if (ws) {
      ws.onclose = null; // поздний close-эвент не должен планировать реконнект
      ws.onmessage = null;
      ws.onopen = null;
      ws.onerror = null;
      ws.close();
    }
  }

  send(intent: Intent): Promise<TurnView> {
    if (!this.ws) return Promise.reject(new Error('netSource: не подключено'));
    const payload = intent.kind === 'token' ? { token: intent.token } : { text: intent.text };
    this.outId += 1;
    const frame: Frame = { id: this.outId, chat_id: this.deps.chatId, channel: 'chat', kind: Kind.data, op: Op.message, payload };
    this.ws.send(JSON.stringify(frame));
    return new Promise<TurnView>((resolve) => {
      this.pending = resolve;
    });
  }

  private onFrame(f: Frame): void {
    if (f.kind === Kind.error) {
      // Серверный error-фрейм несёт только {message} (server/frame.go) — реальный 401
      // это HTTP-отказ хэндшейка, который проявляется как onerror/onclose, а не как
      // фрейм, поэтому здесь нет и не может быть auth-логики. Гасим лоадер: если
      // проза так и не пошла (сбой генерации), снимаем пустой streaming-блок.
      this.view = { ...this.view, narration: this.prose ? [{ kind: 'gm', text: this.prose, streaming: false }] : [] };
      this.emit();
      return;
    }
    switch (f.op) {
      case Op.sessionState:
        this.prose = '';
        this.view = f.payload as TurnView;
        this.emit();
        if (this.pending) {
          const r = this.pending;
          this.pending = null;
          r(this.view);
        }
        break;
      case Op.start: // генерация началась — лоадер до первой дельты
        this.prose = '';
        this.view = { ...this.view, narration: [{ kind: 'gm', text: '', streaming: true }] };
        this.emit();
        break;
      case Op.message: // проза-дельта
        if (f.kind === Kind.data && f.payload?.type === 'text') {
          this.prose += f.payload.delta ?? '';
          this.view = { ...this.view, narration: [{ kind: 'gm', text: this.prose, streaming: true }] };
          this.emit();
        }
        break;
      case Op.done:
        this.view = { ...this.view, narration: this.prose ? [{ kind: 'gm', text: this.prose, streaming: false }] : this.view.narration };
        this.emit();
        break;
      case Op.history:
        if (f.payload?.type === 'narration') {
          this.prose = f.payload.text ?? '';
          this.view = { ...this.view, narration: [{ kind: 'gm', text: this.prose, streaming: false }] };
          this.emit();
        }
        break;
      case Op.ping:
        this.ws?.send(JSON.stringify({ id: 0, chat_id: this.deps.chatId, channel: 'chat', kind: Kind.signal, op: Op.ping }));
        break;
      default:
        break;
    }
  }

  private emit(): void {
    this.listeners.forEach((l) => l(this.view));
  }
}
