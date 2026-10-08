# 07 — Configuração

Precedência no daemon: **flag > variável de ambiente > `.env` gerado pelo provisionamento > padrão**. Opções do algoritmo enviadas em `POST /v1/compact` prevalecem, por requisição, sobre os padrões do daemon.

## Daemon e CLI (`fast-laya`)

| Flag | Variável | Padrão | Descrição |
|---|---|---|---|
| `--addr` | `FAST_LAYA_ADDR` | `127.0.0.1:8610` | Endereço de escuta |
| `--allow-remote` | `FAST_LAYA_ALLOW_REMOTE` | `false` | Permite endereço que não seja loopback |
| `--onnxruntime-lib` | `LAYA_ONNXRUNTIME_LIBRARY` | — (obrigatória) | Caminho de `libonnxruntime.so.1.29.0` |
| `--tokenizers-lib` | `LAYA_TOKENIZERS_LIBRARY` | — (obrigatória) | Caminho de `libtokenizers.a` (estática; também usada em build via `CGO_LDFLAGS=-L`) |
| `--bundle-dir` | `LAYA_BUNDLE_DIR` | — (obrigatória) | Diretório do bundle `laya-multilingual-v1` |
| `--input-token-limit` | `FAST_LAYA_INPUT_TOKEN_LIMIT` | limite do bundle (1.024) | **Só reduz** o limite do bundle (`laya-go` ignora valores maiores); útil para testar janelas menores |
| `--token-scale` | `FAST_LAYA_TOKEN_SCALE` | `1.0` | Multiplicador da estimativa de tokens |
| `--max-body` | `FAST_LAYA_MAX_BODY` | `64MiB` | Limite do corpo da requisição |
| `--shutdown-timeout` | `FAST_LAYA_SHUTDOWN_TIMEOUT` | `30s` | Espera pela compactação em curso ao encerrar |
| `--log-level` | `FAST_LAYA_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `--log-content` | `FAST_LAYA_LOG_CONTENT` | `false` | Inclui estado e textos nos logs de `debug`. Só para depuração local |
| `--env-file` | — | `~/.local/share/fast-laya/.env` | `.env` gerado por `make provision` |

O daemon define `ORT_DISABLE_TELEMETRY=1` no próprio processo antes de criar o runtime, independentemente do ambiente.

## Opções do algoritmo

| Opção | Padrão | Faixa | Descrição |
|---|---:|---|---|
| `goal` | `""` | — | Objetivo da sessão; vazio = últimos 3 prompts do usuário |
| `keepThreshold` | `0.5` | [0, 1] | Probabilidade mínima para manter |
| `preserveRecentMessages` | `6` | ≥ 0 | Mensagens finais fixadas (a primeira é sempre fixada) |
| `maxStateTokens` | `800` | ≥ 1 | Orçamento estimado do estado global |
| `maxRequestTokens` | `960` | ≥ 1 | Orçamento estimado de estado + perguntas por inferência |
| `truncateHeadChars` | `300` | ≥ 0 | Caracteres mantidos de um resultado truncado |
| `windowCalls` | `2` | ≥ 1 | Chamadas por janela no estágio 8 |

Valores não finitos ou fora da faixa: no daemon, `400 invalid_request`; na biblioteca, `ResolveOptions` aplica as mesmas regras do original (`finite`, `floor`, `max`).

Se `maxRequestTokens > InputTokenLimit`, o daemon rejeita com `400`, porque toda inferência estouraria.

## Plugin

Ver [06 — userConfig](06-plugin-claude-code.md#userconfig-pluginclaude-pluginpluginjson).

## Arquivo `.env` gerado

```sh
LAYA_ONNXRUNTIME_LIBRARY=/home/<user>/.local/share/fast-laya/lib/libonnxruntime.so.1.29.0
LAYA_TOKENIZERS_LIBRARY=/home/<user>/.local/share/fast-laya/lib/libtokenizers.a
LAYA_BUNDLE_DIR=/home/<user>/.local/share/fast-laya/bundle/laya-multilingual-v1
```

Não contém segredos; ainda assim fica fora do git (`.gitignore` já ignora `.env`).
