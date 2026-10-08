# 0004 — Inferência dentro do processo com `metalagman/laya-go` e ONNX Runtime

- **Status:** Aceita (condicionada à Fase 0 do plano)
- **Data:** 2026-10-08
- **Relacionadas:** ADR 0002, ADR 0003, ADR 0009

## Contexto

Há duas formas de executar o Laya a partir de Go ([referência](../references/laya-e-laya-go.md)):

1. **`github.com/lengoman/laya-go` v0.2.0** — cliente Go puro (sem cgo) para o servidor oficial Python do Laya (`pip install laya`, `127.0.0.1:8600`) ou um sidecar Python embutido. API madura (`Ask`, `Batch`, `Route`).
2. **`github.com/metalagman/laya-go` v0.2.6** — inferência dentro do processo via ONNX Runtime, com build cgo `laya_native`. Pré-v1; o caminho nativo é descrito pelo autor como "reviewed native-runtime candidate", qualificado em linux/amd64 com ONNX Runtime 1.29.0, FP32, CPU.

Fatos do `metalagman/laya-go` que moldam o desenho:

- só o perfil `laya-multilingual-v1` é aceito pelo bundle v1;
- uma inferência ativa por vez por `Model`, fila FIFO limitada por `QueueCapacity`;
- `RejectOverflow` por padrão: entrada acima do limite → `ErrInputTooLong`;
- precisa de três entradas locais (`LAYA_ONNXRUNTIME_LIBRARY`, `LAYA_TOKENIZERS_LIBRARY`, `LAYA_BUNDLE_DIR`) e `ORT_DISABLE_TELEMETRY=1`;
- não baixa nada: o bundle é exportado offline por um runbook com Python.

## Decisão

Usaremos o **`metalagman/laya-go`** com build tag `laya_native` e cgo, em versão fixada no `go.mod`. Todo uso fica no pacote `internal/layaasker`, atrás da interface `compaction.Asker`. Um arquivo stub (`!laya_native`) compila sem cgo e devolve `ErrNativeUnavailable`.

A decisão é **condicionada à Fase 0** do plano: exportar o bundle e executar um `Predict` real no WSL2. Se a Fase 0 falhar, esta ADR será substituída por uma que adote o `lengoman/laya-go` com servidor Python local — a interface `Asker` torna essa troca local ao adaptador.

## Alternativas consideradas

- **`lengoman/laya-go` + servidor Python** — mais maduro e sem cgo, mas exige Python em tempo de execução e um segundo processo. Mantido como plano B.
- **Interface com os dois adaptadores desde o início** — mais código para manter antes de saber se o caminho nativo funciona; YAGNI.
- **Implementar a inferência do zero com `onnxruntime_go`** — refazer tokenização, marcadores de opção e pós-processamento que o `laya-go` já faz.

## Consequências

- Um binário, sem Python em tempo de execução. Python só no provisionamento.
- Build exige cgo e `gcc`; os testes do núcleo não (stub).
- Lotes são executados em sequência; o paralelismo do original (`Promise.all`) não se aplica.
- Dependemos de uma API pré-v1 e de um caminho nativo "candidate": acompanhar releases (risco R4) e fixar versões.
- Suporte limitado a linux/amd64 (inclui WSL2).
