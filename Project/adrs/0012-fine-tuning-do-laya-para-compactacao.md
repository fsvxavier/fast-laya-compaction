# 0012 — Fine-tuning do Laya para decidir o que apagar

- **Status:** Aceita
- **Data:** 2026-10-08
- **Relacionadas:** ADR 0003, ADR 0004, ADR 0006, ADR 0011

## Contexto

A Fase 0 mostrou que o checkpoint `laya-multilingual` zero-shot **não discrimina** resultados de ferramentas ainda necessários dos obsoletos: AUC 0,445 / 0,548 / 0,451 em 137 perguntas rotuladas, em três formulações ([resultado](../docs/plans/fase-0-resultado.md#avaliação-em-corpus-rotulado-40-cenários-137-perguntas)). Zero-shot e ranking relativo foram descartados. O mantenedor escolheu **ajustar (fine-tune) o Laya** para esta tarefa.

O SDK oficial traz a receita de ajuste (`notebooks/laya_finetune_typed_decisions_2xT4_kaggle.ipynb`): itens `(estado, pergunta, distribuição-alvo)`, perda = entropia cruzada suave + gradiente de política (GRPO) com recompensa por regra de pontuação própria, AdamW com taxas distintas para encoder (2,5e-5) e cabeça (1e-4), calibração de temperatura ao fim. O treino original usou 1.200 casos em 2×T4 (~5 min). A máquina deste projeto tem uma **RTX 2060 de 6 GB** (WSL2) e 8 CPUs.

Há 203 sessões reais do Claude Code do mantenedor em `~/.claude/projects` (≈400 MB, ≈18 mil chamadas de ferramenta só em 60 arquivos), com `Bash`, `Edit`, `Read` e `Write` dominando.

## Decisão

1. **Ajustar o `laya-multilingual`** (mmBERT-base, 322M) para duas perguntas `noul` por chamada, com os textos do original: "o resultado precisa ficar" e "a chamada precisa ficar".
2. **Rótulos de uso futuro, sem LLM.** O resultado de uma chamada é *necessário* quando o que ele continha é usado depois: identificadores raros do resultado reaparecem em texto do assistente ou em entradas de ferramentas posteriores. É *apagável* quando foi substituído (mesma leitura repetida, arquivo editado/escrito depois, mesmo comando reexecutado) ou nunca mais referenciado. O modelo **não vê o resultado** (só a nota de tamanho), então aprende a prever o uso futuro a partir da chamada e do contexto: uma tarefa que regras fixas só aproximam.
3. **Dados**: sessões reais locais (só leitura) + corpus sintético. **Tudo fica fora do git** (`.gitignore`): dataset, checkpoints e bundle. Os **pesos ajustados podem memorizar trechos** das sessões e nunca devem ser publicados nem commitados.
4. **Divisão por sessão** (não por chamada) em treino/validação/teste, para medir generalização e evitar vazamento entre chamadas da mesma sessão.
5. **Treino em 6 GB**: embeddings congelados (o vocabulário de 256k × 768 é ~60% dos parâmetros), checkpoint de gradiente, precisão mista, micro-batches pequenos. Torch com CUDA num venv próprio (o venv do exportador é CPU).
6. **Estado de treino = estado de inferência**: os itens usam o mesmo construtor de estado do daemon (estágios 1–7 e janela), para não haver descompasso entre treino e uso.
7. **Critério de aceite**: no conjunto de teste de sessões reais, AUC ≥ 0,80 em `result_*` e ganho claro sobre a linha de base de regras; senão a decisão é revista (ADR nova).
8. **Exportação**: perfil novo de exportação do `laya-go` (mesmo contrato opset 18/FP32/CPU, hashes novos do checkpoint ajustado), bundle novo, reavaliação com o mesmo avaliador nativo da Fase 0.

## Alternativas consideradas

- **Heurísticas determinísticas** (opção 1 da sessão): mais simples e sem treino, mas perde o objetivo de usar o Laya. Continuam como **linha de base** de comparação.
- **Rótulos por LLM**: custo, privacidade e dependência externa; contraria o objetivo local e gratuito.
- **Só corpus sintético**: sem exposição de dados pessoais, mas o modelo aprenderia os artefatos do gerador.
- **Outro checkpoint (`typed-decisions`)**: bundle v1 do `laya-go` não o suporta e o modelo é maior (421M).

## Consequências

- Três subsistemas novos: extrator/rotulador (Go), treino (Python/CUDA), perfil de exportação. O núcleo de compactação continua em Go.
- O modelo fica acoplado ao formato de estado e às perguntas; mudar um exige retreinar.
- Risco: rótulos de "uso futuro" são aproximados; ruído limita o teto de qualidade. Mitigação: validação em sessões não vistas e inspeção manual de amostras.
- Risco: 6 GB de VRAM é justo; o plano prevê congelar mais camadas se faltar memória.
- Privacidade: dataset e pesos são dados sensíveis locais.
