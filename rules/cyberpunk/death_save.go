package cyberpunk

// DeathSave — Death Save по RED: 1d10 СТРОГО МЕНЬШЕ BODY.
// penalty — накопленный штраф раундов (каждый следующий раунд +1 к
// сложности, кумулятивно): фактически прибавляется к броску d10, потому
// что «сложнее» — значит труднее «попасть ниже BODY». Провал: смерть.
//
// Функция чистая: roll — уже брошенный d10 (1..10). Клиент/сервер бросает
// его через core.Dice.Roll(1,10).
func DeathSave(body, penalty, roll int) (survived bool) {
	return roll+penalty < body
}
