package main

import (
	"fmt"

	"github.com/kliuchnikovv/dnd/llm"
)

// ensurePrice убеждается, что расход выбранной модели можно посчитать. Слаги
// OpenRouter пространственные и в таблицу цен не попадают, поэтому цену для
// них задают флагами — в долларах за миллион токенов, как их публикует сам
// поставщик.
//
// Отказ наступает ДО первого вызова. Узнать про неизвестную цену из ошибки на
// пятнадцатой минуте игры — значит потерять сессию, а весь смысл шлюза в том,
// чтобы расход был известен заранее.
func ensurePrice(model string, in, out float64) error {
	if in > 0 && out > 0 {
		llm.SetPrice(model, llm.Price{
			InputMicroPerMTok:  int64(in * 1_000_000),
			OutputMicroPerMTok: int64(out * 1_000_000),
		})
		return nil
	}
	if llm.HasPrice(model) {
		return nil
	}
	return fmt.Errorf(
		"цена модели %q неизвестна: задайте -price-in и -price-out в долларах за "+
			"миллион токенов\nцены поставщика: https://openrouter.ai/models\n"+
			"например: -price-in 3 -price-out 15", model)
}
