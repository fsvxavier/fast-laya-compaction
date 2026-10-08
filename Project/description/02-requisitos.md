# 02 — Requisitos

## Requisitos funcionais

| ID | Requisito |
|---|---|
| RF-01 | Receber um transcript (lista de mensagens `user`/`assistant` com `toolUses` e `toolResults`) e devolver o transcript compactado, as decisões por chamada e as estatísticas. |
| RF-02 | Parear cada `tool_use` com seu `tool_result` por `tool_use_id`. Chamadas sem resultado não são candidatas. |
| RF-03 | Fixar (nunca alterar) chamadas cuja mensagem de chamada ou de resultado seja a primeira do transcript ou esteja entre as `preserveRecentMessages` finais. |
| RF-04 | Para cada chamada candidata, obter do Laya duas probabilidades: manter a chamada (`keepCall`) e manter o resultado inteiro (`keepResult`). |
| RF-05 | Decidir: `keepResult ≥ keepThreshold` → manter; senão `keepCall ≥ keepThreshold` → truncar o resultado; senão → remover chamada e resultado. |
| RF-06 | Ao truncar, manter os primeiros `truncateHeadChars` caracteres e acrescentar uma nota indicando quantos caracteres foram removidos e que a ferramenta pode ser re-executada. |
| RF-07 | Nunca alterar texto de usuário ou assistente. Nunca deixar `tool_use` sem `tool_result` ou vice-versa. Remover mensagens que fiquem vazias. |
| RF-08 | Montar o estado e encolhê-lo nos estágios 1–7 do original até caber em `maxStateTokens`. |
| RF-09 | Quando o estado global não couber, usar o estágio `windowed`: um estado próprio por janela de `windowCalls` chamadas. |
| RF-10 | Quando nem a parte obrigatória de uma janela couber, ou o modelo rejeitar a entrada após as tentativas, marcar as chamadas como `keep` com motivo `unfit`. |
| RF-11 | Expor o algoritmo como daemon HTTP local (`fast-laya serve`) com `POST /v1/compact`, `GET /healthz` e `GET /readyz`. |
| RF-12 | Expor o algoritmo como comando único (`fast-laya compact`) que lê da stdin e escreve na stdout, com modo `--report` para avaliação. |
| RF-13 | Informar, na resposta, a origem de cada mensagem e bloco mantido (`source`, `unchanged`, `sourceIndex`), para o hook reaproveitar os objetos do Claude Code. |
| RF-14 | Plugin do Claude Code que substitui `session.compact`, cai no resumo padrão em erro, timeout ou redução insuficiente, e dispara a compactação em `turn.complete` ao atingir `compactAtPercent`. |
| RF-15 | Script de provisionamento que instala ONNX Runtime, `libtokenizers` e o bundle do modelo, e Dockerfile alternativo com tudo embutido. |
| RF-16 | Registrar, por compactação: redução, contagem por motivo, estágio, número de inferências, janelas, `unfit`, tentativas e tempo. |

## Requisitos não funcionais

| ID | Requisito |
|---|---|
| RNF-01 | **Custo zero por uso.** Nenhuma chamada a serviço pago; nenhuma chave de API. |
| RNF-02 | **Privacidade.** O transcript nunca sai da máquina. O daemon escuta só em loopback por padrão. Logs não contêm conteúdo de mensagens por padrão. |
| RNF-03 | **Desempenho.** ≤ 10 s para 100 chamadas candidatas em 4 vCPUs; ≤ 50 ms por inferência no p50 (o card do Laya no Hugging Face cita ~33 ms; **não verificado em CPU** com o `metalagman/laya-go` — medido na Fase 0). |
| RNF-04 | **Recursos.** Daemon com até 2 GiB de RAM residente; bundle com ~1,2 GiB de disco. |
| RNF-05 | **Plataforma.** linux/amd64, incluindo WSL2. Outras plataformas ficam fora do suporte. |
| RNF-06 | **Segurança de dados.** Em qualquer dúvida (erro, entrada que não cabe), manter. Nenhuma chamada é apagada sem uma resposta do modelo. |
| RNF-07 | **Reprodutibilidade.** Versões fixas de `laya-go`, ONNX Runtime, `libtokenizers` e revisão do checkpoint; artefatos verificados por SHA-256. |
| RNF-08 | **Testabilidade.** O núcleo não depende de cgo nem do modelo; testes rodam com `go test ./...` sem bundle. |
| RNF-09 | **Fidelidade.** Mesmos textos fixos, perguntas, nomes de estágio e regra de decisão do original, salvo os acréscimos documentados. |

## Fora do escopo

- GPU e outros execution providers.
- Checkpoints `english` e `typed-decisions` (o bundle v1 do `laya-go` só aceita `laya-multilingual-v1`).
- Resumo por LLM ou qualquer reescrita de texto.
- Interface gráfica (o original tem uma demo SwiftUI).
- Autenticação no daemon (loopback por padrão).
- macOS, Windows nativo e arm64.
- Ajuste fino (fine-tuning) do Laya.
