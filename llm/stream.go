package llm

import (
	"context"
	"io"
	"strings"
)

// Delta — приращение потокового ответа: кусок текста, пришедший токенами.
type Delta struct {
	Text string
}

// Stream — потоковый ответ модели. Recv отдаёт дельты до io.EOF; после EOF
// Response() несёт финал (usage/cost/model/provider), проставленный шлюзом.
// Close освобождает ресурсы и безопасен к повторному вызову.
type Stream interface {
	Recv() (Delta, error)
	Response() Response
	Close() error
}

// StreamProvider — провайдер с потоковой генерацией. Опционален: шлюз проверяет
// реализацию и откатывается на Complete, если её нет. Схему потоково не отдаём
// — структурный вывод обязан прийти целиком, стримится только проза.
type StreamProvider interface {
	Stream(ctx context.Context, model string, r Request) (Stream, error)
}

// Stream исполняет запрос потоково: те же потолки и та же цепочка фолбэков, что
// у Do, но ответ приходит дельтами. Расход учитывается ОДИН раз — по завершении
// потока (usage известен только в конце). Порядок важен: потолки проверяются до
// первого вызова, поэтому отказ по бюджету бесплатен.
//
// Провайдер без потоковой генерации (или запрос со схемой) обслуживается
// фолбэком: Complete, затем один кадр — вызывающий код одинаков для обоих.
func (g *Gateway) Stream(ctx context.Context, r Request) (Stream, error) {
	if r.TurnID == "" {
		r.TurnID = TurnIDFrom(ctx)
	}
	if err := g.ledger.Admit(r); err != nil {
		return nil, err
	}

	chain := g.router.ChainFor(r.Role, r.Tier)
	if len(chain) == 0 {
		if g.router.Declared(r.Role) {
			return nil, ErrSchemaUnsafe
		}
		return nil, ErrNoProvider
	}

	var lastErr error
	for _, t := range chain {
		if sp, ok := t.Provider.(StreamProvider); ok && r.Schema == "" {
			st, err := sp.Stream(ctx, t.Model, r)
			if err != nil {
				lastErr = err
				continue
			}
			return &recordingStream{g: g, r: r, t: t, inner: st}, nil
		}
		// Фолбэк: провайдер без стрима или запрос со схемой. Учёт как в Do —
		// расход снимается сразу, кадр отдаётся один.
		resp, err := t.Provider.Complete(ctx, t.Model, r)
		if err != nil {
			lastErr = err
			continue
		}
		resp, err = g.finalize(t, resp)
		if err != nil {
			lastErr = err
			continue
		}
		g.ledger.Record(r, resp)
		g.dump(r, t, resp)
		return newStaticStream(resp), nil
	}
	if lastErr == nil {
		lastErr = ErrNoProvider
	}
	return nil, lastErr
}

// recordingStream оборачивает поток провайдера: пропускает дельты, а по EOF
// достаёт финал, проставляет стоимость и учитывает расход РОВНО ОДИН раз.
type recordingStream struct {
	g     *Gateway
	r     Request
	t     Target
	inner Stream

	recorded bool
	final    Response
}

func (rs *recordingStream) Recv() (Delta, error) {
	d, err := rs.inner.Recv()
	if err == io.EOF && !rs.recorded {
		rs.recorded = true
		resp, ferr := rs.g.finalize(rs.t, rs.inner.Response())
		if ferr != nil {
			// Расход неизвестен — вызов не в счёт (как в Do). Текст уже ушёл
			// дельтами, но учёт честнее провалить, чем списать наугад.
			rs.final = resp
			return Delta{}, ferr
		}
		rs.final = resp
		rs.g.ledger.Record(rs.r, resp)
		rs.g.dump(rs.r, rs.t, resp)
	}
	return d, err
}

func (rs *recordingStream) Response() Response { return rs.final }
func (rs *recordingStream) Close() error       { return rs.inner.Close() }

// staticStream — поток из одного кадра: весь текст разом, затем EOF. Обёртка
// для не-стримящих провайдеров, чтобы вызывающий код не ветвился.
type staticStream struct {
	resp Response
	done bool
}

func newStaticStream(resp Response) *staticStream { return &staticStream{resp: resp} }

func (s *staticStream) Recv() (Delta, error) {
	if s.done {
		return Delta{}, io.EOF
	}
	s.done = true
	return Delta{Text: s.resp.Text}, nil
}

func (s *staticStream) Response() Response { return s.resp }
func (s *staticStream) Close() error       { return nil }

// chunkStream нарезает готовый текст на дельты по словам. Не настоящий стрим —
// стенд-ин для Fake: он проверяет проводку дельт, не заменяя сеть.
type chunkStream struct {
	resp   Response
	pieces []string
	i      int
}

func newChunkStream(resp Response) *chunkStream {
	return &chunkStream{resp: resp, pieces: splitKeepingSpaces(resp.Text)}
}

func (s *chunkStream) Recv() (Delta, error) {
	if s.i >= len(s.pieces) {
		return Delta{}, io.EOF
	}
	d := Delta{Text: s.pieces[s.i]}
	s.i++
	return d, nil
}

func (s *chunkStream) Response() Response { return s.resp }
func (s *chunkStream) Close() error       { return nil }

// splitKeepingSpaces режет текст на слова с пробелами, сохраняя их: склейка
// дельт обязана давать исходную строку байт в байт.
func splitKeepingSpaces(text string) []string {
	if text == "" {
		return nil
	}
	var out []string
	var b strings.Builder
	for _, r := range text {
		b.WriteRune(r)
		if r == ' ' {
			out = append(out, b.String())
			b.Reset()
		}
	}
	if b.Len() > 0 {
		out = append(out, b.String())
	}
	return out
}
