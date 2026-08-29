import type { AuthStatus } from './useAuth';

// screenFor — какой экран показать по статусу авторизации. Чистая функция,
// чтобы гейт был проверяем без рендера.
export function screenFor(status: AuthStatus): 'loading' | 'signin' | 'app' {
  switch (status) {
    case 'signedIn':
      return 'app';
    case 'loading':
      return 'loading';
    default:
      return 'signin';
  }
}
