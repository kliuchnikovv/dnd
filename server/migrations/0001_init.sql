-- Схема журнала сервера (ADR-0002). Форма повторяет store: команды append-only
-- с ключом (session_id, seq), интент — jsonb. Состояние игры не хранится: оно
-- выводится реплеем команд из (case, seed).

CREATE TABLE IF NOT EXISTS sessions (
    chat_id      TEXT PRIMARY KEY,
    case_id      TEXT        NOT NULL,
    seed         BIGINT      NOT NULL,
    snapshot     TEXT        NOT NULL,
    core_version TEXT        NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS command_log (
    session_id      TEXT   NOT NULL,
    seq             INT    NOT NULL,
    snapshot_id     TEXT   NOT NULL,
    core_version    TEXT   NOT NULL,
    intent          JSONB  NOT NULL,
    dice_seed       BIGINT NOT NULL,
    dice_turn       INT    NOT NULL,
    idempotency_key TEXT   NOT NULL,
    status          TEXT   NOT NULL,
    PRIMARY KEY (session_id, seq)
);

-- Ключ идемпотентности уникален в пределах сессии: повторная доставка команды
-- не создаёт вторую строку, и реплей хвоста не применяет ход дважды.
CREATE UNIQUE INDEX IF NOT EXISTS command_log_idem
    ON command_log (session_id, idempotency_key);

CREATE TABLE IF NOT EXISTS audit_log (
    session_id   TEXT NOT NULL,
    seq          INT  NOT NULL,
    raw_input    TEXT NOT NULL,
    llm_role     TEXT NOT NULL,
    llm_proposal TEXT NOT NULL,
    core_verdict TEXT NOT NULL
);

-- narration — буфер прозы хода под досстрим при возобновлении. Пустой в MVP
-- (стрим прозы отложен); таблица заведена заранее: пустая стоит ноль, миграция
-- потом стоит дорого.
CREATE TABLE IF NOT EXISTS narration (
    session_id TEXT NOT NULL,
    turn       INT  NOT NULL,
    ix         INT  NOT NULL,
    delta      TEXT NOT NULL,
    PRIMARY KEY (session_id, turn, ix)
);
