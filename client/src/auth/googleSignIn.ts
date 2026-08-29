import { useMemo } from 'react';
import * as WebBrowser from 'expo-web-browser';
import * as Google from 'expo-auth-session/providers/google';

// GoogleSignIn — граница Google-входа. За ней либо реальный expo-auth-session,
// либо фейк в тестах. Возвращает Google ID-token; бросает при отмене/ошибке.
export interface GoogleSignIn {
  signIn(): Promise<string>;
}

export function fakeGoogleSignIn(result: string | Error): GoogleSignIn {
  return {
    async signIn() {
      if (result instanceof Error) throw result;
      return result;
    },
  };
}

WebBrowser.maybeCompleteAuthSession();

// useGoogleSignIn — реальная обёртка. Client IDs берутся из переменных
// окружения EXPO_PUBLIC_GOOGLE_*. ID-token из результата promptAsync. Тонкая:
// логика входа живёт в useAuth, здесь только получение id_token.
export function useGoogleSignIn(): GoogleSignIn {
  const [, , promptAsync] = Google.useIdTokenAuthRequest({
    iosClientId: process.env.EXPO_PUBLIC_GOOGLE_IOS_CLIENT_ID,
    androidClientId: process.env.EXPO_PUBLIC_GOOGLE_ANDROID_CLIENT_ID,
    webClientId: process.env.EXPO_PUBLIC_GOOGLE_WEB_CLIENT_ID,
  });
  return useMemo<GoogleSignIn>(
    () => ({
      async signIn() {
        const res = await promptAsync();
        if (res?.type !== 'success' || !res.params?.id_token) {
          throw new Error('Google вход отменён или без id_token');
        }
        return res.params.id_token as string;
      },
    }),
    [promptAsync],
  );
}
