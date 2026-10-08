# 0008 — Daemon local em Go e hook TypeScript fino

- **Status:** Aceita
- **Data:** 2026-10-08
- **Relacionadas:** ADR 0002, ADR 0004, ADR 0011

## Contexto

O original integra com o Claude Code por um módulo de function hooks em TypeScript (`hooks/fast-jev.ts`), que intercepta `session.compact` e `turn.complete` e chama o Jev via `$.http.fetch`. Function hooks só carregam módulos TS/JS.

O modelo ocupa ~1,2 GiB e o ONNX Runtime leva segundos para carregar. Compactações são raras, mas carregar o modelo a cada uma somaria esse tempo ao timeout do hook.

O hook do original devolve, para mensagens intactas, **os mesmos objetos** que recebeu do Claude Code (com handles internos), e objetos novos só para as editadas. Por HTTP essa identidade se perde.

## Decisão

1. **`fast-laya serve`**: daemon Go em `127.0.0.1:8610` que carrega o modelo uma vez e expõe `POST /v1/compact`, `/healthz`, `/readyz` ([05](../description/05-api-daemon.md)).
2. **`fast-laya compact`**: o mesmo algoritmo em modo único (stdin → stdout), para depuração e avaliação.
3. **Hook TS fino** derivado do original: troca a chamada ao Jev por `$.http.fetch` ao daemon com timeout; mantém fallback, `minReductionRatio`, `compactAtPercent`, logs e toasts.
4. **Identidade por índice**: a resposta traz `source` e `unchanged` por mensagem e `sourceIndex`/`changed` por bloco; o hook reaproveita `event.messages[source]` e seus blocos. Índices fora dos limites são tratados como resposta malformada (fallback).
5. **Uma compactação por vez**: requisição concorrente recebe `429`.

## Alternativas consideradas

- **CLI chamado a cada compactação** — recarrega modelo e ORT toda vez; não está claro se function hooks podem iniciar processos, enquanto `$.http.fetch` é a API que o original já usa.
- **Só biblioteca e CLI, sem plugin** — não substitui o auto-compact, que é o principal caso de uso.
- **Lógica no hook e daemon como "Laya remoto"** (o daemon só responderia `Predict`) — manteria o algoritmo em TS e o porte em Go perderia o sentido.

## Consequências

- O hook muda pouco em relação ao original; fácil de comparar.
- Um processo residente (~2 GiB). Se estiver parado, o hook faz fallback para o resumo padrão — nunca bloqueia o usuário.
- O contrato HTTP vira API pública entre plugin e daemon; mudanças incompatíveis exigem `/v2`.
