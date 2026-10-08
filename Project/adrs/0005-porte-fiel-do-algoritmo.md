# 0005 — Porte fiel do algoritmo do `fast-jev-compaction`

- **Status:** Aceita
- **Data:** 2026-10-08
- **Relacionadas:** ADR 0006, ADR 0007, ADR 0010

## Contexto

O algoritmo do original (`src/state.ts`, `src/compact.ts`, commit `e3f262a`) já tem decisões finas calibradas na prática: textos fixos das perguntas, ordem dos estágios de redução, limites (1000/200/60, 400/150), regra de decisão, formato da nota de truncamento. Reinventar qualquer uma delas sem dado novo tornaria impossível comparar os dois projetos.

## Decisão

Portaremos o algoritmo **linha a linha** para Go, preservando:

- `STATE_CONTEXT` e os textos das duas perguntas `noul`, em inglês;
- `INPUT_CHARS = [1000, 200, 60]`, `TEXT_HEAD = 400`, `TEXT_TAIL = 150`, `REQUEST_OVERHEAD_TOKENS = 20`;
- os estágios 1–7 do `fitState`, na mesma ordem e com os mesmos nomes;
- `batchCalls`, `decideCall`, `applyDecisions` e as estatísticas;
- os padrões `keepThreshold = 0.5`, `preserveRecentMessages = 6`, `truncateHeadChars = 300`, `compactAtPercent = 60`, `minReductionRatio = 0.25`.

Divergências permitidas, cada uma com ADR própria:

| Divergência | ADR |
|---|---|
| Padrões de `maxStateTokens`/`maxRequestTokens` (800/960 em vez de 25k/30k) | 0006 |
| Estágio 8 `windowed` em vez de `throw` | 0006 |
| Frase extra `WindowedStateNote` no `context`, só no estágio 8 (o histórico é um recorte) | 0006 |
| `tokenScale` e custo de pergunta do Laya | 0007 |
| Motivo `unfit` e retries por `ErrInputTooLong` | 0010 |
| Lotes em sequência | 0004 |
| Metadados de origem (`source`, `unchanged`, `sourceIndex`) | 0008 |
| Nota de truncamento com `fast-laya-compaction` no lugar de `fast-jev-compaction` | esta |

Detalhes de tradução TS → Go a respeitar:

- **Comprimentos em caracteres.** `String.length` em JS conta unidades UTF-16. Em Go, usar contagem de runas (`utf8.RuneCountInString`) e cortar por runa, nunca por byte, para não partir caracteres multibyte. Diferença residual só em caracteres fora do BMP (emoji), aceita.
- **`JSON.stringify` vs `encoding/json`.** Go escapa `<`, `>`, `&` por padrão; usar `json.Encoder` com `SetEscapeHTML(false)` para que tamanhos e estimativas batam com o original.
- **Ordem de chaves de `input`.** JS preserva a ordem de inserção; `map[string]any` em Go ordena alfabeticamente ao serializar. Guardar `input` como `json.RawMessage`, normalizado com `json.Compact` (preserva a ordem). Onde o original itera `Object.entries(input)` (`compactCall`), percorrer as chaves na ordem do documento com `json.Decoder.Token()`.

## Alternativas consideradas

- **Redesenhar o algoritmo para a janela pequena** (só estado por chamada) — mais simples, mas sem paridade com o original em sessões curtas e sem base de comparação.
- **Porte 100% fiel, inclusive o `throw`** — com 1.024 tokens, quase toda sessão real cairia no fallback.

## Consequências

- Em sessões cujo estado cabe em ~800 tokens, o comportamento é o do original.
- Os testes do original viram a especificação executável do porte; golden files gerados pelo original detectam desvios.
- Correções futuras no original podem ser trazidas com diff direto.
