# Referência: Laya e bindings Go

Levantamento feito em 2026-10-08. Itens marcados **(não verificado)** precisam ser confirmados na Fase 0 do plano.

## Modelo Laya (ConvAI Innovations)

- Hugging Face: <https://huggingface.co/convaiinnovations/laya>
- Tipo: modelo de decisão "System 1", **não autoregressivo**. Não gera texto: responde perguntas tipadas sobre um estado com probabilidades calibradas, todas em um único forward pass (~33 ms).
- Tipos de pergunta: `noul` (binária, P(true)), `choice` (uma de N opções), `score` (níveis ordenados).
- Mecanismo: cada opção é pontuada no seu próprio token `[MASK]` ("option markers"), o que permite definir respostas na requisição, sem re-treino.
- Licença: **Apache 2.0**, pesos abertos, uso on-premise.
- Pacote Python: `pip install laya` (Python 3.10+); ONNX opcional via `laya[onnx]`.

| Checkpoint | Base | Parâmetros | Contexto | Tamanho |
|---|---|---:|---:|---:|
| english | ModernBERT-large + cabeça de decisão | 421M | 512 tokens | ~808 MB |
| multilingual | mmBERT-base (22 camadas, vocab 256k) | 322M | 1.024 tokens (extensível a 8.192) | ~647 MB |
| typed-decisions | ModernBERT-large ajustado | — | 1.024 tokens | — |

Relação com o Jev: ambos expõem a mesma abstração (estado + perguntas `noul` → probabilidades). O contrato de perguntas do `fast-jev-compaction` (`{type: 'noul', instructions, criteria: {true, false}}`) mapeia diretamente para `NoulQuestion` do Laya. A diferença decisiva é a janela: o Jev aceitava ~30k tokens por requisição; o Laya multilíngue, 1.024.

## `github.com/metalagman/laya-go` — **escolhido** (ADR 0004)

- pkg.go.dev: <https://pkg.go.dev/github.com/metalagman/laya-go> · Versão: **v0.2.6** (2026-09-24) · Licença: MIT · pré-v1.0 (API pode mudar).
- Inferência **dentro do processo**. Builds comuns retornam `ErrNativeUnavailable`; a inferência real exige build `cgo` com tag **`laya_native`**.

### API relevante

```go
func NewRuntime(ctx context.Context, opts RuntimeOptions) (*Runtime, error) // RuntimeOptions{} sem campos
func (r *Runtime) OpenModelDir(ctx context.Context, dir string, opts ModelOptions) (*Model, error)
func (r *Runtime) OpenModelFS(ctx context.Context, fsys fs.FS, opts FSModelOptions) (*Model, error)

type ModelOptions struct {
    QueueCapacity   int              // quantos podem esperar em FIFO
    MaxQueueWait    time.Duration
    InputTokenLimit int              // sobrescreve o limite do bundle
    Truncation      TruncationPolicy // padrão RejectOverflow
}

func (m *Model) Predict(ctx context.Context, state State, questions []Question) (Prediction, error)
func (m *Model) Noul(ctx context.Context, state State, q NoulQuestion) (NoulResult, error)
func (m *Model) Close(ctx context.Context) error

func TextState(text string) State
func JSONState(data []byte) (State, error) // valida e compacta o JSON

func NewNoulQuestion(id QuestionID, instructions, falseDescription, trueDescription string) NoulQuestion
// NoulResult: QuestionID(), TrueProbability() float64, ActionProbability() float64
// Prediction: resultados na ordem das perguntas + Usage{InputTokens, OutputTokens}

var ErrNativeUnavailable, ErrInputTooLong, ErrQueueFull, ErrInvalidBundle error
// errors.As(err, *InputTooLongError) para detalhes
```

### Restrições

- `Runtime` e `Model` são seguros para uso concorrente, mas **apenas uma chamada fica ativa por vez**; as demais esperam em fila FIFO limitada por `QueueCapacity`.
- Bundle v1 aceita apenas: preprocessing v1, perfil **`laya-multilingual-v1`**, opset 18, **FP32**, **`CPUExecutionProvider`**. Configurações desconhecidas falham.
- Bundle = `manifest.json` + `model.onnx` (+ external data) + arquivos do tokenizer + `rl_agent_config.json`. Cada artefato tem tamanho exato e SHA-256; `BundleID = "sha256:" + sha256(manifest.json)`.
- Entradas do grafo: `input_ids`, `attention_mask`, `marker_pos`, `marker_mask`, `qtype`; saídas: logits e action logits.
- Execução nativa usa três entradas locais: `LAYA_ONNXRUNTIME_LIBRARY`, `LAYA_TOKENIZERS_LIBRARY`, `LAYA_BUNDLE_DIR`; exige `ORT_DISABLE_TELEMETRY=1` antes de criar o runtime.
- ONNX Runtime **1.29.0** (medição do autor). Qualificado só em **linux/amd64**; WSL2 e darwin aparecem em medições, sem garantia.
- Orçamento medido: 4 CPUs lógicas, 2 GiB de RAM, ~1,23 GiB de disco para o bundle.
- A biblioteca **não baixa nada**: não autentica, não resolve revisões, não gerencia cache.
- Exportação oficial (`docs/official-export-v1.md`): fonte `convaiinnovations/laya-multilingual@052592a`; tarefas `task reference:verify-source`, `task reference:verify-sdk`, `task bundle:export`, `task bundle:verify`. Versões de Python/dependências e origem da `libtokenizers` **(não verificado)**.

## Alternativas avaliadas e descartadas

| Projeto | O que é | Por que não |
|---|---|---|
| `github.com/lengoman/laya-go` v0.2.0 (Apache 2.0) | Cliente Go puro (sem cgo) para um servidor Laya Python em `$LAYA_URL` / `127.0.0.1:8600`, ou sidecar Python embutido. API rica (`Ask`, `Batch`, `Route`, presets). | Exige Python em tempo de execução. Opção considerada como alternativa na ADR 0004. |
| `github.com/aayushch/laya` | App desktop de notificações (Tauri + Python + Ollama). | Homônimo sem relação com o modelo de decisão. |
| `github.com/iheeb1/lx/integrations/laya` v0.4.0 | Apenas exporta `Script string` com um script de daemon embutido. | Não é um binding; nada a reutilizar. |
