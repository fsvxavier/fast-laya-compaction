# 0002 — Go como linguagem principal

- **Status:** Aceita
- **Data:** 2026-10-08
- **Relacionadas:** ADR 0004, ADR 0008

## Contexto

O original é uma biblioteca TypeScript que roda dentro do Claude Code como function hook. Este projeto precisa executar um modelo localmente, o que traz um processo de longa duração, bibliotecas nativas (ONNX Runtime) e gerenciamento de memória de ~1,2 GiB de pesos. O mantenedor trabalha em Go.

## Decisão

Algoritmo, daemon, CLI e adaptador do modelo serão escritos em Go (1.25+), com cgo apenas no adaptador do modelo. A única parte em TypeScript é o hook do plugin, porque function hooks do Claude Code só aceitam módulos TS/JS.

## Alternativas consideradas

- **Manter tudo em TypeScript e chamar o Laya via Python** — exigiria Node + Python em tempo de execução; foge do pedido.
- **Rust** — bom para FFI, mas fora da stack do mantenedor e sem binding Laya conhecido.

## Consequências

- Binário único com o modelo carregado uma vez.
- Há binding Go para o Laya (`metalagman/laya-go`), ver ADR 0004.
- O algoritmo existe agora em duas linguagens (original em TS, porte em Go); a fidelidade é verificada por testes portados e golden files gerados a partir do original.
