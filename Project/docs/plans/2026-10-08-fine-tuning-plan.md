# Plano de fine-tuning do Laya

- **Data:** 2026-10-08 · **ADR:** [0012](../../adrs/0012-fine-tuning-do-laya-para-compactacao.md)
- **Convenções:** Go no pipeline de dados, Python só no treino. Nada de dados pessoais, checkpoints ou bundle no git.

## Etapas

| # | Etapa | Entrega | Portão de saída |
|---|---|---|---|
| A | Construtor de estado em janela (Fase 2 do plano geral) | `compaction.BuildWindows` | Testes + fuzz das invariantes; todo estado ≤ orçamento |
| B | Leitor de sessões e rotulador | `tools/labeler` (Go) lê `~/.claude/projects/**/*.jsonl` e grava `dataset/*.jsonl` | Estatísticas de rótulos plausíveis; amostra inspecionada à mão |
| C | Linha de base de regras | AUC/acurácia das regras sobre o conjunto de teste | Número fixado para comparar |
| D | Ambiente de treino | venv com torch CUDA, `laya` e dependências; teste de memória em 6 GB | Um passo de treino sem OOM |
| E | Treino e avaliação | `training/train.py`, relatório de AUC por tipo | AUC ≥ 0,80 e ganho sobre C no teste |
| F | Exportação | perfil novo, bundle novo, reavaliação nativa (`spike/phase0/eval.go`) | Mesmo AUC no runtime Go (paridade) |
| G | Retomar Fases 2–6 do plano geral | núcleo, adaptador, daemon, plugin | conforme plano geral |

## Rótulos (etapa B)

Para cada chamada candidata `c` com resultado `R` (visível só ao rotulador):

- **`result_label` = 0 (apagável)** se: (a) `c` é `Read` e o mesmo arquivo é lido de novo, editado ou escrito depois; (b) `c` é `Bash`/`Grep`/`Glob` idêntico reexecutado depois; (c) `c` é `Edit`/`Write` (resultado só confirma, sem conteúdo útil); (d) nenhum identificador raro de `R` aparece depois.
- **`result_label` = 1 (necessário)** se pelo menos *m* identificadores raros distintos de `R` (≥ 6 caracteres, fora de vocabulário comum, ausentes da própria entrada da chamada) aparecem em texto do assistente ou em entradas de ferramentas posteriores, dentro das próximas *K* mensagens e antes de a chamada ser substituída. Erros de `Bash` ainda não corrigidos contam como 1.
- **`call_label`**: 1 se o recurso da chamada (arquivo/comando) é referenciado depois ou o resultado é necessário; 0 se foi substituída ou nunca mais citada.
- Alvos suaves: 0,05 / 0,95. Parâmetros *m*, *K* e a lista de palavras comuns ficam em configuração e são validados por amostragem manual.

## Treino (etapa D/E)

Receita do notebook do SDK adaptada a uma GPU de 6 GB: embeddings congelados, `gradient_checkpointing`, `fp16` autocast, `MICRO_BATCH` 4–8 com acúmulo para lote efetivo de 32, LR encoder 2,5e-5 e cabeça 1e-4, 3–4 épocas, calibração de temperatura no conjunto de validação. Checkpoint por época. Métricas: AUC e acurácia por `kind`, Brier e ECE na validação; relatório no teste.

## Riscos desta trilha

Rótulos ruidosos; desbalanceamento (a maioria das chamadas `Bash` é apagável); memória de 6 GB; deriva de distribuição se o formato de estado mudar; exposição de dados pessoais nos pesos. Cada um tem mitigação na ADR 0012.
