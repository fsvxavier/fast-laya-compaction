# 09 — Testes e avaliação

## Princípios

- O núcleo é testável sem cgo, sem bundle e sem rede: `go test ./...` roda em qualquer máquina.
- Os casos do original são portados primeiro; servem de especificação executável da fidelidade.
- Teste com modelo real fica atrás da build tag `laya_native` e é pulado quando `LAYA_BUNDLE_DIR` não está definido.

## Camadas

### Núcleo (`internal/compaction`)

Testes de tabela com `fakelaya`, portados de `tests/fast-jev-compaction.test.ts` (23 casos, 20 aplicáveis):

| Grupo | Casos do original |
|---|---|
| `ResolveOptions` | padrões e valores não finitos |
| `EstimateTokens` | palavras, dígitos e símbolos; não subestimar JSON |
| `CollectToolCalls` | pareamento e fixação; chamadas sem resultado |
| `FitState` | histórico com resultados como nota; goal padrão; inputs truncados antes do texto; chamadas antigas em uma linha; textos abreviados das mais antigas e colapso por último; *(original: lança erro — aqui: `ErrStateTooLarge`)* |
| `BatchCalls` | um lote quando cabe; divisão quando sobra pouco espaço; *(original: lança erro se uma pergunta não cabe — aqui: `ErrNoRoomForQuestions`)* |
| `DecideCall` | keep / drop_result / drop_call |
| `ApplyDecisions` | remove e trunca; `truncateHeadChars` inclusive 0 |
| `Compact` | reenvia o estado por lote e junta as respostas; não chama o modelo sem candidatas; redução pequena quando tudo é mantido; rejeita respostas malformadas |

Os 3 casos de `request.ts`/`client.ts` (requisição System One, chave de API) não se aplicam e são substituídos pelos testes do adaptador.

Casos novos:

- **Estágio 8**: transcript que força `windowed`; verifica obrigatório presente, prioridade "mesmo recurso" antes de "fixadas", ordenação por `i`, respeito ao orçamento.
- **Divisão de janela**: janela de 2 que não cabe vira 2 janelas de 1.
- **`unfit`**: chamada cuja parte obrigatória não cabe nem sozinha; não é perguntada e fica `keep`.
- **`ErrInputTooLong`**: `fakelaya` rejeita a primeira tentativa; verifica orçamento ×0,85, `retries`, e `unfit` após duas falhas.
- **Cancelamento**: `ctx` cancelado entre lotes interrompe sem resultado parcial.
- **Origem**: `Source`, `Unchanged` e `SourceIndex` corretos em mensagens intactas, truncadas e com blocos removidos.

### Golden files

`internal/compaction/testdata/stages/` guarda, para um transcript sintético por estágio (`full`, `inputs<=200`, `inputs<=60`, `texts abridged`, `old messages collapsed`, `old calls compacted`, `old messages left out`, `old calls merged`, `windowed`), o JSON exato do estado enviado. `go test -update` regrava. Mudança em golden file exige revisão explícita no PR.

Recomendação para a Fase 1: gerar os golden dos estágios 1–7 **rodando o original em TypeScript** sobre os mesmos transcripts, e comparar com o porte. É a verificação mais forte de fidelidade.

### Invariantes (fuzz)

`FuzzCompact` gera transcripts aleatórios (número de mensagens, chamadas, tamanhos, erros, ids repetidos ou órfãos) e probabilidades aleatórias no `fakelaya`, e verifica as 6 [invariantes do algoritmo](03-algoritmo.md#invariantes). Roda curto no CI (`-fuzztime 30s`).

### Servidor (`internal/server`)

`httptest` + `fakelaya`: 200 com campos de origem; 400 em JSON e opções inválidas; 413; 429 com compactação em curso; 503 antes do `ready`; 500 em resposta malformada; corpo de erro nunca contém texto do transcript.

### Adaptador (`internal/layaasker`, tag `laya_native`)

Com o bundle real:

- `Predict` com 2 perguntas `noul` → probabilidades em [0, 1], `InputTokens > 0`;
- estado gigante → `ErrInputTooLong` mapeado corretamente;
- `Close` libera e chamadas posteriores falham de forma limpa.

Sem a tag: o stub devolve `ErrNativeUnavailable`.

### Plugin

Ver [06 — Testes](06-plugin-claude-code.md#testes).

## Avaliação de qualidade (`fast-laya compact --report`)

Roda o algoritmo com o modelo real sobre `testdata/transcripts/*.json` e imprime, por arquivo e no total:

```
file                         msgs   calls  stage      req  win  kept  trunc  drop  pin  unfit  reduction  ms
session-refactor.json        212    87     windowed   44   44   21    38     17    11   0      61%        1810
...
token estimate: estimated 31 220 / reported 33 905 (ratio 0.92) → suggested --token-scale 1.09
```

Os transcripts são **sintéticos** ou anonimizados — nunca sessões reais com código ou dados de terceiros (o `.gitignore` do repositório já reflete essa política).

### Calibração do `tokenScale`

`suggested = Σ reported / Σ estimated` sobre o corpus, arredondado para cima em 0,05. A Fase 3 fixa o padrão de forma que < 1% das inferências do corpus gere `ErrInputTooLong`.

### Comparação com o Jev (opcional)

Se houver logs `decisions:` do original para as mesmas sessões, `fast-laya compare --jev decisions.log --laya response.json` calcula a concordância de ação por chamada (keep / drop_result / drop_call) e a matriz de confusão. Não é critério de aceite (os modelos são diferentes), só diagnóstico.

## CI

| Job | Comando |
|---|---|
| lint | `golangci-lint run` |
| test | `go test -race ./...` |
| fuzz | `go test -run=^$ -fuzz=FuzzCompact -fuzztime=30s ./internal/compaction` |
| plugin | testes do hook (Node) |
| native (manual) | `make provision && make test-native && make doctor` — não roda no CI público (download de ~1 GiB) |
