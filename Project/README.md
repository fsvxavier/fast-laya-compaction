# fast-laya-compaction — Documentação do projeto

Compactação de sessões do Claude Code **sem resumo, local e sem custo**: porte em Go do [`fast-jev-compaction`](references/fast-jev-compaction.md), trocando o modelo pago Jev pelo modelo aberto [Laya](references/laya-e-laya-go.md), executado em CPU dentro de um daemon local.

> Status: Fase 0 concluída e Fase 1 (núcleo `internal/compaction`) em andamento. A inferência nativa do Laya funciona no WSL2, mas a sonda de qualidade não mostrou discriminação do modelo zero-shot e a latência é alta: leia [fase-0-resultado.md](docs/plans/fase-0-resultado.md) antes de continuar. Plano: [implementação](docs/plans/2026-10-08-fast-laya-compaction-implementation.md).

## Em uma página

- **O que faz.** Quando o Claude Code vai compactar o contexto, o hook envia o transcript ao daemon `fast-laya serve`. Para cada tool call antiga, o Laya responde duas perguntas: a chamada ainda importa? o resultado inteiro ainda importa? Conforme as probabilidades, a chamada fica, fica com o resultado truncado a 300 caracteres, ou sai. Nada é resumido nem reescrito.
- **Por que.** O resumo perde caminhos, erros exatos e restrições. O original resolve isso, mas cada compactação custa uma chamada paga e envia o transcript para fora.
- **Como roda.** `metalagman/laya-go` + ONNX Runtime 1.29.0 (cgo), checkpoint `laya-multilingual-v1`, em linux/amd64 (inclui WSL2). Sem Python em tempo de execução.
- **A diferença técnica principal.** O Laya aceita 1.024 tokens por inferência; o Jev, ~30.000. Sessões curtas seguem o algoritmo original; sessões longas usam um estágio novo, `windowed`, com um estado recortado por janela de chamadas.
- **Segurança.** Na dúvida, mantém. Qualquer falha cai no resumo padrão do Claude Code.

## Mapa

### `description/` — o que é e como funciona

| Documento | Conteúdo |
|---|---|
| [01 — Visão geral](description/01-visao-geral.md) | Problema, objetivo, o que muda em relação ao original, critérios de sucesso |
| [02 — Requisitos](description/02-requisitos.md) | RF/RNF numerados, fora de escopo |
| [03 — Algoritmo](description/03-algoritmo.md) | Pareamento, estado, orçamentos, estágios 1–8, decisão, reconstrução, invariantes |
| [04 — Arquitetura](description/04-arquitetura.md) | Componentes, layout do repositório, sequência, ciclo de vida, concorrência |
| [05 — API do daemon](description/05-api-daemon.md) | Contrato HTTP, exemplos, códigos de erro, CLI |
| [06 — Plugin do Claude Code](description/06-plugin-claude-code.md) | Hook TS, `userConfig`, fallback, testes |
| [07 — Configuração](description/07-configuracao.md) | Flags, variáveis de ambiente, opções do algoritmo |
| [08 — Provisionamento](description/08-provisionamento.md) | `make provision`, Docker, requisitos de máquina |
| [09 — Testes e avaliação](description/09-testes-e-avaliacao.md) | Camadas de teste, golden files, fuzz, `--report`, calibração |
| [10 — Riscos](description/10-riscos.md) | Riscos, mitigações e gatilhos de revisão |
| [Glossário](description/glossario.md) | Termos |

### `adrs/` — por que foi decidido assim

[Índice das ADRs](adrs/README.md) — 11 decisões, da linguagem à política de privacidade.

### `references/` — material de origem

- [fast-jev-compaction](references/fast-jev-compaction.md) — análise do original (commit `e3f262a`): arquivos, constantes, textos fixos, algoritmo.
- [Laya e laya-go](references/laya-e-laya-go.md) — modelo, checkpoints, API do binding escolhido, alternativas descartadas.

### `docs/plans/` — design e execução

- [Design validado](docs/plans/2026-10-08-fast-laya-compaction-design.md)
- [Plano de implementação](docs/plans/2026-10-08-fast-laya-compaction-implementation.md) — fases 0 a 6, com portões de saída.

## Créditos

- Algoritmo original: [`tamaratran/fast-jev-compaction`](https://github.com/tamaratran/fast-jev-compaction) (MIT).
- Modelo: [Laya, ConvAI Innovations](https://huggingface.co/convaiinnovations/laya) (Apache 2.0).
- Binding: [`metalagman/laya-go`](https://pkg.go.dev/github.com/metalagman/laya-go) (MIT).
