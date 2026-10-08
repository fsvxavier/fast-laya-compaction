# 0003 — Laya no lugar do Jev

- **Status:** Aceita
- **Data:** 2026-10-08
- **Relacionadas:** ADR 0004, ADR 0006

## Contexto

O `fast-jev-compaction` usa o Jev, modelo de decisão acessado pela API System One da TypeSafe. Cada compactação custa dinheiro, exige `TYPESAFE_API_KEY` e envia o transcript inteiro (código, caminhos, possivelmente segredos) para fora da máquina.

O Laya (ConvAI Innovations, <https://huggingface.co/convaiinnovations/laya>) é um modelo de decisão de pesos abertos, Apache 2.0, com a mesma abstração: um estado e perguntas tipadas (`noul`, `choice`, `score`) respondidas com probabilidades calibradas em um único forward pass. As perguntas `noul` do original (`{type: 'noul', instructions, criteria: {true, false}}`) mapeiam diretamente para `NoulQuestion(id, instructions, falseDescription, trueDescription)`.

Diferença crítica: o Jev aceitava ~30.000 tokens por requisição; o checkpoint multilíngue do Laya aceita 1.024 (o inglês, 512).

## Decisão

Usaremos o Laya, checkpoint **`laya-multilingual-v1`** (mmBERT-base, 322M parâmetros, 1.024 tokens), executado localmente em CPU. Os textos das perguntas e do contexto do estado serão os mesmos do original, em inglês.

## Alternativas consideradas

- **Checkpoint `english`** (ModernBERT-large, 421M) — janela de 512 tokens, ainda menor; e o bundle v1 do `laya-go` não o suporta.
- **Checkpoint `typed-decisions`** — 1.024 tokens, mas também não suportado pelo bundle v1.
- **LLM local generativo (Ollama/llama.cpp) respondendo sim/não** — janelas maiores, porém sem probabilidades calibradas, muito mais lento por decisão e sujeito a respostas fora do formato. Mudaria a natureza do algoritmo.
- **Manter o Jev** — contraria o objetivo (local e sem custo).

## Consequências

- Custo zero por uso; nenhum dado sai da máquina.
- A janela de 1.024 tokens obriga a um estágio novo de montagem do estado (ADR 0006); o comportamento deixa de ser idêntico ao original em sessões longas.
- A qualidade das decisões não é comparável diretamente com a do Jev; a avaliação usa corpus próprio ([09](../description/09-testes-e-avaliacao.md)).
- Ganho colateral: o checkpoint multilíngue entende sessões em português.
