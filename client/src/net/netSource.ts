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
  private prose = ''; // накопитель прозы текущего хода (используется с Task 4)

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

  // send() — Task 4. Реализуй здесь по Task 4.
  send(_intent: Intent): Promise<TurnView> {
    return Promise.resolve(this.view);
  }

  private onFrame(f: Frame): void {
    if (f.op === Op.sessionState && f.kind === Kind.data) {
      this.prose = '';
      this.view = f.payload as TurnView;
      this.emit();
    }
    // message/data, done, history, ping, error — Tasks 4–5.
  }

  private emit(): void {
    this.listeners.forEach((l) => l(this.view));
  }
}
