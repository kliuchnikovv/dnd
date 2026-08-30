// listKey — React-ключ для элементов turn-view. Сервер не гарантирует, что id
// в списках непустые и уникальные (живьём пришёл пустой option.id → два пустых
// ключа → React «two children with the same key» и потеря/дублирование узлов).
// Индекс-префикс делает ключ уникальным всегда; id оставляем для читаемости в
// devtools. Списки хода заменяются целиком каждый ход, поэтому привязки к id
// для reconciliation тут не требуется.
export function listKey(id: string | undefined | null, index: number): string {
    return `${index}:${id ?? ''}`;
}
