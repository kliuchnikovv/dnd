package llm

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// Fake — детерминированный провайдер: тесты и офлайновые прогоны не должны
// зависеть от сети. Токены считаются от длины текста, поэтому стоимость
// воспроизводима, как и всё остальное в этом проекте.
type Fake struct {
	name   string
	strict bool

	mu    sync.Mutex
	calls []Request
	err   error
	reply func(Request) string
}

func NewFake(name string, strict bool) *Fake {
	return &Fake{name: name, strict: strict}
}

// FailWith заставляет провайдер отказывать — для проверки фолбэков.
func (f *Fake) FailWith(err error) *Fake {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
	return f
}

// ReplyWith задаёт ответ. По умолчанию ответ выводится из роли и ввода.
func (f *Fake) ReplyWith(reply func(Request) string) *Fake {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reply = reply
	return f
}

func (f *Fake) Name() string       { return f.name }
func (f *Fake) StrictOutput() bool { return f.strict }

func (f *Fake) Calls() []Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Request(nil), f.calls...)
}

func (f *Fake) Complete(_ context.Context, model string, r Request) (Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, r)
	if f.err != nil {
		return Response{}, f.err
	}
	text := fmt.Sprintf("[%s/%s] %s", f.name, r.Role, r.Input)
	if f.reply != nil {
		text = f.reply(r)
	}
	// Четыре символа на токен — грубо, но стабильно, а стабильность здесь
	// важнее точности: цена в тестах должна быть предсказуемой.
	return Response{
		Text: text,
		Usage: Usage{
			InputTokens:  tokens(r.System) + tokens(r.Input),
			OutputTokens: tokens(text),
		},
	}, nil
}

func tokens(s string) int {
	n := len(strings.TrimSpace(s)) / 4
	if n == 0 && s != "" {
		return 1
	}
	return n
}
