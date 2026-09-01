// Обобщённый рендерер turn-view. Жанро-нейтрален: рисует дескрипторы, не конкретику.
// Никаких литералов правила/сценария («grit»/«d20»/«casebook»/«who/how/when/why») — guard
// агностичности (src/components/turnview/__tests__/agnostic.test.ts) следит за этим.
export { NarrationFeed } from './NarrationFeed';
export { MeterRow } from './MeterRow';
export type { MeterTone } from './MeterRow';
export { ResolutionCard } from './ResolutionCard';
export { OptionsRail } from './OptionsRail';
export type { OptionsLayout } from './OptionsRail';
export { Panel } from './Panel';
export { AdventureSectionView, ADVENTURE_SECTION_KINDS } from './AdventurePanel';
export { MapGraph } from './MapGraph';
export { Participants, Avatar } from './Participants';
export { Composer } from './Composer';
export { StreamingCursor } from './StreamingCursor';
export { tierTone, dispositionTone } from './tone';
