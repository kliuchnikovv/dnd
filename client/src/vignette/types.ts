// TS-зеркало server/vignette_runtime.go: vignetteView — тело OpSessionState для
// виньетка-сессии. НЕ путать с ../turnview/types.ts (TurnView) — виньетка не
// core.Game и в TurnView не проецируется (см.
// docs/handoff/2026-09-06-vignette-turnview-projection.md: вердикт «натужное»,
// рекомендация — своя тонкая вью). Этот файл — та своя вью, домен-модель.
//
// ВАЖНО о JSON: Surfaces/Revealed в Go объявлены БЕЗ `omitempty`, но это слайсы —
// nil-слайс без omitempty у encoding/json всё равно кодируется как `null`, не
// `[]`. Поэтому оба поля здесь `| null`, и рендер обязан фолбэчить на `?? []`
// (см. использование в VignetteScreen). Это не тайпчек-догадка — это поведение
// Go-кодировщика; НЕ проверено live-прогоном сервера в этой сессии.
//
// Beat/EndText в Go — `omitempty` (пустая строка опущена из JSON) → optional.
// Ended без omitempty → всегда присутствует, boolean.

/** Тело OpSessionState для сессии виньетки (server/vignette_runtime.go:47). */
export interface VignetteView {
    /** Заголовок сцены (Scene.Title). НЕ intro — Scene.Intro сервер сегодня
     *  клиенту вообще не отдаёт (ни здесь, ни отдельным кадром на старте сессии
     *  — см. vignette_manager.go: CreateVignette не зовёт narrateLocked).
     *  Это дыра контракта, не догадка клиента — см. README-заметку в конце
     *  vignetteSource.ts. */
    scene: string;
    /** Открытые (не-секретные) объекты сцены прямо сейчас. Может быть null. */
    surfaces: string[] | null;
    /** Тексты, раскрытые ЭТИМ ходом (bandwidth-reveal). Может быть null — и
     *  ПОЧТИ ВСЕГДА избыточно с прозой (Мастер и так их проговаривает), см.
     *  §3 хендоффа. Держим как вспомогательный, тихий список, не как основную
     *  подачу. */
    revealed: string[] | null;
    /** Нейтральное физическое положение игрока — постоянная строка статуса
     *  («Ты у очага, дверь на засове.»). Не мера (не число), не проза Мастера. */
    state_note: string;
    /** Событие эскалации (hold: расписание давления по ходам). Присутствует не
     *  всегда — отсутствие поля значит «в этот ход эскалации не было», не
     *  «сброшено». */
    beat?: string;
    /** Сцена завершена (win ИЛИ lose — vignetteView эту разницу не несёт,
     *  только текст; см. TODO в хендоффе о добавлении kind win/lose). */
    ended: boolean;
    /** Текст развязки. Приходит ОДНОВРЕМЕННО с ended:true в session_state —
     *  но ДО завершения прозы финального хода (see vignette_runtime.go:
     *  applyInput шлёт снимок раньше narrateLocked). Рендерить banner сразу же
     *  не стоит — игрок ещё не дочитал финальную прозу. См. useVignetteView.ts:
     *  showEnded держится до OpDone текущего прозы-стрима. */
    end_text?: string;
}

/** Плейсхолдер до первого OpSessionState (симметрично NetSource.PLACEHOLDER). */
export const VIGNETTE_PLACEHOLDER: VignetteView = {
    scene: '',
    surfaces: null,
    revealed: null,
    state_note: '',
    ended: false,
};
