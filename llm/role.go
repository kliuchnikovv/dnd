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
	// RoleChatMaster — один вызов, который и разбирает фразу игрока, и
	// отвечает ему репликой Мастера. Роль своя, а не RoleIntentParser,
	// потому что роутинг и тиринг идут по роли: разбор со речью стоит
	// иначе, чем разбор молча, и выбирать модель для них надо отдельно.
	RoleChatMaster Role = "chat_master"
	// RoleOptions — слова для набора вариантов. Роль своя, потому что роутинг
	// и тиринг идут по роли: назвать четыре строки словами игрока стоит иначе,
	// чем описать сцену, и выбирать модель для этого надо отдельно.
	RoleOptions Role = "options"
)

// AllRoles — все объявленные роли в стабильном порядке. Нужен, чтобы
// проводка проверялась исчерпывающе: роль, объявленную и вызываемую, но не
// зароученную, шлюз отдаёт как ErrNoProvider, а откат надстройки молчит — и
// это уже дважды выглядело как плохая модель вместо ненастроенного маршрута.
func AllRoles() []Role {
	return []Role{
		RoleIntentParser,
		RoleChatMaster,
		RoleNarrator,
		RoleOptions,
		RoleActor,
		RoleCanonGuard,
		RoleModeration,
		RoleWorldsmith,
	}
}

// RequiresStrictOutput сообщает, допустимы ли для роли только провайдеры с
// гарантией соответствия схеме. Ответ ВЫВОДИТСЯ из реестра капабилити, а не
// задаётся списком: список ролей отставал бы от полномочий.
//
// Правило: роль что-то предлагает ядру — значит, её выход дойдёт до
// состояния, и разобранный «почти валидный» JSON мутирует канон. Совет
// (guard, moderation) и просто речь (актёр) строгого провайдера не требуют:
// плохой разбор стоит там бледной реплики, а не порчи состояния.
func RequiresStrictOutput(r Role) bool {
	return len(Capabilities[r].Proposes) > 0
}

// MutatesState — прежнее имя того же вопроса.
//
// Deprecated: зовите RequiresStrictOutput. Оставлено обёрткой, потому что
// «мутирует» — уже неправда: роль ничего не мутирует, она предлагает, а
// применяет ядро (ADR-0001).
func (r Role) MutatesState() bool { return RequiresStrictOutput(r) }

type Usage struct {
	InputTokens  int
	OutputTokens int
}

// Tier — уровень действия, а не роли. Ход, не меняющий мир, не должен стоить
// как ход, который его меняет: приветствие и прощание идут дешёвой моделью с
// коротким выводом. Уровень известен ДО вызова — иначе выбирать было бы
// поздно.
type Tier string

const (
	TierMain  Tier = ""
	TierCheap Tier = "cheap"
)

type Request struct {
	Role      Role
	Tier      Tier
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
	ErrSchemaUnsafe = errors.New("llm: роль предлагает ядру изменение, а провайдер не гарантирует схему")
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
