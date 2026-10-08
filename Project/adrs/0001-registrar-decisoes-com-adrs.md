# 0001 — Registrar decisões com ADRs

- **Status:** Aceita
- **Data:** 2026-10-08

## Contexto

O projeto é um porte com várias decisões que divergem do original por causa de restrições técnicas (janela do modelo, execução local, linguagem). Quem mantiver o código depois precisa saber o que foi decidido e por quê, especialmente nos pontos em que o porte deixa de ser fiel.

## Decisão

Registraremos decisões de arquitetura em `Project/adrs/`, um arquivo por decisão, numerado sequencialmente, no formato de Michael Nygard (Contexto, Decisão, Alternativas consideradas, Consequências), em português. O índice fica em `Project/adrs/README.md`.

## Alternativas consideradas

- **Só o documento de design** — mistura decisões com descrição e não registra a evolução.
- **MADR** — mais campos do que o projeto precisa.

## Consequências

- Cada divergência do original tem uma ADR para apontar em revisões de código.
- Mudar uma decisão exige uma ADR nova que substitui a anterior.
