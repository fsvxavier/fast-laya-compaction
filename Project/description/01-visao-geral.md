# 01 — Visão geral

## Problema

Sessões longas de agentes de código enchem a janela de contexto, principalmente com saídas de ferramentas: arquivos lidos, logs, resultados de busca. O auto-compact padrão do Claude Code resolve resumindo turnos antigos, e o resumo perde exatamente o que mais importa depois: caminhos de arquivo, mensagens de erro exatas, restrições combinadas com o usuário.

O [`fast-jev-compaction`](../references/fast-jev-compaction.md) propôs outra abordagem: **nunca reescrever nada, só apagar** tool calls e tool results que um modelo de decisão considera desnecessários. Funciona bem, mas depende do Jev, um modelo pago acessado pela API da TypeSafe. Toda compactação custa dinheiro, exige chave de API e envia o transcript inteiro para um serviço externo.

## Objetivo

`fast-laya-compaction` reproduz o comportamento do `fast-jev-compaction` com três diferenças:

1. **Laya no lugar do Jev.** O [Laya](../references/laya-e-laya-go.md) é um modelo de decisão de pesos abertos (Apache 2.0) com a mesma abstração: estado + perguntas `noul` → probabilidades calibradas.
2. **Tudo local.** Inferência em CPU, dentro do processo, via ONNX Runtime. Sem chave, sem custo por chamada, sem o transcript sair da máquina.
3. **Go no lugar de TypeScript.** O algoritmo, o daemon e a CLI são Go. Só a casca do plugin do Claude Code continua em TypeScript, porque function hooks exigem módulos TS/JS.

## O que se mantém do original

- A garantia central: texto de usuário e assistente nunca é alterado; nenhuma chamada fica sem resultado; mensagens intactas voltam idênticas.
- As duas perguntas por tool call (manter a chamada? manter o resultado inteiro?) e os mesmos textos.
- A regra de decisão por `keepThreshold` e o truncamento do resultado a `truncateHeadChars` caracteres com nota.
- Os estágios de redução do estado (`fitState`) e a divisão em lotes (`batchCalls`).
- O fallback para o resumo padrão do Claude Code em qualquer erro ou quando a redução fica abaixo de `minReductionRatio`.
- O gatilho por `compactAtPercent` em `turn.complete`.

## O que muda

| Aspecto | fast-jev-compaction | fast-laya-compaction |
|---|---|---|
| Modelo | Jev (`jev-latest`), API paga | Laya `laya-multilingual-v1`, local |
| Janela por requisição | ~30.000 tokens | 1.024 tokens |
| Execução | HTTP para `api.typesafe.ai` | `laya-go` + ONNX Runtime em CPU, dentro do daemon |
| Paralelismo | Lotes em paralelo | Lotes em sequência (uma inferência ativa por vez) |
| Estado que não cabe | Erro → fallback | Estágio novo `windowed`: um estado recortado por lote |
| Integração | Hook TS chama a API | Hook TS chama o daemon local `fast-laya serve` |
| Configuração sensível | `TYPESAFE_API_KEY` | Nenhuma; caminhos de bibliotecas e bundle |

A janela de 1.024 tokens é a maior consequência técnica da troca e motiva o [estágio de janela por lote](03-algoritmo.md#8-estágio-8-windowed-novo) ([ADR 0006](../adrs/0006-estagio-de-janela-por-lote.md)).

## Critérios de sucesso

- Em uma sessão real com dezenas de tool calls, a compactação termina sem fallback e reduz o transcript em pelo menos 25% (o `minReductionRatio` padrão).
- Uma compactação termina dentro de `timeoutMs` do hook, ou mantém o que não deu tempo de avaliar (`unfit`); a meta numérica de tempo depende das decisões pendentes da Fase 0 (medido: ~0,3–2,3 s por pergunta).
- Nenhum byte do transcript sai da máquina; nenhuma chave de API é necessária.
- Os casos de teste do original, portados, passam com o `fakelaya`.

## Público

Quem usa Claude Code localmente (Linux ou WSL2, x86-64) e quer compactação sem resumo e sem custo. Também quem quer usar o algoritmo como biblioteca Go ou CLI para processar transcripts salvos.
