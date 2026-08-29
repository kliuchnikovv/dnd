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

export class NetSource implements TurnViewSource {
  private view: TurnView = PLACEHOLDER;
  private listeners = new Set<(v: TurnView) => void>();
  private ws: WebSocketLike | null = null;
  private prose = ''; // накопитель прозы текущего хода
  private outId = 0;
  private pending: ((v: TurnView) => void) | null = null;

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
  // Refresh при 401/обрыве и onAuthLost — Tasks 4–5 (auth-aware reconnect); здесь токен
  // получается один раз перед подключением.
  async connect(): Promise<void> {
    const token = await this.deps.getToken();
    const url = `${this.deps.wsUrl}?token=${encodeURIComponent(token)}&chat_id=${encodeURIComponent(this.deps.chatId)}`;
    const make = this.deps.makeSocket ?? ((u: string) => new WebSocket(u) as unknown as WebSocketLike);
    const ws = make(url);
    this.ws = ws;
    ws.onmessage = (e) => this.onFrame(JSON.parse(e.data) as Frame);
    ws.onopen = () => {};
  }

  close(): void {
    this.ws?.close();
    this.ws = null;
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
