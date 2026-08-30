// Мок нативного Google Sign-In: тесты не грузят нативный модуль. googleSignIn.ts
// на импорте вызывает GoogleSignin.configure и импортирует isSuccessResponse —
// мок отдаёт их как no-op/предикат. Реальную ветку signIn тесты не трогают
// (используют fakeGoogleSignIn), но мок держит модуль импортируемым под jest.
jest.mock('@react-native-google-signin/google-signin', () => ({
  GoogleSignin: {
    configure: jest.fn(),
    hasPlayServices: jest.fn().mockResolvedValue(true),
    signIn: jest
      .fn()
      .mockResolvedValue({ type: 'success', data: { idToken: 'mockIdToken' } }),
  },
  isSuccessResponse: (r) => r?.type === 'success',
  statusCodes: {},
}));
