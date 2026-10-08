# Fase 0 — Resultado do spike de viabilidade

- **Data:** 2026-10-08
- **Ambiente:** WSL2 (kernel 6.18), linux/amd64, 8 vCPUs, 15 GiB RAM, Go 1.27.1, gcc 13.3, `uv` (venv gerenciado com Python 3.13)
- **Código:** [`spike/phase0/`](../../../spike/phase0/main.go) · versões fixadas: [`scripts/versions.env`](../../../scripts/versions.env)

## Veredito

**O caminho nativo da [ADR 0004](../../adrs/0004-inferencia-in-process-com-laya-go-onnx.md) funciona.** O bundle foi exportado, verificado, carregado e respondeu perguntas `noul` reais dentro do processo Go, no WSL2. O plano B (`lengoman/laya-go` + servidor Python) **não** é necessário.

**Mas a latência é de 10 a 30 vezes pior do que a documentação assumia** (ver "Latência"). Isso obriga a decisões novas sobre o orçamento de tempo, listadas no fim.

## O que foi executado

| Passo | Resultado |
|---|---|
| Download de ONNX Runtime 1.29.0, `libtokenizers`, checkpoint (7 arquivos, 644 MB de pesos) e SDK no commit fixado | Todos os SHA-256 conferiram, inclusive o do arquivo do SDK |
| `uv sync --frozen` do exportador | OK (venv de ~1 GB com torch CPU) |
| Exportação do bundle (`laya_export.cli … export`) | OK. `bundle_id = sha256:ca06dcb1166aad3287d32ed5423d4551d36af246acc0e0421780c141b9377f23`, 9 arquivos, grafo opset 18 com 1.486 nós, 321.908.995 parâmetros |
| Paridade com a referência oficial (verificada pelo exportador) | `logits` máx. abs. 3,1e-6; `act_logits` 6,1e-4 |
| Build `CGO_ENABLED=1 go build -tags laya_native` | OK, com `CGO_LDFLAGS=-L<dir de libtokenizers.a>` |
| `NewRuntime` → `OpenModelDir` → `Predict` | OK |

Tamanhos: bundle em disco **1,29 GiB** (`model.onnx.data`); pico de RAM do processo **1,96 GB** (de acordo com os 2 GiB documentados); carga do modelo **~5 s**.

## Correções em relação ao que a documentação do projeto assumia

1. **`libtokenizers` é uma biblioteca estática (`libtokenizers.a`)** do release `v1.27.0` de `daulet/tokenizers`, ligada em tempo de build com `CGO_LDFLAGS=-L…`. Não é `.so`. `LAYA_TOKENIZERS_LIBRARY` aponta para o `.a`.
2. **A exportação usa `uv`** (lockfile `tools/export/uv.lock`, `requires-python >= 3.12`), não `pip`+`venv`. O `task` (go-task) é dispensável: os comandos do `Taskfile.yml` podem ser executados diretamente. A exportação precisa também do **SDK** `NandhaKishorM/laya@573e5b6…` como entrada, além do checkpoint.
3. **Go 1.26.6 ou superior** é exigido pelo `laya-go` (`go.mod` dele declara `go 1.26.6`).
4. **Cada pergunta é uma sequência separada.** `prepareInferenceBatch` monta, para cada pergunta, `[CLS] "noul question: <instruções>" [SEP] <mask> "false: …" <mask> "true: …" [SEP] <estado> [SEP]` e junta as sequências num batch. Consequências:
   - o **limite de 1.024 tokens vale por sequência**: estado + cabeça da pergunta + opções + 4 tokens especiais. Perguntas **não** disputam o mesmo orçamento (não se somam);
   - o **estado é codificado de novo para cada pergunta**: N perguntas custam N vezes o mesmo estado;
   - as descrições padrão das opções (`false: no, the statement does not hold` / `true: yes, the statement holds`) ocupam ~25 tokens por pergunta; podem ser encurtadas passando `falseDescription`/`trueDescription`.
5. **`ModelOptions.InputTokenLimit` só reduz o limite**: o código usa `opts.InputTokenLimit < manifest.MaxLen`. O bundle exportado declara `max_len = 1024`; subir o limite exigiria outra exportação. O experimento "limite até 8.192" da ADR 0006 não é executável com este bundle.
6. **`InputTooLongError` traz a contagem real**: `InputTokens` (observado) e `Limit`. Serve para recalibrar na hora, melhor que o corte cego de ×0,85.
7. **`QueueCapacity: 0` rejeita** quem não conseguir começar imediatamente (`ErrQueueFull`); o desenho de uma compactação por vez no daemon continua válido.
8. O `ORT_DISABLE_TELEMETRY=1` precisa estar no ambiente **antes** de `NewRuntime`; o spike usa `os.Setenv` no início de `main`.

## Latência (mediana de 5 execuções após 1 de aquecimento, 8 vCPUs)

`inputTokens` é a soma sobre as perguntas do batch (`Usage.InputTokens`). O estado sintético usa o formato de uma linha por chamada do projeto.

| Entradas no estado | Tokens por pergunta | ms por pergunta |
|---:|---:|---:|
| 1 | 193 | 315 |
| 2 | 258 | 431 |
| 4 | 388 | 702 |
| 6 | 518 | 1.002 |
| 8 | 648 | 1.393 |
| 10 | 781 | 1.823 |
| 12 | 917 | 2.295 |

- Custo **linear no número de perguntas**: 1, 2, 4 e 8 perguntas custam ~1×, 2×, 4× e 8× (batch não acelera).
- **~1,8 ms por token** até ~650 tokens, subindo para ~2,5 ms/token perto de 900.
- A CPU usada foi em média ~3,8 núcleos de 8 (`user` 1.470 s em 385 s); o `laya-go` não expõe configuração de threads do ONNX Runtime (`RuntimeOptions` não tem campos).
- Limite: com este estado sintético, **14 entradas = 1.053 tokens** → `ErrInputTooLong` (`observed=1053 limit=1024`).

### O que isso significa para o projeto

Para um estado de ~650 tokens (próximo do orçamento de 800), **cada pergunta custa ~1,4 s**. Uma sessão com 60 chamadas candidatas gera 120 perguntas: **~170 s**. Com estados menores (~260 tokens) cai para ~0,43 s por pergunta, ~52 s para as mesmas 120. A meta anterior (RNF-03: 100 candidatas em ≤ 10 s, ≤ 50 ms por inferência) **não é alcançável** nesta CPU com este modelo. O número de 33 ms vinha do card do Hugging Face e não corresponde a sequências de centenas de tokens em CPU.

## Calibração da estimativa de tokens (amostra sintética)

A heurística herdada do original **superestima** o consumo real do mmBERT em ~25–35% neste formato de estado (real/estimado = 0,73 a 0,80, entre 1 e 12 entradas). Errar para cima é o lado seguro (nenhum `ErrInputTooLong`), mas desperdiça janela. A calibração fina (`tokenScale`, provavelmente ~0,8) fica para a Fase 3, com corpus real.

## Probabilidades

Nas 4 perguntas do teste, todas com o mesmo estado repetitivo, o modelo devolveu P(true) entre 0,85 e 0,88 e `action = 1,0`. Isso só prova que o pipeline funciona; **não avalia a qualidade das decisões**, que depende de transcripts reais (Fase 3).

## Decisões pendentes (para o mantenedor)

O tempo por compactação domina o desenho. Opções, não excludentes:

1. **Cascata de perguntas.** Perguntar primeiro só `result_*`; perguntar `call_*` apenas quando `keepResult < keepThreshold` (a decisão `keep` já resolve com a primeira). Corta até ~metade das perguntas quando a maioria é mantida, e menos que isso quando a maioria é descartada. Muda o algoritmo original (que sempre faz as duas).
2. **Estados menores por pergunta.** Com janelas de ~250–400 tokens o custo cai 2–4×, ao preço de menos contexto por decisão (já aceito na ADR 0006, agora com um incentivo extra).
3. **Teto de trabalho por compactação.** Orçamento de tempo (`maxMs`): perguntar das chamadas mais antigas/maiores primeiro e **manter** o restante (`unfit`) quando o tempo acabar. Mantém a política conservadora e garante resposta dentro do `timeoutMs` do hook.
4. **Compactação assíncrona.** O hook `turn.complete` dispara o daemon antes de o contexto encher (por exemplo a 40%), de modo que o resultado esteja pronto quando `session.compact` for chamado. Depende de o hook poder guardar o resultado entre eventos.
5. **Subir `timeoutMs` do hook** (padrão 30 s é insuficiente; 120–180 s é viável para uma compactação rara).

Recomendação: 1 + 3 + 5 já, 2 como parte do desenho de janelas, 4 como evolução.
