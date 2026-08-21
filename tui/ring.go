package tui

import (
	"strings"
	"sync"
)

// Ring — кольцевой буфер записей отладки. Шлюз пишет в io.Writer, поэтому
// панель отладки — это просто другой writer, а не другой формат дампа.
//
// Ограничение обязательно: одна сессия с -debug-llm даёт мегабайты, и держать
// их целиком ради прокрутки незачем. Вытесненное считается — молча обрезанный
// лог читается как пропавшие вызовы.
type Ring struct {
	mu      sync.Mutex
	limit   int
	items   []string
	dropped int
}

func NewRing(limit int) *Ring {
	if limit < 1 {
		limit = 1
	}
	return &Ring{limit: limit}
}

// Write принимает одну запись дампа. Шлюз пишет её одним вызовом, поэтому
// делить поток на строки не нужно.
func (r *Ring) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items = append(r.items, string(p))
	if len(r.items) > r.limit {
		r.dropped += len(r.items) - r.limit
		r.items = append([]string(nil), r.items[len(r.items)-r.limit:]...)
	}
	return len(p), nil
}

// Text — содержимое буфера от старого к новому.
func (r *Ring) Text() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return strings.Join(r.items, "")
}

// Dropped — сколько записей вытеснено.
func (r *Ring) Dropped() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.dropped
}
