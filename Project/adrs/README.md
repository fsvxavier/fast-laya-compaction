# ADRs — Architecture Decision Records

Decisões de arquitetura do `fast-laya-compaction`, no formato de Michael Nygard ([ADR 0001](0001-registrar-decisoes-com-adrs.md)).

| # | Título | Status | Data |
|---|---|---|---|
| [0001](0001-registrar-decisoes-com-adrs.md) | Registrar decisões com ADRs | Aceita | 2026-10-08 |
| [0002](0002-go-como-linguagem-principal.md) | Go como linguagem principal | Aceita | 2026-10-08 |
| [0003](0003-laya-no-lugar-do-jev.md) | Laya no lugar do Jev | Aceita | 2026-10-08 |
| [0004](0004-inferencia-in-process-com-laya-go-onnx.md) | Inferência dentro do processo com `metalagman/laya-go` e ONNX Runtime | Aceita (condicionada à Fase 0) | 2026-10-08 |
| [0005](0005-porte-fiel-do-algoritmo.md) | Porte fiel do algoritmo do `fast-jev-compaction` | Aceita | 2026-10-08 |
| [0006](0006-estagio-de-janela-por-lote.md) | Estágio de janela por lote quando o estado global não cabe | Aceita | 2026-10-08 |
| [0007](0007-estimativa-de-tokens-e-calibracao.md) | Estimativa heurística de tokens com calibração | Aceita | 2026-10-08 |
| [0008](0008-daemon-local-e-hook-ts-fino.md) | Daemon local em Go e hook TypeScript fino | Aceita | 2026-10-08 |
| [0009](0009-provisionamento-script-e-docker.md) | Provisionamento por script local, com Docker como alternativa | Aceita | 2026-10-08 |
| [0010](0010-politica-conservadora-de-falhas.md) | Política conservadora de falhas | Aceita | 2026-10-08 |
| [0011](0011-privacidade-logs-e-loopback.md) | Privacidade: loopback e logs sem conteúdo | Aceita | 2026-10-08 |

## Status possíveis

`Proposta` → `Aceita` → (`Substituída por NNNN` | `Obsoleta`). Uma ADR aceita não é editada no mérito; uma mudança de decisão gera uma ADR nova que a substitui.

## Template

```markdown
# NNNN — Título no imperativo ou como decisão

- **Status:** Proposta
- **Data:** AAAA-MM-DD
- **Relacionadas:** ADR XXXX

## Contexto

Forças em jogo: o problema, restrições, fatos verificados (com fonte).

## Decisão

O que foi decidido, em voz ativa ("Usaremos…").

## Alternativas consideradas

- **Opção A** — por que não.

## Consequências

Positivas, negativas e o que fica para acompanhar.
```
