package llm

import (
	"context"
	"fmt"
	"io"
)

// Gateway — единственная точка, через которую вызываются модели.
type Gateway struct {
	router *Router
	ledger *Ledger
	debug  io.Writer
}

// WithDebug включает дамп обмена с моделью. Нужен ровно тогда, когда ответ
// модели расходится с ожиданиями, а угадывать причину дороже, чем посмотреть.
func (g *Gateway) WithDebug(w io.Writer) *Gateway {
	g.debug = w
	return g
}

func NewGateway(r *Router, l *Ledger) *Gateway {
	return &Gateway{router: r, ledger: l}
}

// Do исполняет запрос: проверяет потолки, идёт по цепочке фолбэков, учитывает
// расход. Порядок важен — потолки проверяются ДО первого вызова, поэтому
// отказ по бюджету бесплатен.
func (g *Gateway) Do(ctx context.Context, r Request) (Response, error) {
	// Ход опознаётся из контекста, если вызывающий не назвал его явно: иначе
	// потолок вызовов на ход молча не применяется ни к чему.
	if r.TurnID == "" {
		r.TurnID = TurnIDFrom(ctx)
	}
	if err := g.ledger.Admit(r); err != nil {
		return Response{}, err
	}

	chain := g.router.ChainFor(r.Role, r.Tier)
	if len(chain) == 0 {
		if g.router.Declared(r.Role) {
			// Роль настроена, но все цели отфильтрованы как непригодные.
			return Response{}, ErrSchemaUnsafe
		}
		return Response{}, ErrNoProvider
	}

	var lastErr error
	for _, t := range chain {
		resp, err := t.Provider.Complete(ctx, t.Model, r)
		if err != nil {
			lastErr = err
			continue
		}
		// Провайдер вправе сообщить фактическую стоимость сам — маршрутизатор
		// знает, на кого ушёл запрос, а статическая таблица про это не знает.
		// Молча тратить по-прежнему нельзя: нет ни цены от провайдера, ни
		// строки в таблице — значит расход неизвестен, и вызов не в счёт.
		if resp.CostMicro <= 0 {
			cost, err := CostMicro(t.Model, resp.Usage)
			if err != nil {
				lastErr = err
				continue
			}
			resp.CostMicro = cost
		}
		resp.Model = t.Model
		resp.Provider = t.Provider.Name()
		g.ledger.Record(r, resp)
		g.dump(r, t, resp)
		return resp, nil
	}
	if lastErr == nil {
		lastErr = ErrNoProvider
	}
	return Response{}, lastErr
}

func (g *Gateway) Stats() Stats { return g.ledger.Stats() }

// dump печатает обмен целиком, включая схему: расхождение чаще всего в ней.
//
// Собирается в одну строку и пишется ОДНИМ вызовом Fprint — не ради красоты,
// а потому что читатель дампа (tui.Ring) считает записи, а не строки: три
// отдельных Write на один обмен — это три записи кольца вместо одной, и
// счётчик вытесненного и потолок буфера начинают лгать. Текст не меняется ни
// на байт — меняется только число вызовов Write.
func (g *Gateway) dump(r Request, t Target, resp Response) {
	if g.debug == nil {
		return
	}
	text := fmt.Sprintf("\n--- llm %s -> %s/%s ---\n", r.Role, t.Provider.Name(), t.Model)
	if r.Schema != "" {
		text += fmt.Sprintf("схема: %s\n", r.Schema)
	}
	text += fmt.Sprintf("ввод:\n%s\nответ:\n%s\nтокены: %d/%d, стоимость: %d мкд\n",
		r.Input, resp.Text, resp.Usage.InputTokens, resp.Usage.OutputTokens, resp.CostMicro)
	fmt.Fprint(g.debug, text)
}
