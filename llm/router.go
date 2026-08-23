package llm

// Target — куда идёт роль: провайдер плюс модель.
type Target struct {
	Provider Provider
	Model    string
}

// Router держит цепочку целей на роль: первая основная, остальные фолбэки.
type Router struct {
	chains map[Role][]Target
	cheap  map[Role][]Target
}

func NewRouter() *Router {
	return &Router{chains: map[Role][]Target{}, cheap: map[Role][]Target{}}
}

// Route задаёт цепочку для роли. Порядок значим.
func (r *Router) Route(role Role, targets ...Target) *Router {
	r.chains[role] = targets
	return r
}

// RouteCheap задаёт цепочку для дешёвого тира этой роли. Необязательна: без
// неё дешёвый запрос идёт основной цепочкой, а не падает.
func (r *Router) RouteCheap(role Role, targets ...Target) *Router {
	r.cheap[role] = targets
	return r
}

// ChainFor выбирает цепочку по роли и уровню действия.
func (r *Router) ChainFor(role Role, tier Tier) []Target {
	if tier == TierCheap && len(r.cheap[role]) > 0 {
		return r.filter(role, r.cheap[role])
	}
	return r.Chain(role)
}

// Chain возвращает цепочку, отбрасывая цели, недопустимые для этой роли:
// роль, предлагающая ядру что-либо, не пойдёт к провайдеру без гарантии
// схемы. Пригодность решает реестр капабилити — здесь её только исполняют.
func (r *Router) Chain(role Role) []Target { return r.filter(role, r.chains[role]) }

func (r *Router) filter(role Role, in []Target) []Target {
	var out []Target
	for _, t := range in {
		if RequiresStrictOutput(role) && !t.Provider.StrictOutput() {
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
