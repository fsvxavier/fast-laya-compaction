# 0007 — Estimativa heurística de tokens com calibração

- **Status:** Aceita
- **Data:** 2026-10-08
- **Relacionadas:** ADR 0005, ADR 0006, ADR 0010

## Contexto

O original estima tokens sem tokenizer (palavras, dígitos e símbolos com pesos), calibrado contra o uso reportado pelo Jev: fica 2–18% acima do real. O tokenizer do Laya multilíngue (mmBERT, vocabulário de 256k) é outro, e a calibração não se transfere.

Com orçamento de 1.024 tokens, errar para baixo custa caro: o `laya-go` rejeita a entrada com `ErrInputTooLong` (política `RejectOverflow`). Errar para cima desperdiça a já pequena janela.

O `laya-go` reporta `Usage.InputTokens` em cada `Prediction`, mas não expõe (pelo que foi verificado) uma função de contagem prévia.

## Decisão

1. Portaremos `estimateTokens` sem alteração e o multiplicaremos por `tokenScale` (padrão 1,0, configurável).
2. O custo de uma pergunta será `EstimateTokens(instructions) + questionOverheadTokens` (padrão 8).
3. Toda inferência registra estimado × reportado (`inputTokensEstimated`, `inputTokensReported`).
4. Na Fase 3, `fast-laya compact --report` sobre o corpus define os padrões de `tokenScale` e `questionOverheadTokens` de modo que < 1% das inferências gere `ErrInputTooLong`.
5. `ErrInputTooLong` é tratado com retries a ×0,85 do orçamento (ADR 0010).
6. Se o `laya-go` passar a expor contagem exata de tokens, uma ADR nova pode trocar a estimativa pela contagem.

## Alternativas consideradas

- **Tokenizer exato em Go** (carregar `tokenizer.json` do bundle com uma biblioteca de tokenizers) — exato, mas duplica o tokenizer que o `laya-go` já carrega via `libtokenizers`, e a montagem do estado chama a estimativa centenas de vezes por estágio.
- **`Truncation` que corta o excesso** em vez de rejeitar — o modelo perderia silenciosamente o fim do estado ou as perguntas; inaceitável.

## Consequências

- Mesmo código de estimativa do original; diferença isolada em dois parâmetros.
- A calibração depende de um corpus representativo; corpus ruim gera `unfit` demais ou retries demais.
