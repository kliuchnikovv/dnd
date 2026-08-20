package llm

// Target — куда идёт роль: провайдер плюс модель.
type Target struct {
	Provider Provider
	Model    string
}

// Router держит цепочку целей на роль: первая основная, остальные фолбэки.
type Router struct {
	chains map[Role][]Target
}

func NewRouter() *Router { return &Router{chains: map[Role][]Target{}} }

// Route задаёт цепочку для роли. Порядок значим.
func (r *Router) Route(role Role, targets ...Target) *Router {
	r.chains[role] = targets
	return r
}

// Chain возвращает цепочку, отбрасывая цели, недопустимые для этой роли:
// роль, мутирующая состояние, не пойдёт к провайдеру без гарантии схемы.
func (r *Router) Chain(role Role) []Target {
	var out []Target
	for _, t := range r.chains[role] {
		if role.MutatesState() && !t.Provider.StrictOutput() {
			continue
		}
		out = append(out, t)
	}
	return out
}

// Declared сообщает, объявлена ли роль вообще — отдельно от того, прошла ли
// она фильтр пригодности. Нужно, чтобы различать «не настроено» и
// «настроено небезопасно».
func (r *Router) Declared(role Role) bool { return len(r.chains[role]) > 0 }
