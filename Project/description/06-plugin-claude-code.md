# 06 — Plugin do Claude Code

## Papel

`plugin/hooks/fast-laya.ts` é um adaptador fino, derivado de `hooks/fast-jev.ts` do original. Não contém nenhuma lógica de compactação: lê a configuração, chama o daemon, reconstrói as mensagens e decide entre aplicar o resultado ou cair no resumo padrão.

## Pré-requisitos

Os mesmos do original:

- Claude Code **2.1.274+**, com function hooks habilitados: `export CLAUDE_CODE_ENABLE_FUNCTION_HOOKS=1`.
- Tipos em `plugin/types/claude-code.d.ts`, gerados pelo Claude Code; regenerar após cada atualização do Claude Code, porque function hooks estão em acesso antecipado.
- Daemon `fast-laya serve` rodando e com `/readyz` = 200.

## Instalação

```sh
export CLAUDE_CODE_ENABLE_FUNCTION_HOOKS=1
claude plugin marketplace add fsvxavier/fast-laya-compaction
claude plugin install fast-laya-compaction@fast-laya-compaction

# desenvolvimento local
CLAUDE_CODE_ENABLE_FUNCTION_HOOKS=1 claude --plugin-dir ./plugin
```

## Hooks

### `session.compact`

```
1. config = resolveHookConfig(options)
2. response = $.http.fetch(`${daemonUrl}/v1/compact`, POST {messages: event.messages, options})
      com timeout de timeoutMs (Promise.race)
3. status ≠ 200, timeout, JSON malformado      → notify("fallback … (motivo)"); return next(event)
4. messages = toSessionMessages(event.messages, response.messages)   // usa source/unchanged/sourceIndex
5. registra decisionLogLines(response) em $.ui.log                   // mesmo formato do original
6. response.stats.reductionRatio < minReductionRatio → notify(fallback); return next(event)
7. notify("kept X/Y messages, no summary (…)"); return {messages}
```

`toSessionMessages` substitui a busca por identidade de objeto do original:

- `unchanged: true` → devolve `event.messages[source]` (o próprio objeto do engine, com handle).
- senão monta uma mensagem nova; cada bloco com `sourceIndex` e sem `changed` reaproveita o objeto `event.messages[source].toolUses[sourceIndex]` (ou `toolResults`); bloco com `changed: true` vira um objeto novo sem handle.

Validação defensiva: se `source` ou `sourceIndex` apontar para fora dos limites, o hook trata como resposta malformada e faz fallback.

### `turn.complete`

Igual ao original: se `(await $.session.usage()).context.percent ≥ compactAtPercent`, chama `$.session.compact()` com a trava `compacting` para não disparar em paralelo. Erros só geram `$.ui.log("auto-compact skipped (…)")`.

## `userConfig` (`plugin/.claude-plugin/plugin.json`)

| Opção | Padrão | Origem |
|---|---:|---|
| `daemonUrl` | `http://127.0.0.1:8610` | **novo** |
| `timeoutMs` | `30000` | **novo** |
| `keepThreshold` | `0.5` | original |
| `preserveRecentMessages` | `6` | original |
| `compactAtPercent` | `60` | original |
| `minReductionRatio` | `0.25` | original |
| `maxStateTokens` | `800` | original, padrão novo |
| `maxRequestTokens` | `960` | original, padrão novo |
| `truncateHeadChars` | `300` | original |
| `windowCalls` | `2` | **novo** |
| ~~`apiKey`~~ | — | removido |
| ~~`model`~~ | — | removido (o modelo é o bundle do daemon) |

Campos numéricos ausentes ou não finitos não são enviados; o daemon aplica seus padrões. `daemonUrl` deve ser `http://127.0.0.1:*` ou `http://localhost:*`; outro host gera um aviso no log, mas é aceito (o usuário pode ter o daemon em Docker com outro mapeamento).

## Mensagens ao usuário

Mantêm o formato do original para facilitar comparação:

- sucesso: `kept 42/88 messages, no summary (63% reduction; 12 kept, 20 results truncated, 9 call_dropped, 6 pinned; state ~790 tokens (windowed) in 21 request(s))`
- acréscimo quando houver: `, 3 unfit`
- fallback: `fallback to built-in summary (<motivo>)`, com motivos como `daemon not reachable at http://127.0.0.1:8610`, `daemon busy`, `daemon not ready`, `timeout after 30000 ms`, `below 25% minimum: …`.
- diagnóstico: linhas `decisions: t3:Read:drop_result/call=0.81/result=0.12 …` (até 4.096 caracteres por linha).

## Testes

Porte de `tests/hook.test.ts` com `$.http.fetch` falso:

1. lê `userConfig` e aplica padrões;
2. devolve os objetos do engine para mensagens/blocos intactos e cópias sem handle para os alterados;
3. preserva mensagens com resultado truncado curto;
4. chama o daemon e reporta o resultado;
5. quebra o log de decisões em linhas abaixo do limite;
6. faz fallback em: daemon fora do ar, 429, 503, 500, timeout, JSON malformado, `source` fora dos limites, redução baixa.
