# Fase 0 — Resultado do spike de viabilidade

- **Data:** 2026-10-08
- **Ambiente:** WSL2 (kernel 6.18), linux/amd64, 8 vCPUs, 15 GiB RAM, Go 1.27.1, gcc 13.3, `uv` (venv gerenciado com Python 3.13)
- **Código:** [`spike/phase0/`](../../../spike/phase0/main.go) · versões fixadas: [`scripts/versions.env`](../../../scripts/versions.env)

## Veredito

**O caminho nativo da [ADR 0004](../../adrs/0004-inferencia-in-process-com-laya-go-onnx.md) funciona.** O bundle foi exportado, verificado, carregado e respondeu perguntas `noul` reais dentro do processo Go, no WSL2. O plano B (`lengoman/laya-go` + servidor Python) **não** é necessário.

**Mas há dois problemas sérios, medidos:**

1. **A latência é de 10 a 30 vezes pior do que a documentação assumia** (ver "Latência").
2. **Na sonda de contraste (ver "Sonda de qualidade"), o modelo zero-shot não discriminou** chamadas obsoletas de necessárias: respondeu "true" a quase tudo, com pouca variação. Se isso se confirmar em amostras maiores, a premissa do projeto (usar o Laya base como no Jev) está em risco, e as decisões de latência são secundárias. **Este é o ponto a decidir primeiro.**

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
- **~1,6 ms por token** com ~190 tokens, ~2,2 ms/token em ~650 e ~2,5 ms/token em ~900 (a atenção torna o custo levemente superlinear).
- A CPU usada foi em média ~3,8 núcleos de 8 (`user` 1.470 s em 385 s); o `laya-go` não expõe configuração de threads do ONNX Runtime (`RuntimeOptions` não tem campos).
- Limite: com este estado sintético, **14 entradas = 1.053 tokens** → `ErrInputTooLong` (`observed=1053 limit=1024`).

### O que isso significa para o projeto

Para um estado de ~650 tokens (próximo do orçamento de 800), **cada pergunta custa ~1,4 s**. Uma sessão com 60 chamadas candidatas gera 120 perguntas: **~170 s**. Com estados menores (~260 tokens) cai para ~0,43 s por pergunta, ~52 s para as mesmas 120. A meta anterior (RNF-03: 100 candidatas em ≤ 10 s, ≤ 50 ms por inferência) **não é alcançável** nesta CPU com este modelo. O número de 33 ms vinha do card do Hugging Face e não corresponde a sequências de centenas de tokens em CPU.

## Calibração da estimativa de tokens (amostra sintética)

A heurística herdada do original **superestima** o consumo real do mmBERT em ~25–35% neste formato de estado (real/estimado = 0,73 a 0,80, entre 1 e 12 entradas). Errar para cima é o lado seguro (nenhum `ErrInputTooLong`), mas desperdiça janela. A calibração fina (`tokenScale`, provavelmente ~0,8) fica para a Fase 3, com corpus real.

## Sonda de qualidade (discriminação do modelo)

**Cenário** (`PROBE=1`, ~610 tokens por pergunta, 6 perguntas em ~9,7 s): um mini-transcript de 8 entradas com meta "corrigir TestParse em pkg/parser". `t1` lê `parse.go`; `t2` lê `CHANGELOG.md` (irrelevante); `t3` roda `go test` (falha); `t4` **relê** `parse.go`; `t5` edita `parse.go` (a correção); `t6` roda `go test` (passa). Respostas que um humano esperaria: o resultado de `t1` (substituído por `t4`/`t5`) e de `t2` (irrelevante) **baixos**; `t5` e `t6` **altos**.

Probabilidades P(true) de "manter", com as perguntas do original (formulação 1):

| Pergunta | Esperado | P(true) |
|---|---|---:|
| `result_t1` (leitura depois relida e editada) | baixo | 0,794 |
| `result_t2` (CHANGELOG irrelevante) | baixo | 0,763 |
| `result_t3` (falha já corrigida) | baixo/médio | 0,790 |
| `result_t6` (teste passando, final) | alto | 0,815 |
| `call_t2` (leitura irrelevante) | baixo | 0,958 |
| `call_t5` (a edição) | alto | 0,961 |

Outras duas formulações, na tentativa de achar uma que separe: (A) polaridade invertida, "pode ser apagado com segurança" → todas entre 0,955 e 0,966; (B) pergunta curta com `falseDescription`/`trueDescription` explicativas → todas entre 0,902 e 0,936.

**Leitura (limitada):** em três formulações, a variação entre perguntas ficou em ~0,02–0,05 e a resposta acompanhou a polaridade da pergunta, não o conteúdo. Com `keepThreshold = 0,5`, a formulação do original mantém **tudo** (redução 0, fallback sempre). A ordenação dos `result_*` da formulação 1 (t6 maior, t2 menor) aponta o sentido certo, mas `t1` não caiu. **Limites da evidência:** um único cenário sintético, seis perguntas por formulação, sem repetição. Não prova que o modelo seja inútil; prova que ele **não funciona zero-shot nesta tarefa com este desenho**, e que sozinho não substitui o Jev. Lembrando que o card do Laya descreve o uso pretendido como triagem de e-mail, guardrails e roteamento, com adaptação por fine-tuning para outros domínios.

### Avaliação em corpus rotulado (40 cenários, 137 perguntas)

Para sair da amostra de um cenário: [`tools/corpus`](../../../tools/corpus/main.go) gera cenários sintéticos com rótulos **por construção**, e o estado de cada um é montado pelo `FitState` de produção (inclui estágios comprimidos). Tipos: `superseded` (arquivo lido e depois editado/relido → apagável), `irrelevant` (documento nunca mais citado → apagável), `error_fixed` (falha depois corrigida e retestada → apagável), `cited` (conteúdo usado no raciocínio seguinte → manter), `error_cited` (erro em que os passos seguintes se apoiam → manter). 85 perguntas "apagável", 52 "manter". O avaliador é [`spike/phase0/eval.go`](../../../spike/phase0/eval.go); o corpus, [`testdata/probe/corpus.json`](../../../testdata/probe/corpus.json).

| Formulação | AUC | Acurácia em 0,5 (limiar do algoritmo) | Melhor acurácia (limiar ótimo) |
|---|---:|---:|---:|
| original (`should stay … verbatim`) | **0,445** | 0,387 | 0,635 |
| A (inverte: "pode ser apagado") | **0,548** | 0,620 | 0,635 |
| B (curta, com descrições false/true) | **0,451** | 0,380 | 0,620 |

Linha de base (sempre responder "apagar"): 0,620. Médias de P(manter) por tipo, formulação original: `superseded` 0,780, `irrelevant` 0,740, `error_fixed` 0,767, `cited` 0,751, `error_cited` 0,745 — **praticamente iguais** entre os tipos que deveriam ser manter e apagar.

**Conclusão: ao nível do acaso.** Com 137 perguntas o erro-padrão do AUC é ~0,05; os três valores (0,445–0,548) são indistinguíveis de 0,5. A acurácia no limiar do algoritmo (0,5) fica **abaixo** da linha de base nas duas formulações de polaridade "manter". O cenário é favorável (os sinais são textuais e claros: o mesmo caminho aparece depois em `Edit`/`Read`, ou é citado no texto do assistente), então não há motivo para esperar melhora em transcripts reais. Isso responde ao caminho 1 abaixo e **também descarta o caminho 2** (ranking relativo): a ordem das probabilidades não carrega sinal.

**Caminhos possíveis** (decisão do mantenedor; 1 e 2 já avaliados e descartados pelos números acima):
1. **Ranking relativo em vez de limiar absoluto**: apagar os X% com menor P(manter) dentro da sessão. Usa a ordem, não o valor. Precisa de avaliação em corpus maior para ver se a ordem tem sinal.
2. **Heurísticas determinísticas como sinal principal** (mesmo recurso lido/editado depois, tamanho do resultado, erro já corrigido) e o Laya como desempate. Perde a fidelidade ao original.
3. **Ajustar (fine-tune) o Laya** para esta tarefa. Exige dados rotulados e treino; fora do escopo atual e do formato de bundle v1 (que só aceita o perfil publicado).
4. **Testar o checkpoint `typed-decisions`** (ModernBERT-large, 1.024 tokens). O bundle v1 do `laya-go` não o suporta; exigiria perfil de exportação novo.
5. **Aceitar que o projeto não atinge a paridade com o Jev** e reposicionar o escopo.

Reproduzir: `PROBE=1` (e `QPHRASE=A|B`) em [`spike/phase0/main.go`](../../../spike/phase0/main.go).

## Decisões pendentes (para o mantenedor)

**0. Qualidade do modelo** (ver "Sonda de qualidade"): decidir o caminho antes de qualquer otimização de tempo.

**Latência.** Opções, não excludentes:

1. **Cascata de perguntas.** Como cada pergunta é uma sequência independente (a resposta a `result_*` não depende de quais outras perguntas vão junto) e `DecideCall` só consulta `keepCall` quando `keepResult < keepThreshold`, perguntar `result_*` primeiro e `call_*` apenas para as que ficaram abaixo do limiar produz **exatamente as mesmas ações** do algoritmo original. O que muda: (a) uma segunda passada sequencial; (b) `keepCall` fica desconhecido para as chamadas mantidas, o que só afeta o log `decisions:` (mostraria `call=—`). Corta as perguntas `call_*` das chamadas mantidas: quase metade do trabalho quando a maioria é mantida, quase nada quando a maioria é descartada.
2. **Estados menores por pergunta.** Com janelas de ~250–400 tokens o custo cai 2–4×, ao preço de menos contexto por decisão (já aceito na ADR 0006, agora com um incentivo extra).
3. **Teto de trabalho por compactação.** Orçamento de tempo (`maxMs`): perguntar das chamadas mais antigas/maiores primeiro e **manter** o restante (`unfit`) quando o tempo acabar. Mantém a política conservadora e garante resposta dentro do `timeoutMs` do hook.
4. **Compactação assíncrona.** O hook `turn.complete` dispara o daemon antes de o contexto encher (por exemplo a 40%), de modo que o resultado esteja pronto quando `session.compact` for chamado. Depende de o hook poder guardar o resultado entre eventos.
5. **Subir `timeoutMs` do hook** (padrão 30 s é insuficiente; 120–180 s é viável para uma compactação rara).

Recomendação: **primeiro resolver a qualidade** (sonda acima); só depois a latência. Se a qualidade se resolver: 1 + 3 + 5 já, 2 como parte do desenho de janelas, 4 como evolução.

## Receita de exportação (reproduzível)

Passos executados, na ordem, a partir de [`spike/phase0/download.sh`](../../../spike/phase0/download.sh) (baixa e verifica ONNX Runtime, `libtokenizers.a`, o checkpoint em `CHECKPOINT_REVISION` e o SDK, e roda `uv sync --frozen` no `tools/export` do `laya-go` v0.2.6). Depois, dentro de um clone de `metalagman/laya-go` na tag `v0.2.6`:

```sh
export UV_OFFLINE=1 PYTHONDONTWRITEBYTECODE=1 HF_HUB_OFFLINE=1 TRANSFORMERS_OFFLINE=1 ORT_DISABLE_TELEMETRY=1
export PYTHONPATH=$PWD/tools/export/src UV_PROJECT_ENVIRONMENT=<venv do uv sync>
go build -o .cache/bundlecheck ./internal/cmd/bundlecheck
uv run --project tools/export --offline --frozen python -m laya_export.cli \
  --profile tools/export/profiles/laya-multilingual-v1.json export \
  --source <checkpoint> --sdk <sdk> --output <novo diretório do bundle> \
  --epoch 1790035200 --repository-root $PWD --verifier $PWD/.cache/bundlecheck
```

Saída: `bundle_id = sha256:ca06dcb1166aad3287d32ed5423d4551d36af246acc0e0421780c141b9377f23`. Build e execução do spike: `CGO_ENABLED=1 CGO_LDFLAGS=-L<dir do libtokenizers.a> go build -tags laya_native` e `LAYA_ONNXRUNTIME_LIBRARY`, `LAYA_TOKENIZERS_LIBRARY`, `LAYA_BUNDLE_DIR` no ambiente.

Os artefatos desta execução (bundle de 1,29 GiB, bibliotecas, venv) ficaram no diretório temporário da sessão do Claude Code; o destino documentado é `~/.local/share/fast-laya/` (a mover quando o `make provision` existir).
