-- Лента ходов: долговечная история для клиента (проза Мастера/NPC и эхо действий
-- игрока). В отличие от буфера прозы в памяти — переживает реконнект и рестарт.
-- ord (bigserial) задаёт глобальный порядок вставки; на сессию читаем по нему.
CREATE TABLE IF NOT EXISTS transcript (
    ord        BIGSERIAL PRIMARY KEY,
    session_id TEXT NOT NULL,
    role       TEXT NOT NULL,
    body       TEXT NOT NULL,
    speaker    TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS transcript_session_ord ON transcript (session_id, ord);
