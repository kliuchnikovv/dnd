# Дознание (`dnd`)

**A console detective role-playing game in which the rules engine is deterministic and
the narrator is a language model.**

The investigation — who knew what, when, and how the facts connect — is resolved by a Go
core that never calls a model. Language models only narrate what the core has already
decided: the **Master** describes a scene, an **Actor** speaks a character's line, and a
**Guard** audits every line for leakage of the case. Because the core is deterministic,
a playthrough is reproducible from `(seed, input)`, which makes the narrative layer
debuggable instead of a black box.

## Who is this for

- **Players** who want a text detective game that needs no GPU, no game engine and no
  network — it runs offline, for free, with a deterministic core.
- **Developers** interested in trust boundaries around model output: the model writes
  prose, the core decides what is true. The repository is a worked example of that
  split — 10 architecture decision records, 15 design specs, 17 implementation plans.
- **People who want to read a project that treats a game as a system** — about 188k
  lines of Go across 35 packages, 1211 test functions, and a design document that
  states plainly what is deliberately *not* built and why.

## Quick start

No API key, no network, fully deterministic:

```sh
git clone https://github.com/kliuchnikovv/dnd.git
cd dnd
go run ./cmd/dnd --case cases/harbour/case.json
```

The game prints a briefing, describes the pier, and offers what the player may do. Input
is structured — pick an option by number — or free-form text once the model layer is
enabled.

Requires **Go 1.26 or newer**.

### With a narrator

The Master needs a model. Keys are read from the environment (or from a `.env` file in
the repository root — `.env` is gitignored):

```sh
export OPENROUTER_API_KEY=...      # or ANTHROPIC_API_KEY

go run ./cmd/dnd --case cases/harbour/case.json --nl \
  --provider openrouter --model anthropic/claude-sonnet-5 \
  --model-cheap anthropic/claude-haiku-4.5 \
  --price-in 2 --price-out 10 --price-in-cheap 1 --price-out-cheap 5 \
  --cap-day 0.30
```

`-nl` translates free-form player input into actions through a model. `-chat` is the
alternative: one call parses the phrase and answers with a line from the Master. The two
are deliberately incompatible — they are two different readings of the same sentence, and
silently preferring one would hand the player a reading they did not ask for.

A turn costs roughly two cents. The daily cap (`-cap-day`) and the per-turn call cap
(`-cap-turn`, 8 by default) bound the spend.

## Commands

| Command | What it does |
|---|---|
| `go run ./cmd/dnd` | The console game. Flags: `--case`, `--seed`, `--nl`, `--chat`, `--provider`, `--model`, `--cap-day`, `--cap-turn`, `--hunch`, `--plain`, `--journal`, `--replay`. |
| `go run ./cmd/server` | Thin HTTP/WebSocket transport over the domain. No game logic — state lives in the core. |
| `go run ./cmd/eval` | Evaluates the AI Master against a golden playthrough, scoring every turn on a 13-point rubric. `-dry` runs without a model. |
| `go run ./cmd/vignette` | Plays a single scenario in the terminal through the same turn path as the server, and records each run to `runs/*.jsonl` for offline leak analysis. |

The mobile client is a separate Expo/React Native app under [`client/`](client/) — a
thin, untrusted renderer of the server's `turn-view` (ADR-0001/0004/0006). It computes
nothing and owns no canon.

## Architecture

The whole project turns on one boundary: **the core is trusted, the world is not.**

```
player input
   │
   ▼
 intent  ──►  core  ──►  rules  ──►  resolution     ← deterministic, no models
                 │                                    (facts, clocks, accusation)
                 │  revealed facts only
                 ▼
            master / actor  ──►  guard  ──►  prose  ← models, narrow scope
```

- **Core** — the fact graph, what each party knows, corroboration, the pressure clock,
  the accusation, and the taxonomy of failure. It holds the truth and it is the only
  place the truth lives.
- **Rules** — a pluggable `RuleSystem` behind one interface (`Threshold` for the
  detective scenario, D&D 5e for the adventure scenario). The core never sees dice;
  the rules never see the fact graph.
- **Master** — narrates the scene and the outcome. It is given *revealed* facts only.
- **Actor** — speaks a character's line from a first-person view, knowing the
  character's own life, what was said in this conversation, and the case facts from a
  white list. What it does not know, it asks the Master for — it does not invent.
- **Guard** — a separate role that catches leakage: specifics of the crime and invented
  inhabitants must not reach the player. It is measured on a labelled corpus, and the
  cheap judge is known to miss roughly one leak in ten while the full judge misses
  none.

The `llm/` package is a **client-side gateway**, not model infrastructure: it selects a
provider and model per role, follows a fallback chain, enforces call and spend ceilings,
and streams the response. It talks to hosted APIs over HTTP. There is no model code, no
training, no inference server and no model weights in this repository.

## Reproducibility

A playthrough is a function of `(case, seed, input)`.

- `--seed` fixes the dice stream; `--script` replaces the player's typing with a file.
- `--journal` records the session — commands, whether they were applied, and an audit of
  untrusted input.
- `--replay` re-runs a recorded journal. `-case` and `-seed` must match the ones it was
  recorded with.

## Documentation

The design work is in the repository, not just in the commit history:

| Path | What it holds |
|---|---|
| [`docs/adr/`](docs/adr/) | 10 architecture decision records — why the world is layered, why the action log is write-ahead, what the threat model for free dialogue is. |
| [`docs/superpowers/specs/`](docs/superpowers/specs/) | 15 design specs. |
| [`docs/superpowers/plans/`](docs/superpowers/plans/) | 17 implementation plans, phase by phase. |
| [`docs/status.md`](docs/status.md) | The current state of the project: what the game does, the invariants the tests hold, and — in §4 — what is deliberately *not* done and why. |
| [`docs/eval/`](docs/eval/) | Evaluation reports, including the injection battery. |
| [`client/README.md`](client/README.md) | The mobile client. |

`docs/status.md` is the best entry point: §5 explains how to run everything, and §6
lists what comes next.

## Roadmap

Planned work is tracked as GitHub issues, and the source of truth for the plan is
[`docs/status.md`](docs/status.md) §6. The near-term items are the live comparison of
the two dialogue modes, a world worker behind the existing mutation boundary, and
runtime enforcement of the read-scope that is currently only asserted by tests.

## Testing

```sh
go test ./...
go vet ./...
```

The suite is the main defence of the invariants — the core does not leak, the rules do
not see the truth, and the client cannot compute a resolution. Roughly 1211 test
functions across 35 packages.

## License

Released under the [MIT License](LICENSE).
