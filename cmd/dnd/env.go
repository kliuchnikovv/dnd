package main

import "github.com/kliuchnikovv/dnd/dotenv"

// loadDotenv подхватывает .env из указанного пути.
//
// Реализация живёт в общем пакете dotenv (см. dotenv/dotenv.go): формат
// файла и правило приоритета (уже заданное окружение перебивает файл)
// должны быть ОДНИМИ для всех бинарников проекта, а не копироваться в
// каждый cmd. Этот файл оставлен как тонкая обёртка — её зовёт main.go и
// её же проверяет env_test.go.
//
// Причина конкретная: `source .env` в шелле задаёт переменную ОБОЛОЧКИ, а не
// окружения, поэтому дочерний процесс её не видит — грабли, на которые
// наступают все и дважды. Файл читает сам бинарник, и вопрос закрыт.
func loadDotenv(path string) { dotenv.Load(path) }

// parseDotenvLine разбирает строку файла. Понимает форму с export и без,
// кавычки и комментарии; всё прочее молча пропускает.
func parseDotenvLine(line string) (key, value string, ok bool) { return dotenv.ParseLine(line) }
