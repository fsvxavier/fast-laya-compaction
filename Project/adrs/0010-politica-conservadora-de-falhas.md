# 0010 — Política conservadora de falhas

- **Status:** Aceita
- **Data:** 2026-10-08
- **Relacionadas:** ADR 0006, ADR 0007, ADR 0008

## Contexto

Apagar contexto que ainda importa é o pior resultado possível da compactação: o assistente perde informação sem saber. Manter demais só reduz menos. O original já é conservador em dois pontos: chamada sem resposta do modelo recebe `{keepCall: 1, keepResult: 1}`, e qualquer erro leva ao resumo padrão.

O porte acrescenta fontes novas de falha parcial: janela que não cabe e `ErrInputTooLong` por estimativa errada.

## Decisão

Em toda dúvida, **manter**. Concretamente:

| Situação | Comportamento |
|---|---|
| Parte obrigatória da janela não cabe nem com uma chamada e inputs de 60 chars | chamada `keep`, motivo `unfit`, sem pergunta |
| `ErrInputTooLong` | refaz como janela com orçamento ×0,85, até 2 vezes; depois `keep`/`unfit` para as chamadas afetadas |
| Resposta malformada do modelo (pergunta sem resposta, probabilidade não finita ou fora de [0,1]) | aborta a compactação inteira → `500` → fallback no hook |
| Erro de runtime não recuperável | aborta → `500` → fallback |
| Daemon ocupado, fora do ar, carregando, timeout | fallback no hook |
| Redução abaixo de `minReductionRatio` | fallback no hook (como no original) |

Nenhuma chamada é removida ou truncada sem probabilidades vindas do modelo para ela.

## Alternativas consideradas

- **Abortar a compactação ao primeiro `ErrInputTooLong`** — uma estimativa ruim numa chamada derrubaria toda a compactação.
- **Truncamento automático do `laya-go`** — o modelo decidiria sem ver parte do estado ou das perguntas, silenciosamente.
- **Tratar `unfit` como `drop_result`** — apagaria sem consulta.

## Consequências

- Falhas parciais reduzem menos, nunca apagam indevidamente.
- `unfit` alto é sinal de calibração ruim (ADR 0007) e aparece nas estatísticas e no toast.
- O usuário nunca fica sem compactação: o pior caso é o resumo padrão do Claude Code.
