// Package dotenv — минимальный загрузчик .env-файла.
//
// Зачем он, когда есть godotenv: в проекте нужен РОВНО один формат файла и
// ноль зависимостей сверх стандартной библиотеки. Плюс важное поведение,
// которого нет у большинства загрузчиков по умолчанию:
//
//   - УЖЕ ЗАДАННОЕ окружение приоритетнее файла. Экспортированный ключ должен
//     перебивать забытый в .env, а не наоборот. Иначе нельзя переопределить
//     модель на CI одним `MODEL=... go run ./cmd/eval`.
//
//   - Файл читает сам бинарник, а не шелл. `source .env` задаёт переменную
//     ОБОЛОЧКИ, а не окружения, поэтому дочерний процесс её не видит —
//     грабли, на которые наступают все и дважды.
//
// Формат поддерживается минимальный, но покрывает всё, что нужно:
// `KEY=value`, `export KEY=value`, одинарные и двойные кавычки, комментарии
// с `#`, пустые строки. Всё прочее молча пропускается.
package dotenv

import (
	"bufio"
	"os"
	"strings"
)

// Load подхватывает .env из указанного пути. Отсутствие файла — НЕ ошибка:
// инструмент без ключа должен работать в офлайн-режиме, а не падать.
//
// Уже заданные переменные окружения не перезаписываются (см. пакет-комментарий).
func Load(path string) {
	f, err := os.Open(path)
	if err != nil {
		return // файла нет — это норма, а не ошибка
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		key, value, ok := ParseLine(sc.Text())
		if !ok {
			continue
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		_ = os.Setenv(key, value)
	}
}

// ParseLine разбирает строку файла. Понимает форму с export и без, кавычки и
// комментарии; всё прочее возвращает как «не строка настройки».
func ParseLine(line string) (key, value string, ok bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}
	line = strings.TrimPrefix(line, "export ")
	key, value, found := strings.Cut(line, "=")
	if !found {
		return "", "", false
	}
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') ||
			(value[0] == '\'' && value[len(value)-1] == '\'') {
			value = value[1 : len(value)-1]
		}
	}
	if key == "" {
		return "", "", false
	}
	return key, value, true
}
