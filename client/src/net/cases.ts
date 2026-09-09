import { apiBaseUrl } from './config';

// CaseSummary — сводка о деле из витрины GET /cases (см. server/catalog.go:
// CaseSummary). rules — код системы правил дела ("threshold"/"dnd5e"), тот же
// код, что и CharacterRecord.ruleset — по нему на SessionSelectScreen
// фильтруются совместимые дела.
// kind — вид сессии, который поднимет это дело (см. server/catalog.go:
// CaseSummary.Kind): "adventure" — приключение (turn-view), "vignette" —
// виньеточный трек (VignetteScreen). Клиент решает экран по kind, а не по
// rules: rules — «правила», kind — «какой рантайм это». Для дел витрины,
// созданных до появления поля (обратная совместимость сервером), kind может
// прийти пустой строкой — трактуется как "adventure".
export type CaseSummary = {
  id: string;
  name: string;
  kind: string;
  rules: string;
  scenario: string;
  blurb: string;
};

export class CasesError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.name = 'CasesError';
    this.status = status;
  }
}

// listCases — GET /cases. Требует Bearer-токен, как и остальные ручки сервера
// за auth-гейтом (см. server/server.go: WithCatalog).
export async function listCases(token: string): Promise<CaseSummary[]> {
  const resp = await fetch(apiBaseUrl() + '/cases', {
    method: 'GET',
    headers: { Authorization: 'Bearer ' + token },
  });
  if (!resp.ok) throw new CasesError(resp.status, 'cases: GET /cases -> ' + resp.status);
  return (await resp.json()) as CaseSummary[];
}
