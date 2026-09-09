import { CaseSummary } from '../../net/cases';

// sessionKind — чистая vm-логика (симметрично sessionSelect.ts в этой же папке):
// по какому экрану вести сессию, ДО коннекта к WS. Сервер тегирует дело полем
// CaseSummary.kind (см. server/catalog.go: CaseKindAdventure/CaseKindVignette);
// клиент решает по нему, а не по rules (rules — «правила», kind — «какой
// рантайм»). Для дел без kind (обратная совместимость на старом сервере) есть
// фолбэк на rules === "vignette" — исторический дискриминант.
//
// VIGNETTE_KIND — зеркало server.CaseKindVignette (server/catalog.go).
// VIGNETTE_RULES_KIND — зеркало server.VignetteRulesKind (тот же файл).
// Расхождение строк с сервером ловится TestLoadCatalogReadsRealCases + этим
// же файлом на клиенте: локальный юнит-тест sessionKind.test.ts берёт
// именно эти константы.
export const VIGNETTE_KIND = 'vignette';
export const VIGNETTE_RULES_KIND = 'vignette';

export type SessionKind = 'turn' | 'vignette';

// sessionKindOf решает вид сессии по CaseSummary. Приоритет — kind (сервер
// >= 2026-09-09), фолбэк — rules (совместимость со старым сервером, у
// которого kind в ответе ещё пуст).
export function sessionKindOf(c: Pick<CaseSummary, 'kind' | 'rules'> | undefined): SessionKind {
    if (!c) return 'turn';
    if (c.kind === VIGNETTE_KIND) return 'vignette';
    if (!c.kind && c.rules === VIGNETTE_RULES_KIND) return 'vignette';
    return 'turn';
}

/**
 * kindForResumedSession — для RESUME (список «Твои дела»): SessionSummary
 * (net/sessionsClient.ts) не несёт kind/rules, но SessionSelectScreen и так
 * грузит listCases() параллельно с listSessions() (Promise.all) — join по
 * caseId не требует похода на сервер.
 *
 * Если caseId не нашёлся в каталоге (дело сняли с витрины, но сессия жива) —
 * фолбэк 'turn': это НЕ доказанная безопасная сторона, просто она не хуже
 * сегодняшнего поведения (весь resume-путь безусловно ведёт в
 * SessionScreen/AppShell). Непроверено: нет теста и живого прогона этого
 * пограничного случая.
 */
export function kindForResumedSession(caseId: string, cases: CaseSummary[]): SessionKind {
    const found = cases.find((c) => c.id === caseId);
    return sessionKindOf(found);
}
