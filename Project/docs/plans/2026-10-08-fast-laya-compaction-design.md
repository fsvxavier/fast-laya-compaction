# fast-laya-compaction — Design

- **Data:** 2026-10-08
- **Status:** Validado (sessão de brainstorming)
- **Referência:** [`tamaratran/fast-jev-compaction@e3f262a`](../../references/fast-jev-compaction.md)

Este documento consolida o desenho validado em cinco seções. O detalhe de cada parte está em `description/`; as decisões, em `adrs/`.

## Decisões tomadas na sessão

| # | Pergunta | Escolha | ADR |
|---|---|---|---|
| 1 | Como executar o Laya a partir de Go? | Dentro do processo, `metalagman/laya-go` + ONNX Runtime (cgo, `laya_native`) | 0004 |
| 2 | Como encaixar o histórico em ~1.024 tokens? | Porte fiel dos estágios 1–7 + estágio novo `windowed` no lugar do `throw` | 0005, 0006 |
| 3 | Como integrar com o Claude Code? | Daemon local `fast-laya serve` + hook TS fino via `$.http.fetch` | 0008 |
| 4 | Como provisionar modelo e bibliotecas? | `make provision` (principal) + Dockerfile multi-stage (alternativa) | 0009 |
| 5 | O que entregar agora? | Só documentação | — |

## Seção 1 — Visão geral e componentes

Substituir o auto-compact do Claude Code apagando só tool calls/resultados que o Laya julga desnecessários. Tudo local, em CPU, sem chave e sem custo. Texto de usuário e assistente nunca é alterado.

Componentes: `internal/compaction` (algoritmo, sem I/O, depende de `Asker`), `internal/layaasker` (adaptador cgo), `fakelaya` (testes), `internal/server` (HTTP loopback), `cmd/fast-laya` (`serve`, `compact`, `doctor`), `plugin/hooks/fast-laya.ts`, `scripts/provision.sh` + `Dockerfile`.

Fora do escopo: GPU, checkpoints `english`/`typed-decisions`, resumo por LLM, UI, autenticação no daemon.

→ [01 — Visão geral](../../description/01-visao-geral.md), [04 — Arquitetura](../../description/04-arquitetura.md)

## Seção 2 — Estado, orçamentos e estágio de janela

- Orçamentos: `maxRequestTokens = L − 64 = 960`, `maxStateTokens = 800` (proporção do original).
- Estimativa: `estimateTokens` do original × `tokenScale` (calibrado na Fase 3); custo de pergunta = instruções + 8.
- Textos fixos e perguntas idênticos ao original, em inglês; `NewNoulQuestion(id, instructions, "", "")`, `TrueProbability()`.
- Estágios 1–7 idênticos. Se couber, lotes compartilham o estado.
- Estágio 8 `windowed`: janelas de `windowCalls = 2`; obrigatório (context, goal ≤ 25%, chamadas da janela) + opcional por prioridade (mesmo recurso depois → fixadas recentes → posteriores → anteriores); ordenado por `i`.
- Se o obrigatório não cabe: inputs 60 → janela de 1 → `keep`/`unfit`.
- `ErrInputTooLong`: refaz com ×0,85 até 2 vezes → `unfit`.
- Stats novas: `unfit`, `windows`, `retries`, tokens estimados × reportados.

→ [03 — Algoritmo](../../description/03-algoritmo.md)

## Seção 3 — Daemon, hook e erros

- `127.0.0.1:8610`; recusa não-loopback sem `--allow-remote`.
- `POST /v1/compact` → `{messages, decisions, stats}` com `source`/`unchanged`/`sourceIndex` para o hook reaproveitar objetos do engine.
- Uma compactação por vez; concorrente → `429`. Erros: 400, 413, 429, 500, 503.
- Hook: mesma lógica do `fast-jev.ts`; fallback em erro, timeout (`timeoutMs` 30 s), resposta malformada ou redução < `minReductionRatio`.
- `userConfig`: remove `apiKey`/`model`; adiciona `daemonUrl`, `windowCalls`, `timeoutMs`.
- Logs `slog` JSON sem conteúdo por padrão; `--log-content` para depuração.

→ [05 — API](../../description/05-api-daemon.md), [06 — Plugin](../../description/06-plugin-claude-code.md), [07 — Configuração](../../description/07-configuracao.md)

## Seção 4 — Testes, avaliação e provisionamento

- Núcleo: porte dos 20 casos aplicáveis do original + casos novos (janela, `unfit`, retries, origem); golden files por estágio, de preferência gerados rodando o original em TS; fuzz das invariantes.
- Servidor com `httptest`; adaptador real atrás de `laya_native`; hook com `$` falso.
- `fast-laya compact --report` sobre transcripts sintéticos: redução, distribuição de ações, estágio, calibração de tokens.
- `make provision` / `make doctor`; Dockerfile exporter → libs → builder → runtime; `scripts/versions.env` como fonte única.

→ [08 — Provisionamento](../../description/08-provisionamento.md), [09 — Testes](../../description/09-testes-e-avaliacao.md), [10 — Riscos](../../description/10-riscos.md)

## Seção 5 — Estrutura da documentação

`Project/{README.md, description/, adrs/, references/, docs/plans/}` — ver [README](../../README.md).

## Questões em aberto (resolvidas na implementação)

1. Origem e versão exatas da `libtokenizers` esperada pelo `laya-go` (Fase 0).
2. Versões de Python e dependências do runbook de exportação (Fase 0).
3. ~~Se o bundle aceita `InputTokenLimit` > 1.024~~ — **resolvido na Fase 0: não**; o `laya-go` só reduz o limite (`max_len = 1024`).
4. Valores calibrados de `tokenScale` e `questionOverheadTokens` (Fase 3).
5. Se function hooks expõem `$.session.usage()` e `$.http.fetch` com timeout nativo na versão atual do Claude Code (Fase 5).
