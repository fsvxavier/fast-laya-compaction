# Referência: fast-jev-compaction

Análise do projeto original que o `fast-laya-compaction` porta para Go.

- Repositório: <https://github.com/tamaratran/fast-jev-compaction>
- Commit analisado: `e3f262a7f4d42bd8dd32ced30d26176f7cb545b0` (2026-09-17)
- Versão do plugin: `0.3.0` · Licença: MIT · Linguagem: TypeScript

## O que faz

Substitui o auto-compact do Claude Code. Em vez de resumir turnos antigos, **nunca reescreve nada**: só apaga tool calls e tool results que o modelo Jev (API System One da TypeSafe, paga) considera desnecessários. Texto de usuário e assistente nunca é tocado.

## Arquivos

| Arquivo | Linhas | Papel |
|---|---:|---|
| `src/types.ts` | 202 | Tipos: `Message`, `ToolUse`, `ToolResult`, `ToolCall`, `CallDecision`, `HistoryEntry`, `CompactionState`, `CompactOptions`, `CompactResult`, `JevAsker` |
| `src/state.ts` | 304 | `estimateTokens`, `collectToolCalls`, `fitState` (estágios de redução), `goalFromMessages` |
| `src/compact.ts` | 309 | `resolveOptions`, `questionsFor`, `batchCalls`, `decideCall`, `applyDecisions`, `compact`, `reductionRatio` |
| `src/request.ts` | 80 | `buildJevRequest`, `parseJevResponse`, `noulAnswer` |
| `src/client.ts` | 43 | `JevClient` (fetch + `TYPESAFE_API_KEY`) |
| `src/index.ts` | 6 | `compactMessages` |
| `hooks/fast-jev.ts` | 310 | Adaptador de function hooks (`session.compact`, `turn.complete`) |
| `.claude-plugin/plugin.json` | — | `userConfig` do plugin |
| `tests/*.test.ts` | — | 23 casos da biblioteca (3 de request/client) + 6 do hook, com Jev falso |
| `demo/JevDemo/` | — | App SwiftUI de demonstração (fora do escopo do porte) |

## Tipos centrais

```ts
interface Message { role: 'user' | 'assistant'; text: string; toolUses: ToolUse[]; toolResults?: ToolResult[] }
interface ToolUse  { tool_use_id: string; tool: string; input: Record<string, unknown>; text?: string; isError?: boolean }
interface ToolResult { tool_use_id: string; text: string; isError?: boolean }

interface ToolCall {
  id: string;            // t1, t2, ... (usado no estado e nos nomes das perguntas)
  tool_use_id: string; tool: string; input: Record<string, unknown>;
  callIndex: number; resultIndex: number; resultChars: number; isError: boolean;
  pinned: boolean;       // na primeira mensagem ou nas `preserveRecentMessages` mais novas
}

type CallAction = 'keep' | 'drop_result' | 'drop_call';
interface CallDecision { id; tool; keepCall: number; keepResult: number; action: CallAction;
  reason: 'pinned' | 'kept' | 'result_dropped' | 'call_dropped' }
```

Observação: `ToolUse.text`/`isError` existem porque o Claude Code anexa o resultado ao próprio tool_use em alguns casos; `applyDecisions` trunca ambos.

## Constantes e textos fixos

```ts
STATE_CONTEXT =
  'A coding assistant conversation is being compacted to free context. `history` is the whole conversation so far, oldest first; tool outputs are replaced by a short `result` note and long texts may be abridged. Each question asks whether one tool call, or the full output of that call, still needs to stay in the history verbatim. Whatever is not kept is deleted permanently, but the assistant can always re-run a tool or re-read a file.'

INPUT_CHARS = [1000, 200, 60]
TEXT_HEAD = 400; TEXT_TAIL = 150
REQUEST_OVERHEAD_TOKENS = 20

DEFAULT_OPTIONS = { goal: '', keepThreshold: 0.5, preserveRecentMessages: 6,
                    maxStateTokens: 25_000, maxRequestTokens: 30_000, truncateHeadChars: 300 }
HOOK_DEFAULTS   = { compactAtPercent: 60, minReductionRatio: 0.25, model: 'jev-latest' }
```

Perguntas por chamada (`questionsFor`):

```ts
[`call_${id}`]:   noul — `Tool call ${id} (${tool}) should stay in the history: knowing this call was made, with its input, still matters for what the assistant does next`
[`result_${id}`]: noul — `The full output of tool call ${id} (${tool}, ${resultChars} chars) should stay in the history verbatim: the assistant still needs its contents and re-running the tool would not do`
```

## Algoritmo (resumo fiel)

1. **`collectToolCalls`**: indexa `tool_result` por `tool_use_id`; cada `tool_use` com resultado vira um `ToolCall` (`t1`, `t2`, …). Chamada sem resultado não é candidata. `pinned` se a mensagem da chamada **ou** do resultado for a primeira ou estiver entre as `preserveRecentMessages` finais.
2. **`goalFromMessages`**: últimos 3 prompts do usuário (mensagens `user` com texto e sem `toolResults`), cada um truncado a 500 chars, unidos por `\n`.
3. **Estado**: `{context, goal, history}`; cada `HistoryEntry` = `{i, role, text, tool_calls?: [{id, tool, input (JSON truncado), result: "ok|error, N chars (omitted)"}]}`. Mensagens sem texto e sem chamadas são omitidas.
4. **`fitState`** — encolhe até caber em `maxStateTokens`, devolvendo o nome do estágio:
   1. `full` (inputs ≤ 1000 chars)
   2. `inputs<=200`, `inputs<=60`
   3. `texts abridged` — `abridge(text, 400, 150)` com marcador `[… N chars omitted …]`, ordem: não fixadas antes, mais antigas primeiro
   4. `old messages collapsed` — texto de mensagens não fixadas vira `[… N chars omitted …]`
   5. `old calls compacted` — chamadas não fixadas viram uma linha: `t3 Read file_path=src/x.go → ok 4213ch`
   6. `old messages left out` — remove entradas não fixadas sem chamadas
   7. `old calls merged` — funde sequências adjacentes de entradas só-chamada (mesmo papel)
   8. senão `throw new Error('history too large for Jev …')`
5. **`batchCalls`**: orçamento = `maxRequestTokens − stateTokens − 20`; acumula perguntas até estourar; se uma única chamada não cabe, `throw`.
6. **Pedido**: lotes em paralelo (`Promise.all`), **o mesmo estado completo** em todos.
7. **`decideCall`**: `pinned → keep`; `keepResult ≥ t → keep`; `keepCall ≥ t → drop_result`; senão `drop_call`. Chamada sem resposta recebe `{keepCall: 1, keepResult: 1}`.
8. **`applyDecisions`**: `drop_call` remove tool_use e tool_result; `drop_result` mantém cabeça de `truncateHeadChars` + nota `[fast-jev-compaction truncated N chars of this tool result (error)?; re-run the tool if needed]` (só se o texto passar de `head + 120`). Mensagens que ficam vazias somem; mensagens intactas voltam como **o mesmo objeto**.
9. **Estatísticas**: `messagesBefore/After`, `charsBefore/After`, `calls`, `kept`, `resultsDropped`, `callsDropped`, `pinned`, `stateTokens`, `stateStage`, `requests`, `ms`. `reductionRatio = (before − after) / before`.

### Estimativa de tokens

```ts
TOKEN_PIECES = /[A-Za-z]+|\d+|[^\sA-Za-z\d]/g
palavra: 1 + floor((len − 1) / 6) · dígitos: len / 2 · outro símbolo: 0.9 · total: ceil
```

Calibrada contra o uso reportado pelo Jev: fica 2–18% acima do real; uma razão chars/token simples subestimava estados JSON em até 40%.

## Hook (`hooks/fast-jev.ts`)

- `session.compact`: resolve config e chave → `compact()` → `toSessionMessages` (reaproveita objetos/handles do engine para o que não mudou) → registra linhas `decisions:` (até 4096 chars por linha) → se `reductionRatio < minReductionRatio` faz fallback (`next(event)`), senão retorna `{messages}`. Qualquer exceção → toast + fallback.
- `turn.complete`: se `context.percent ≥ compactAtPercent`, chama `$.session.compact()` com trava `compacting`.
- Requer Claude Code 2.1.274+ e `CLAUDE_CODE_ENABLE_FUNCTION_HOOKS=1`; tipos gerados em `types/claude-code.d.ts`.

## Limitações declaradas pelo original

- Tokens estimados, não contados.
- Probabilidades do Jev informam mas não garantem; o assistente pode re-executar ferramentas.
- O estado completo é reenviado a cada lote (custo cresce com o histórico).
