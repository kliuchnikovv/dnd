// Package llm — шлюз к языковым моделям: роутинг по ролям, лимиты расхода,
// учёт стоимости. Домен (core, rules, store, cases, dice) о нём не знает —
// граница проверяется архитектурными тестами.
//
// Шлюз существует раньше любой роли намеренно: без потолков расхода один
// цикл регенерации превращает счёт в порядок величины. Лимиты — не
// оптимизация, а условие включения моделей.
package llm

import (
	"context"
	"errors"
)

// Role — за что вызывается модель. Роутинг и тиринг идут по роли, а не по
// месту вызова: так модель меняется в одном месте.
type Role string

const (
	RoleIntentParser Role = "intent_parser"
	RoleNarrator     Role = "narrator"
	RoleActor        Role = "actor"
	RoleCanonGuard   Role = "canon_guard"
	RoleModeration   Role = "moderation"
	RoleWorldsmith   Role = "worldsmith"
)

// MutatesState сообщает, управляет ли выход роли изменением состояния.
// Для таких ролей допустимы только провайдеры с гарантией соответствия
// схеме: разобранный «почти валидный» JSON мутирует канон.
func (r Role) MutatesState() bool {
	return r == RoleIntentParser
}

type Usage struct {
	InputTokens  int
	OutputTokens int
}

type Request struct {
	Role      Role
	System    string
	Input     string
	MaxTokens int

	// Кто платит. Пустые поля означают «лимит этого уровня не применяется»,
	// кроме глобального — он применяется всегда.
	UserID  string
	PartyID string
	TurnID  string

	// Schema непуста, если выход обязан соответствовать схеме.
	Schema string
}

type Response struct {
	Text      string
	Model     string
	Provider  string
	Usage     Usage
	CostMicro int64 // микродоллары, целые
}

// Provider — один поставщик моделей. Реализации живут в этом же пакете.
type Provider interface {
	Name() string
	// StrictOutput — гарантирует ли провайдер, что выход соответствует схеме.
	// Провайдер без гарантии не допускается к ролям, мутирующим состояние.
	StrictOutput() bool
	Complete(ctx context.Context, model string, r Request) (Response, error)
}

var (
	ErrKillSwitch   = errors.New("llm: суточный потолок расхода превышен, шлюз закрыт")
	ErrUserBudget   = errors.New("llm: суточный потолок пользователя превышен")
	ErrPartyBudget  = errors.New("llm: суточный потолок парти превышен")
	ErrTurnCalls    = errors.New("llm: превышено число вызовов на ход")
	ErrNoProvider   = errors.New("llm: для роли не осталось доступных провайдеров")
	ErrSchemaUnsafe = errors.New("llm: роль мутирует состояние, а провайдер не гарантирует схему")
	ErrUnknownModel = errors.New("llm: у модели нет цены в таблице")
)

// turnKey — ключ хода в контексте. Потолок вызовов на ход защищает от цикла
// регенерации, но только если ход вообще опознан. Передавать его через все
// интерфейсы было бы шумом, поэтому он едет в контексте, а шлюз подхватывает
// его сам.
type turnKey struct{}

// WithTurnID помечает контекст идентификатором хода.
func WithTurnID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, turnKey{}, id)
}

// TurnIDFrom возвращает идентификатор хода из контекста, если он там есть.
func TurnIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(turnKey{}).(string)
	return id
}
