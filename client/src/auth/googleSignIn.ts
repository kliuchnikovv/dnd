import { useMemo } from 'react';
import {
  GoogleSignin,
  isSuccessResponse,
} from '@react-native-google-signin/google-signin';

// GoogleSignIn — граница Google-входа. За ней либо реальный нативный Google Sign-In
// (@react-native-google-signin), либо фейк в тестах. Возвращает Google ID-token;
// бросает при отмене/ошибке.
//
// Почему нативная библиотека, а не expo-auth-session: iOS-клиент Google работает
// только по authorization-code flow и id_token напрямую не отдаёт (promptAsync
// возвращал code, а не id_token). Нативный SDK отдаёт id_token сразу. Аудитория
// id_token — webClientId, поэтому на бэке GOOGLE_CLIENT_ID должен включать web
// client id.
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

// Конфигурируем один раз при загрузке модуля: configure синхронна и обязана быть
// вызвана до signIn.
GoogleSignin.configure({
  iosClientId: process.env.EXPO_PUBLIC_GOOGLE_IOS_CLIENT_ID,
  webClientId: process.env.EXPO_PUBLIC_GOOGLE_WEB_CLIENT_ID,
});

// useGoogleSignIn — реальная обёртка. Тонкая: логика входа живёт в useAuth, здесь
// только получение id_token из нативного модального окна Google.
export function useGoogleSignIn(): GoogleSignIn {
  return useMemo<GoogleSignIn>(
    () => ({
      async signIn() {
        await GoogleSignin.hasPlayServices();
        const res = await GoogleSignin.signIn();
        if (!isSuccessResponse(res) || !res.data?.idToken) {
          const type = (res as { type?: string })?.type ?? 'нет-ответа';
          throw new Error(`Google: type=${type} без id_token`);
        }
        return res.data.idToken;
      },
    }),
    [],
  );
}
