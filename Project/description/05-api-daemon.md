# 05 — API do daemon

Base: `http://127.0.0.1:8610`. JSON UTF-8. Sem autenticação (loopback; ver [ADR 0011](../adrs/0011-privacidade-logs-e-loopback.md)).

## `POST /v1/compact`

### Requisição

```json
{
  "messages": [
    {
      "role": "user",
      "text": "Corrija o teste que falha em pkg/x",
      "toolUses": [],
      "toolResults": []
    },
    {
      "role": "assistant",
      "text": "",
      "toolUses": [
        { "tool_use_id": "toolu_01", "tool": "Read", "input": { "file_path": "pkg/x/x.go" } }
      ]
    },
    {
      "role": "user",
      "text": "",
      "toolUses": [],
      "toolResults": [
        { "tool_use_id": "toolu_01", "text": "package x\n…", "isError": false }
      ]
    }
  ],
  "options": {
    "goal": "",
    "keepThreshold": 0.5,
    "preserveRecentMessages": 0,
    "maxStateTokens": 800,
    "maxRequestTokens": 960,
    "truncateHeadChars": 300,
    "windowCalls": 2
  }
}
```

- O formato das mensagens é o subconjunto de `SessionMessage` usado pelo original: `role`, `text`, `toolUses[]` (`tool_use_id`, `tool`, `input`, `text?`, `isError?`), `toolResults[]` (`tool_use_id`, `text`, `isError?`). Campos extras são ignorados.
- `options` é opcional; cada campo ausente assume o padrão do daemon ([07](07-configuracao.md)).
- Limite de corpo: 64 MiB (configurável). Acima disso, `413`.

### Resposta `200`

```json
{
  "messages": [
    { "source": 0, "unchanged": true,  "role": "user", "text": "Corrija o teste…", "toolUses": [], "toolResults": [] },
    { "source": 1, "unchanged": true,  "role": "assistant", "text": "", "toolUses": [ { "sourceIndex": 0, "tool_use_id": "toolu_01", "tool": "Read", "input": { "file_path": "pkg/x/x.go" } } ] },
    { "source": 2, "unchanged": false, "role": "user", "text": "", "toolUses": [], "toolResults": [ { "sourceIndex": 0, "tool_use_id": "toolu_01", "text": "package x\n…\n[fast-laya-compaction truncated 3913 chars of this tool result; re-run the tool if needed]", "isError": false } ] }
  ],
  "decisions": [
    { "id": "t1", "tool": "Read", "keepCall": 0.81, "keepResult": 0.12, "action": "drop_result", "reason": "result_dropped" }
  ],
  "stats": {
    "messagesBefore": 3, "messagesAfter": 3,
    "charsBefore": 4260, "charsAfter": 390,
    "calls": 1, "kept": 0, "resultsDropped": 1, "callsDropped": 0, "pinned": 0, "unfit": 0,
    "stateTokens": 214, "stateStage": "full",
    "requests": 1, "windows": 0, "retries": 0,
    "inputTokensReported": 251, "inputTokensEstimated": 238,
    "reductionRatio": 0.908,
    "ms": 41
  }
}
```

Campos de origem:

| Campo | Onde | Significado |
|---|---|---|
| `source` | mensagem | índice da mensagem de entrada que originou esta |
| `unchanged` | mensagem | `true` se a mensagem é idêntica à de entrada; o hook deve devolver o objeto original do Claude Code |
| `sourceIndex` | `toolUses[i]`, `toolResults[i]` | posição do bloco na mensagem de origem; se o bloco não foi alterado, o hook reaproveita o objeto original |

Um bloco alterado (resultado truncado) vem com `"changed": true`; o hook cria um objeto novo sem handle, como o `toolUseSummary`/`toolResultSummary` do original. Mensagens removidas simplesmente não aparecem.

`reductionRatio` vem pronto para o hook não precisar recalcular.

### Erros

Corpo de erro: `{"error": {"code": "<código>", "message": "<texto>"}}`. A mensagem nunca inclui conteúdo do transcript.

| HTTP | `code` | Quando |
|---|---|---|
| 400 | `invalid_request` | JSON inválido, `role` desconhecido, opção fora da faixa (ex.: `keepThreshold` fora de [0,1], orçamentos ≤ 0) |
| 405 | `method_not_allowed` | método diferente de POST |
| 413 | `payload_too_large` | corpo acima do limite |
| 429 | `busy` | outra compactação em curso |
| 499 | `canceled` | cliente cancelou (só aparece em log; o cliente já se foi) |
| 500 | `inference_failed` | erro não recuperável do runtime ou resposta malformada do modelo |
| 503 | `not_ready` | modelo carregando ou indisponível (`ErrNativeUnavailable`, binário sem `laya_native`) |

`ErrInputTooLong` nunca chega ao cliente: é tratado dentro do algoritmo (retries e `unfit`).

## `GET /healthz`

`200 {"status":"ok"}` enquanto o processo estiver de pé.

## `GET /readyz`

- `200 {"status":"ready","bundleId":"sha256:…","profile":"laya-multilingual-v1","inputTokenLimit":1024,"version":"0.1.0"}`
- `503 {"status":"loading"}` ou `503 {"status":"unavailable","reason":"…"}`

## `GET /v1/info`

Configuração efetiva (padrões do algoritmo, limites, versões de `laya-go` e ONNX Runtime). Sem caminhos absolutos do usuário além do diretório do bundle.

## CLI equivalente

```sh
fast-laya compact [--options opts.json] < request.json > response.json
fast-laya compact --report testdata/transcripts/*.json
```

`compact` aceita o mesmo corpo de `POST /v1/compact` e escreve a mesma resposta. Códigos de saída: `0` sucesso, `2` requisição inválida, `3` modelo indisponível, `1` outros erros.
