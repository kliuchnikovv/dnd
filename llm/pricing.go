package llm

// Цены — микродоллары за миллион токенов. Целые числа намеренно: деньги во
// float складывать нельзя, а все цены кратны центу.
type Price struct {
	InputMicroPerMTok  int64
	OutputMicroPerMTok int64
}

var prices = map[string]Price{
	"claude-opus-5":    {5_000_000, 25_000_000},
	"claude-sonnet-5":  {3_000_000, 15_000_000},
	"claude-haiku-4-5": {1_000_000, 5_000_000},
	"claude-fable-5":   {10_000_000, 50_000_000},
}

// CostMicro считает стоимость в микродолларах. Возвращает ошибку для модели
// без цены: неизвестная цена означает неизвестный расход, а весь смысл
// шлюза в том, чтобы расход был известен.
func CostMicro(model string, u Usage) (int64, error) {
	p, ok := prices[model]
	if !ok {
		return 0, ErrUnknownModel
	}
	in := int64(u.InputTokens) * p.InputMicroPerMTok / 1_000_000
	out := int64(u.OutputTokens) * p.OutputMicroPerMTok / 1_000_000
	return in + out, nil
}

// HasPrice сообщает, известна ли цена модели. Спрашивать надо ДО вызова:
// узнать о неизвестной модели из ошибки посреди сессии — значит потерять
// сессию.
func HasPrice(model string) bool {
	_, ok := prices[model]
	return ok
}

// SetPrice регистрирует цену модели. Для моделей вне таблицы по умолчанию.
func SetPrice(model string, p Price) { prices[model] = p }
