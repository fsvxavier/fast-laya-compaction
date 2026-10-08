# fast-laya-compaction — Plano de implementação

- **Data:** 2026-10-08
- **Base:** [design](2026-10-08-fast-laya-compaction-design.md)
- **Método:** TDD — cada tarefa começa por um teste que falha. Uma tarefa = um commit pequeno.
- **Convenções:** Go 1.25+, `gofmt`/`golangci-lint`, identificadores e comentários de código em inglês, documentação em português.

## Visão das fases

| Fase | Entrega | Depende de | Portão de saída |
|---|---|---|---|
| 0 | Spike de viabilidade do Laya nativo | — | `Predict` real com 2 perguntas `noul` no WSL2 |
| 1 | Núcleo `compaction` (estágios 1–7) com `fakelaya` | — (paralela à 0) | 20 casos portados passando; golden 1–7 iguais aos do original |
| 2 | Estágio 8 `windowed`, `unfit`, retries | 1 | Casos novos e fuzz das invariantes passando |
| 3 | Adaptador Laya + calibração | 0, 2 | `--report` no corpus com < 1% `ErrInputTooLong` |
| 4 | Daemon e CLI | 3 | Testes `httptest`; `make doctor` verde |
| 5 | Plugin TS | 4 | Testes do hook; compactação real numa sessão do Claude Code |
| 6 | Provisionamento, Docker, README final | 0, 4 | `make provision` do zero numa máquina limpa; `docker run` + `/readyz` 200 |

---

## Fase 0 — Spike de viabilidade (portão)

**Objetivo:** provar que o caminho da [ADR 0004](../../adrs/0004-inferencia-in-process-com-laya-go-onnx.md) funciona no WSL2 antes de investir no resto. Código descartável em `spike/`.

1. Clonar `metalagman/laya-go` na tag `v0.2.6`; ler `docs/official-export-v1.md`, `docs/bundle-format-v1.md` e os `Taskfile`.
2. Descobrir e registrar: versão de Python, dependências, origem e versão da `libtokenizers`, URL e SHA-256 do ONNX Runtime 1.29.0 linux-x64.
3. Rodar `task reference:verify-source`, `reference:verify-sdk`, `bundle:export`, `bundle:verify`. Registrar tempo, disco e RAM.
4. Programa mínimo com `-tags laya_native`: `NewRuntime` → `OpenModelDir` → `Predict(JSONState(estado de exemplo), [2 NoulQuestion])`. Imprimir probabilidades, `Usage` e latência (10 execuções).
5. Testar o limite: estado crescente até `ErrInputTooLong`; registrar o `InputTooLongError`.
6. (Opcional) `InputTokenLimit: 2048` e `4096`: o bundle aceita? latência?
7. Escrever `scripts/versions.env` com tudo o que funcionou.

**Saída:** nota em `Project/docs/plans/fase-0-resultado.md` com os números. Se falhar em 3 ou 4, abrir ADR substituindo a 0004 (plano B: `lengoman/laya-go` + servidor Python) e ajustar fases 3, 4 e 6.

---

## Fase 1 — Núcleo `compaction` (estágios 1–7)

Ordem pensada para cada passo depender só dos anteriores. Cada item: teste portado → implementação.

1. `go mod init github.com/fsvxavier/fast-laya-compaction`; `Makefile` com `test`, `lint`.
2. `types.go`: `Message`, `ToolUse`, `ToolResult` (com `Input json.RawMessage`), `ToolCall`, `Decision`, `Action`, `Reason` (inclui `unfit`), `Options`, `Result`, `Stats`. Testes de (de)serialização JSON no formato da [API](../../description/05-api-daemon.md).
3. `options.go`: `DefaultOptions`, `ResolveOptions` (porte de `finite`/`floor`/`max`), derivação dos orçamentos por `InputTokenLimit`. Caso: *fills in defaults and ignores non-finite values*.
4. `tokens.go`: `EstimateTokens` + `tokenScale`. Caso: *charges words, digits and symbols separately*. Acrescentar vetores de teste gerados pelo original para 20 strings variadas (igualdade exata).
5. `text.go`: `truncate`, `abridge` por runa; `compactJSON` com `SetEscapeHTML(false)`; iteração ordenada de chaves.
6. `calls.go`: `CollectToolCalls`, `isPinned`, `GoalFromMessages`. Casos: *pairs each tool call…*, *ignores calls without a result*, *defaults the goal…*.
7. `state.go`: `State`, `HistoryEntry`, `ToolCalls` (estruturadas | linhas), `FitState` estágio a estágio, um teste por estágio. Casos: *sends the whole history…*, *truncates tool inputs before touching message text*, *shrinks old tool calls to one line…*, *abridges long texts oldest-first…*, e `ErrStateTooLarge`.
8. **Golden files 1–7**: script `tools/golden/` (Node) que roda `fitState` do original (commit `e3f262a`) sobre `testdata/stages/*.input.json` e grava o estado esperado; o teste Go compara byte a byte (após trocar nada — o texto de contexto é o mesmo).
9. `batch.go`: `QuestionsFor`, `BatchCalls` com custo de pergunta do Laya. Casos: *puts everything in one request…*, *splits questions…*, `ErrNoRoomForQuestions`.
10. `decide.go`: `DecideCall`. Caso: *keeps, drops the result, or drops the call…*.
11. `apply.go`: `ApplyDecisions` com `Source`/`Unchanged`/`SourceIndex`/`Changed`, `MessageChars`, `ReductionRatio`. Casos: *removes dropped calls and truncates dropped results*, *honours truncateHeadChars, including a zero head*, + origem.
12. `fakelaya/`: regras por id/ferramenta/função, gravação das requisições, injeção de erros.
13. `compact.go`: `Compact(ctx, messages, asker, opts)` — lotes em sequência, validação de respostas, stats, cancelamento. Casos: *resends the full state with every batch…*, *keeps everything without calling…*, *reports a tiny reduction…*, *rejects malformed answers*.

**Pronto quando:** `go test ./internal/compaction/...` verde; golden 1–7 idênticos ao original.

---

## Fase 2 — Estágio `windowed`, `unfit` e retries

1. `resources.go`: extração de recurso (`file_path`, `path`, `notebook_path`, `pattern`, `url`, `command`) do `Input`; testes com inputs de ferramentas reais do Claude Code (`Read`, `Edit`, `Write`, `Bash`, `Grep`, `Glob`, `WebFetch`).
2. `window.go`: `BuildWindows(messages, calls, candidates, opts) ([]Window, []unfitCall)`:
   1. formação das janelas;
   2. orçamento por janela;
   3. parte obrigatória (goal ≤ 25%);
   4. preenchimento por prioridade, pulando o que não cabe;
   5. ordenação por `i` e agrupamento por mensagem;
   6. degradação: inputs 60 → janela de 1 → `unfit`.
   Um teste por passo; golden `windowed`.
3. Integrar no `Compact`: `ErrStateTooLarge`/`ErrNoRoomForQuestions` → estágio 8; `stateStage = "windowed"`.
4. Retries: `ErrInputTooLong` (sentinela do núcleo, mapeada pelo adaptador) → refaz o lote como janela com ×0,85, até 2 vezes → `unfit`. Testes com `fakelaya` rejeitando N vezes.
5. `FuzzCompact` com as 6 invariantes.

**Pronto quando:** casos novos e fuzz (`-fuzztime 2m` local) verdes.

---

## Fase 3 — Adaptador Laya e calibração

1. `internal/layaasker/asker_stub.go` (`!laya_native`): `New` devolve `ErrNativeUnavailable`.
2. `asker_native.go` (`laya_native`): `New(ctx, Config)` abre runtime e modelo; `Ask` converte estado (`JSONState`) e perguntas, chama `Predict`, valida a ordem dos resultados, devolve probabilidades e `Usage.InputTokens`; mapeia `ErrInputTooLong` → `compaction.ErrInputTooLong`, `ErrQueueFull` → `ErrBusy`, demais → erro de inferência; `Close`.
3. Testes de integração (`laya_native`, pulam sem `LAYA_BUNDLE_DIR`).
4. Corpus sintético em `testdata/transcripts/` (≥ 10 sessões, 20–300 chamadas, mistura de `Read`/`Edit`/`Bash`/`Grep`, releituras do mesmo arquivo, erros).
5. `fast-laya compact --report` (pode nascer aqui, antes do daemon).
6. Calibrar `tokenScale` e `questionOverheadTokens`; atualizar padrões e a [ADR 0007](../../adrs/0007-estimativa-de-tokens-e-calibracao.md) com os números (como adendo, não reescrita).
7. Medir latência por inferência e por compactação; comparar com RNF-03.
8. (Opcional) experimento `InputTokenLimit` maior; se promissor, ADR nova.

**Pronto quando:** < 1% de `ErrInputTooLong` no corpus; p50 ≤ 50 ms por inferência.

---

## Fase 4 — Daemon e CLI

1. `internal/config`: flags + env + `.env`, precedência e validação ([07](../../description/07-configuracao.md)); recusa não-loopback.
2. `internal/server`: rotas, limite de corpo, semáforo não bloqueante (429), `readyz` com estados `loading`/`ready`/`unavailable`, corpo de erro sem conteúdo, cancelamento por `ctx`.
3. Logs `slog`: campos fixos (`compaction_id`, `calls`, `stage`, `requests`, `ms`, `reduction`); `--log-content`.
4. `cmd/fast-laya`: `serve` (escuta antes de carregar o modelo; shutdown gracioso), `compact`, `doctor`, `version`.
5. Testes `httptest` com `fakelaya`; teste de shutdown com compactação em curso.

**Pronto quando:** testes verdes; `make build && make serve` + `curl /readyz` = 200; `make doctor` verde.

---

## Fase 5 — Plugin TS

1. `plugin/` com `package.json`, `tsconfig`, `.claude-plugin/plugin.json` (userConfig da [06](../../description/06-plugin-claude-code.md)), `hooks/hooks.json`; e `.claude-plugin/marketplace.json` na **raiz do repositório** com `source: "./plugin"` (confirmar o formato exigido por `claude plugin marketplace add owner/repo` na versão atual).
2. Gerar `types/claude-code.d.ts` com a versão atual do Claude Code; confirmar `session.compact`, `turn.complete`, `$.http.fetch`, `$.session.usage`, `$.ui.log`, `$.ui.toast`.
3. Portar `hooks/fast-jev.ts` → `hooks/fast-laya.ts`: remover chave e `buildJevRequest`; cliente do daemon com timeout; `toSessionMessages` por índices com validação de limites; mensagens de fallback.
4. Portar `tests/hook.test.ts` + casos de fallback novos.
5. Teste manual: sessão real com `CLAUDE_CODE_ENABLE_FUNCTION_HOOKS=1 claude --plugin-dir ./plugin`, forçar compactação, conferir toast, `decisions:` e que a sessão continua coerente.

**Pronto quando:** testes do hook verdes; teste manual registrado em `Project/docs/plans/fase-5-resultado.md`.

---

## Fase 6 — Provisionamento, Docker e README

1. `scripts/versions.env` final (da Fase 0).
2. `scripts/provision.sh` idempotente, com `--prefix`, `--keep-cache`, `--offline`; `shellcheck` limpo.
3. `Makefile`: `provision`, `build`, `build-stub`, `serve`, `doctor`, `test`, `test-native`, `lint`, `docker`.
4. `Dockerfile` multi-stage, usuário não-root, `HEALTHCHECK`; `.dockerignore`.
5. Exemplo de unidade `systemd --user`.
6. `README.md` na raiz: o que é, requisitos, início rápido (provision → build → serve → plugin), Docker, configuração, limitações, créditos (original MIT, Laya Apache 2.0, `laya-go` MIT, ONNX Runtime MIT) e `NOTICE`.
7. Atualizar o `CLAUDE.md` da raiz com comandos reais e remover do `.gitignore` as entradas herdadas de outro projeto.
8. Validação do zero: máquina/WSL limpa → `make provision && make build && make doctor`; `docker build` + `docker run -p 127.0.0.1:8610:8610` + `/readyz`.

**Pronto quando:** os dois caminhos funcionam do zero seguindo só o README.

---

## Rastreabilidade

| Requisito | Fase |
|---|---|
| RF-01–RF-08 | 1 |
| RF-09, RF-10 | 2 |
| RF-11, RF-13, RF-16 | 4 (RF-13/16 começam na 1) |
| RF-12 | 3–4 |
| RF-14 | 5 |
| RF-15 | 0, 6 |
| RNF-03, RNF-04 | 3 |
| RNF-02 | 4 |
| RNF-07 | 0, 6 |
| RNF-08 | 1–3 |
| RNF-09 | 1 (golden) |
