# 0009 — Provisionamento por script local, com Docker como alternativa

- **Status:** Aceita
- **Data:** 2026-10-08
- **Relacionadas:** ADR 0004

## Contexto

O caminho nativo do `laya-go` precisa de três artefatos locais que a biblioteca, por desenho, não baixa: `libonnxruntime` 1.29.0, `libtokenizers` e um bundle exportado do checkpoint `convaiinnovations/laya-multilingual@052592a`. A exportação usa Python e `go-task` uma única vez. O ambiente principal do mantenedor é WSL2.

## Decisão

1. **Principal: `make provision`** (`scripts/provision.sh`), idempotente, que baixa as bibliotecas com SHA-256 verificado, exporta e verifica o bundle num venv temporário, instala tudo em `~/.local/share/fast-laya/` e gera um `.env` com as três variáveis.
2. **Alternativa: `Dockerfile` multi-stage** (exporter → libs → builder → runtime) que produz uma imagem com binário, bibliotecas e bundle, a rodar com `-p 127.0.0.1:8610:8610`.
3. **Fonte única de versões**: `scripts/versions.env` (versões, URLs e SHA-256), lida pelo script e pelo Dockerfile.
4. **`fast-laya doctor`**: verifica as três entradas, valida o bundle, carrega o modelo e faz uma inferência de teste.

Detalhes em [08 — Provisionamento](../description/08-provisionamento.md).

## Alternativas consideradas

- **Só script** — sem opção limpa para quem não quer Python e cgo no host.
- **Só Docker** — imagem de ~1,6 GB e ciclo de desenvolvimento mais lento no dia a dia.
- **Instruções manuais no README** — pouco reprodutível; cada passo é uma chance de versão errada.

## Consequências

- Python é dependência só de provisionamento, nunca de execução.
- Dois caminhos para manter; o `versions.env` compartilhado reduz a divergência.
- O script depende do runbook de exportação do `laya-go`, pouco documentado (risco R1): a Fase 0 o valida e ajusta.
- Redistribuir a imagem com o bundle exige manter o aviso Apache 2.0 do modelo (risco R10).
