package llm

import "context"

// Gateway — единственная точка, через которую вызываются модели.
type Gateway struct {
	router *Router
	ledger *Ledger
}

func NewGateway(r *Router, l *Ledger) *Gateway {
	return &Gateway{router: r, ledger: l}
}

// Do исполняет запрос: проверяет потолки, идёт по цепочке фолбэков, учитывает
// расход. Порядок важен — потолки проверяются ДО первого вызова, поэтому
// отказ по бюджету бесплатен.
func (g *Gateway) Do(ctx context.Context, r Request) (Response, error) {
	if err := g.ledger.Admit(r); err != nil {
		return Response{}, err
	}

	chain := g.router.Chain(r.Role)
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
		return resp, nil
	}
	if lastErr == nil {
		lastErr = ErrNoProvider
	}
	return Response{}, lastErr
}

func (g *Gateway) Stats() Stats { return g.ledger.Stats() }
