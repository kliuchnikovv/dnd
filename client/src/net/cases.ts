import { apiBaseUrl } from './config';

// CaseSummary — сводка о деле из витрины GET /cases (см. server/catalog.go:
// CaseSummary). rules — код системы правил дела ("threshold"/"dnd5e"), тот же
// код, что и CharacterRecord.ruleset — по нему на SessionSelectScreen
// фильтруются совместимые дела.
export type CaseSummary = {
  id: string;
  name: string;
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
