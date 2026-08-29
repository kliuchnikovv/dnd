import { TurnView } from './types';
import { Intent } from './intents';

// TurnViewSource — граница между клиентом и «сервером хода». Сейчас за ней MockSource
// (детерминированный сценарий на фикстурах). Когда появится серверная спека — тем же
// интерфейсом встанет NetSource (req/resp + стрим прозы), а экраны не изменятся.
export interface TurnViewSource {
    /** Текущий вид. */
    current(): TurnView;
    /** Отправить интент, получить новый вид. Сервер валидирует — клиент не решает исход. */
    send(intent: Intent): Promise<TurnView>;
    /** Подписка на смену вида (в т.ч. будущий стрим прозы). Возвращает отписку. */
    subscribe(listener: (view: TurnView) => void): () => void;
}

/**
 * MockSource — источник на фикстурах. Держит именованные виды и таблицу переходов
 * «токен → id следующего вида». Неизвестный токен возвращает текущий вид без изменений
 * (сервер бы валидировал; мок просто не двигается). Свободный текст в моке никуда не ведёт —
 * возвращает текущий вид (плумбинг проверяется тестом, содержательного ответа мок не даёт).
 */
export class MockSource implements TurnViewSource {
    private views: Record<string, TurnView>;
    private order: string[];
    private currentId: string;
    private transitions: Record<string, string>;
    private listeners = new Set<(view: TurnView) => void>();

    constructor(params: {
        views: Record<string, TurnView>;
        start: string;
        /** token → id следующего вида. */
        transitions?: Record<string, string>;
    }) {
        this.views = params.views;
        this.order = Object.keys(params.views);
        this.currentId = params.start;
        this.transitions = params.transitions ?? {};
    }

    current(): TurnView {
        return this.views[this.currentId];
    }

    async send(intent: Intent): Promise<TurnView> {
        if (intent.kind === 'token') {
            const next = this.transitions[intent.token];
            if (next !== undefined && this.views[next] !== undefined) {
                this.currentId = next;
            }
        }
        // Свободный текст и неизвестный токен: мок остаётся на месте.
        const view = this.current();
        this.emit(view);
        return view;
    }

    subscribe(listener: (view: TurnView) => void): () => void {
        this.listeners.add(listener);
        return () => {
            this.listeners.delete(listener);
        };
    }

    /** Тест-хелпер: прямой переход к именованному виду. */
    goto(id: string): TurnView {
        if (this.views[id] !== undefined) {
            this.currentId = id;
            this.emit(this.views[id]);
        }
        return this.current();
    }

    private emit(view: TurnView): void {
        this.listeners.forEach((l) => l(view));
    }
}
