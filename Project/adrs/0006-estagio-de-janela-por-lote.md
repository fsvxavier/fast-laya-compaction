# 0006 — Estágio de janela por lote quando o estado global não cabe

- **Status:** Aceita
- **Data:** 2026-10-08
- **Relacionadas:** ADR 0003, ADR 0005, ADR 0010

## Contexto

O original monta **um** estado com toda a conversa e o reenvia em cada lote de perguntas. O estado é reduzido em 7 estágios até caber em 25.000 tokens; se não couber, lança erro e o hook cai no resumo padrão.

Com o Laya, a requisição inteira (estado + perguntas) precisa caber em 1.024 tokens. Mesmo no estágio 7 (`old calls merged`, uma linha por chamada), uma sessão com algumas dezenas de chamadas passa desse limite. Cada pergunta `noul` também consome tokens de marcador e instrução. Um porte puro faria o projeto viver em fallback.

Ao mesmo tempo, a decisão sobre uma chamada depende principalmente de: (a) o objetivo da sessão, (b) a própria chamada, (c) o que veio depois e toca o mesmo recurso (o arquivo foi relido? editado?), (d) o que está acontecendo agora (mensagens recentes).

## Decisão

1. Orçamentos padrão derivados do limite do modelo `L`: `maxRequestTokens = L − 64` (960) e `maxStateTokens ≈ 0,83 × maxRequestTokens` (800), mantendo a proporção do original.
2. Os estágios 1–7 rodam como no original. Se o estado couber, os lotes compartilham esse estado.
3. Onde o original lançaria erro, entra o **estágio 8 `windowed`**: as candidatas são agrupadas em janelas de `windowCalls` (padrão 2) e cada janela recebe um estado próprio, com parte obrigatória (context, goal truncado a 25% do orçamento, as chamadas da janela) e parte opcional preenchida por prioridade:
   1. chamadas posteriores que tocam o mesmo recurso;
   2. mensagens fixadas recentes;
   3. demais chamadas posteriores, da mais próxima para a mais distante;
   4. chamadas anteriores.
4. No estágio 8, o `context` recebe uma frase extra (`WindowedStateNote`) avisando que o `history` é um recorte, já que o texto original afirma que é a conversa inteira.
5. Se a parte obrigatória não couber: inputs a 60 caracteres → janela de uma chamada → `unfit` (ADR 0010).

Especificação completa em [03 — Algoritmo, §8](../description/03-algoritmo.md#8-estágio-8-windowed-novo).

## Alternativas consideradas

- **Porte 100% fiel** — fallback na maioria das sessões reais.
- **Só estado por chamada** — abandona o estado global mesmo quando ele cabe; perde paridade com o original e faz uma inferência por chamada.
- **Aumentar `InputTokenLimit` para até 8.192** (o mmBERT suporta) e manter o estado global — fora do contrato medido pelo `laya-go`; latência e qualidade não verificadas. Fica como **experimento**: a flag `--input-token-limit` existe e os orçamentos acompanham, mas o padrão é o limite do bundle.

## Consequências

- Sessões curtas: comportamento idêntico ao original.
- Sessões longas: compactação local sem fallback, com visão parcial da conversa por decisão. A prioridade "mesmo recurso" cobre o caso mais comum (resultado de leitura antiga obsoleto).
- Mais inferências: ~`candidatas / windowCalls`. Com ~33 ms cada (número do card do Laya, não verificado em CPU; medido na Fase 0), 100 candidatas ≈ 50 inferências ≈ 2 s.
- Risco de decisões piores por falta de contexto (R2), mitigado pela política conservadora e pela avaliação.
- Revisar esta ADR se o experimento com `InputTokenLimit` maior mostrar qualidade e latência aceitáveis.
