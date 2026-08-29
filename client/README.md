# Дознание — RN-клиент (тонкий рендерер turn-view)

Мобильный клиент (React Native / Expo, iOS-first) детективной RPG «Дознание». Клиент —
**тонкий недоверенный рендерер `turn-view`** (ADR-0001/0004/0006): не считает состояние, не владеет
каноном, не решает исход. Рисует типизированный вид, шлёт интенты непрозрачными токенами, сервер
валидирует. Сервера ещё нет — клиент работает против **мок-фикстур** дела «Гавань».

## Запуск

```bash
cd client
npm install
npm run ios        # нативный dev-билд в симулятор (нужен Xcode + CocoaPods)
npm run type-check # tsc --noEmit
npm test           # jest: плумбинг, паритет контракта, guard агностичности
```

> Примечание про locale: если `pod install` падает с `Encoding::CompatibilityError (ASCII-8BIT)`,
> запускайте с `LANG=en_US.UTF-8 LC_ALL=en_US.UTF-8`.

Expo Go на симуляторе может быть старее SDK 57 («requires newer Expo Go») — тогда только нативный
билд (`npm run ios`) или обновлённый Expo Go.

## Не-переговорные принципы (держать во всём коде)

1. **Тонкий/недоверенный** — никакой игровой логики; состояние только из `turn-view`.
2. **Дескрипторы, не конкретика** (ADR-0006) — `Meter`/`Resolution` от правила, `Panel` от сценария.
   Рендерер не знает слов «grit»/«d20»/«casebook»/слотов who/how/when/why. Их называет только
   конфиг-слой (`src/mocks/haven/skin.ts`, как `cli/refview.go` в ядре). Следит guard-тест
   `src/components/turnview/__tests__/agnostic.test.ts`.
3. **Минимализм через surfacing** — рисуем только `Surface:true` (зеркало `cli/render.go`).
4. **Интент — непрозрачный токен** — тап шлёт `Option.token`/`PanelItem.token` как есть.
5. **Проза — контент** — текст в безопасном контейнере, по нему не навигируем.

## Структура

```
src/
  ds/              вендор @genie/ds (Card/Text/Chip/Button/… — API без изменений)
  theme/           вендор тем nomi + ThemeContext; nightDetective.ts (тема «Ночной детектив») + fonts
  turnview/        types.ts — TS-ЗЕРКАЛО view/turnview.go; intents.ts; source.ts (TurnViewSource/MockSource)
  mocks/haven/     фикстуры дела «Гавань» (typed TurnView) + skin.ts (окраска мер, конфиг-слой)
  mocks/app.ts     app-level view-модели (Хаб/Лента Мира/Создание) — ПОКА без серверного контракта
  components/turnview/  обобщённый рендерер: MeterRow, ResolutionCard, NarrationFeed, OptionsRail,
                        Panel, MapGraph, Participants, Composer, StreamingCursor
  components/Icon.tsx   линейные SVG-иконки
  screens/         SceneScreen(6b) DialogueScreen(10a) MapScreen(9a) DossierScreen(8a)
                   HubScreen(13b) WorldFeedScreen(15a) CharacterCreateScreen(14a) + SessionScreen (роутер)
  screens/auth/    SignInScreen, ProfileScreen — вход/профиль (см. §Auth)
  navigation/      FloatingDock(12a) + AppShell (маршрутизация табов)
  auth/            googleSignIn.ts (GoogleSignIn/useGoogleSignIn), tokenStore.ts (secure-store)
  net/             authClient.ts (/auth/google, /auth/refresh, /me), config.ts (apiBaseUrl)
  state/           store.ts (zustand: таб + источник), useTurnView (привязка источника к экрану),
                   useAuth.ts (authStore/createAuthStore), gate.ts (screenFor — гейт App.tsx)
```

Сессионный под-экран выбирается **из данных вида** (`sessionModeOf`): карта → MapScreen, всплывшая
панель → DossierScreen, игрок в разговоре → DialogueScreen, иначе SceneScreen. Клиент не
интерпретирует токены для навигации.

## Отклонения от исходного плана (осознанные)

- **SDK 57, не 54.** `create-expo-app` дал текущий стабильный SDK 57 (RN 0.86, reanimated 4).
  Донор — SDK 54; вендоренные компоненты на стандартных RN-примитивах перенеслись без правок.
- **Диалог и док построены на turn-view рендерере, а не вендорингом `ChatView`/`MainTabs`.**
  Дерево зависимостей `ChatView` (react-native-markdown-display, модель `Message` из nomi) тяжёлое и
  не ложится на мок-контракт; `DialogueScreen` собран на `NarrationFeed` + пузыри + `OptionsRail`,
  `FloatingDock` — свой по спекам `12a`. Переиспользована архитектура ДС (`@genie/ds`) и паттерны,
  но не их код 1:1.
- **`@gorhom/bottom-sheet` установлен, но командная область сессии пока — инлайн-композер.**
  Bottom-sheet вариант («меры + вход в панели») — следующий заход.

## Auth

Клиент гейтит приложение по статусу `useAuth()`/`authStore` (`src/state/gate.ts`,
`screenFor`): `loading` → сплэш (текущий ActivityIndicator), `signedOut` →
`SignInScreen`, `signedIn` → `AppShell`. Вход — Google (`expo-auth-session`),
токены — `expo-secure-store` (`src/auth/tokenStore.ts`).

### Google client IDs

1. В [Google Cloud Console](https://console.cloud.google.com/) → APIs & Services →
   Credentials создать OAuth 2.0 Client ID для **iOS**, **Android** и **Web**
   (тип «Web application» нужен даже для мобильного flow — им подписывается ID-token).
   - iOS: bundle id `com.kliuchnikovv.client` (см. `app.json` → `ios.bundleIdentifier`).
   - Android: package name + SHA-1 dev-keystore.
   - Web: используется как `webClientId` в `expo-auth-session` (audience ID-токена).
2. Значения положить в `client/.env` (не коммитить):
   ```
   EXPO_PUBLIC_GOOGLE_IOS_CLIENT_ID=...apps.googleusercontent.com
   EXPO_PUBLIC_GOOGLE_ANDROID_CLIENT_ID=...apps.googleusercontent.com
   EXPO_PUBLIC_GOOGLE_WEB_CLIENT_ID=...apps.googleusercontent.com
   EXPO_PUBLIC_API_URL=http://localhost:8080
   ```
   Код читает их через `process.env.EXPO_PUBLIC_*` (`src/auth/googleSignIn.ts`,
   `src/net/config.ts`) — доступны в рантайме Expo без доп. настройки. `app.json` →
   `expo.extra.googleAuth` содержит только плейсхолдеры-документацию (куда класть
   client ID), рантайм их не читает.
3. Redirect-схема — `app.json` → `expo.scheme` (`dnd-client`); нативные модули
   auth — `expo-auth-session`, `expo-web-browser`, `expo-secure-store`.
4. **После добавления/смены client ID нужен пересбор dev-билда**
   (`npx expo run:ios --device` / `npx expo run:android`), т.к. `expo-secure-store` и
   `expo-auth-session` — нативные модули, а не JS-конфигурация; `expo start` их не
   подхватит без нового нативного билда.

Реальный e2e-проход через Google на устройстве — ручная проверка (client IDs +
пересборка нужны), не юнит-тест.

## Вне этого захода

Транспорт/сервер (req/resp + стрим/пуш) — до серверной спеки только `MockSource`. Асс-стор/арт —
плейсхолдеры (`*Ref`-стабы). Варианты-пары экранов `9b/8b/10b/14b/15b`, монетизация StoreKit,
Sign in with Apple, пуши.
