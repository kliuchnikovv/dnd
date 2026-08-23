package llm

// Реестр полномочий ролей. ADR-0001: LLM предлагают, ядро располагает — и у
// каждой роли свои полномочия. Раньше это жило одной булевой функцией на
// список ролей и устной договорённостью о том, кто что читает; растущее число
// ролей превращало договорённость в дыру — новая роль тихо получала лишнее
// чтение или лишнее право.
//
// Реестр — единственная правда о полномочиях. RequiresStrictOutput выводится
// из него, роутер фильтрует по нему, тесты его исполняют.

// ReadScope — домен чтения. Набор закрыт намеренно: чего нет в scope роли, то
// не попадает в её промпт.
//
// Здесь НЕТ и не должно появиться скоупа правды дела и чужих гейтнутых
// фактов. Тип cases.truth уже несериализуем — отсутствие константы делает то
// же правилом для всех ролей, а не свойством одного типа.
type ReadScope string

const (
	// ReadScenePublic — что видно вокруг: место, присутствующие, обстановка.
	ReadScenePublic ReadScope = "scene_public"
	// ReadPartyKnowledge — факты дела, которые парти уже знает.
	ReadPartyKnowledge ReadScope = "party_knowledge"
	// ReadCarriedItems — что у парти на руках.
	ReadCarriedItems ReadScope = "carried_items"
	// ReadOwnDossier — досье персонажа: голос, желания, нити.
	ReadOwnDossier ReadScope = "own_dossier"
	// ReadMasterGrants — то, что Мастер выдал персонажу в ответ на needs.
	ReadMasterGrants ReadScope = "master_grants"
	// ReadWorldCanon — уже решённое о мире.
	ReadWorldCanon ReadScope = "world_canon"
	// ReadAuthoredFlavour — авторский текст дела: рамка, которой держится речь.
	ReadAuthoredFlavour ReadScope = "authored_flavour"
	// ReadMechanicalOutcome — исход хода от ядра: ключ факта, цена.
	ReadMechanicalOutcome ReadScope = "mechanical_outcome"
	// ReadCandidateLine — реплика, поданная на проверку.
	ReadCandidateLine ReadScope = "candidate_line"
	// ReadPlayerInput — фраза игрока.
	ReadPlayerInput ReadScope = "player_input"
)

// ProposalKind — что роль вправе ПРЕДЛОЖИТЬ ядру. Не применить: применяет
// ядро, провалидировав. «Ничего не предлагает» — это пустой Proposes, а не
// отдельная константа: два способа сказать одно разошлись бы.
type ProposalKind string

const (
	// ProposeCommand — действие игрока, разобранное из его фразы.
	ProposeCommand ProposalKind = "command"
	// ProposeCanonAmbient — расширение мира деталью по запросу.
	ProposeCanonAmbient ProposalKind = "canon_ambient"
	// ProposeWorldMutation — изменение состояния мира.
	ProposeWorldMutation ProposalKind = "world_mutation"
)

// Capability — полномочия одной роли: что читает и что предлагает.
//
// Поля прямой мутации здесь нет, и это конструктивно, а не по недосмотру:
// ADR-0001 не оставляет LLM права менять состояние, и права, которое негде
// записать, нельзя выдать по ошибке.
type Capability struct {
	Reads    []ReadScope
	Proposes []ProposalKind
}

// Capabilities — полномочия по ролям.
var Capabilities = map[Role]Capability{
	RoleIntentParser: {
		Reads:    []ReadScope{ReadPlayerInput, ReadScenePublic, ReadPartyKnowledge, ReadCarriedItems},
		Proposes: []ProposalKind{ProposeCommand},
	},
	// RoleChatMaster разбирает фразу и отвечает голосом Мастера одним
	// вызовом: к scope парсера добавляется авторская рамка, которой эта речь
	// держится. Предлагает он то же, что парсер, — команду.
	RoleChatMaster: {
		Reads: []ReadScope{
			ReadPlayerInput, ReadScenePublic, ReadPartyKnowledge,
			ReadCarriedItems, ReadAuthoredFlavour,
		},
		Proposes: []ProposalKind{ProposeCommand},
	},
	RoleNarrator: {
		Reads: []ReadScope{
			ReadAuthoredFlavour, ReadScenePublic, ReadWorldCanon, ReadMechanicalOutcome,
		},
		Proposes: []ProposalKind{ProposeCanonAmbient},
	},
	// Актёр не предлагает ничего: его выход — речь и needs. Чего он не знает,
	// он спрашивает у Мастера, а не сочиняет.
	RoleActor: {
		Reads: []ReadScope{
			ReadPlayerInput, ReadPartyKnowledge, ReadOwnDossier,
			ReadMasterGrants, ReadScenePublic,
		},
	},
	// Судьи возвращают вердикт-совет, а не предложение: их выход не доходит
	// до состояния, и плохой разбор стоит бледной реплики, а не канона.
	RoleCanonGuard: {
		Reads: []ReadScope{
			ReadCandidateLine, ReadPartyKnowledge, ReadOwnDossier, ReadScenePublic,
		},
	},
	RoleModeration: {
		Reads: []ReadScope{ReadPlayerInput, ReadCandidateLine},
	},
	// RoleWorldsmith — куратор окружения из ADR-0001: решает не-каноничный
	// слой мира и предлагает его ядру.
	RoleWorldsmith: {
		Reads:    []ReadScope{ReadWorldCanon, ReadScenePublic},
		Proposes: []ProposalKind{ProposeCanonAmbient, ProposeWorldMutation},
	},
}
