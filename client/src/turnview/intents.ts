// Интент — то, что клиент шлёт на ход. Две формы, обе непрозрачны для клиента:
//   - token: тап по аффордансу (Option.token / PanelItem.token). Клиент шлёт id как есть.
//   - free:  свободный текст игрока — равноправная альтернатива.
// Сервер разворачивает и ВСЕГДА валидирует (ADR-0001/0004). Клиент интентов не сочиняет
// и токен не интерпретирует.

export type Intent =
    | { kind: 'token'; token: string }
    | { kind: 'free'; text: string };

export const tokenIntent = (token: string): Intent => ({ kind: 'token', token });
export const freeIntent = (text: string): Intent => ({ kind: 'free', text });
