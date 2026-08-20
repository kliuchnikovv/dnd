package main

import (
	"bufio"
	"os"
	"strings"
)

// loadDotenv подхватывает .env из текущего каталога.
//
// Причина конкретная: `source .env` в шелле задаёт переменную ОБОЛОЧКИ, а не
// окружения, поэтому дочерний процесс её не видит — грабли, на которые
// наступают все и дважды. Файл читает сам бинарник, и вопрос закрыт.
//
// Уже заданное окружение приоритетнее файла: экспортированный ключ должен
// перебивать забытый в .env, а не наоборот.
func loadDotenv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return // файла нет — это норма, а не ошибка
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		key, value, ok := parseDotenvLine(sc.Text())
		if !ok {
			continue
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		os.Setenv(key, value)
	}
}

// parseDotenvLine разбирает строку файла. Понимает форму с export и без,
// кавычки и комментарии; всё прочее молча пропускает.
func parseDotenvLine(line string) (key, value string, ok bool) {
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
