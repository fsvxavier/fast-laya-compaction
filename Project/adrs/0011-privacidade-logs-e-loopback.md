# 0011 — Privacidade: loopback e logs sem conteúdo

- **Status:** Aceita
- **Data:** 2026-10-08
- **Relacionadas:** ADR 0008

## Contexto

Transcripts de sessões de código contêm código-fonte, caminhos, saídas de comandos e, com frequência, segredos (variáveis de ambiente, tokens em logs, conteúdo de `.env`). Um dos motivos do projeto é que esses dados não saiam da máquina. O daemon recebe o transcript inteiro por HTTP e não tem autenticação.

## Decisão

1. O daemon escuta em `127.0.0.1:8610` por padrão e **recusa iniciar** em endereço que não seja loopback sem `--allow-remote`.
2. Na imagem Docker o daemon escuta em `0.0.0.0` dentro do container; a documentação exige mapear só em loopback (`-p 127.0.0.1:8610:8610`).
3. Logs (`log/slog`, JSON) registram apenas metadados: ids curtos (`t3`), nomes de ferramentas, contagens, probabilidades, estágios, tempos, tamanhos. **Nunca** texto de mensagens, inputs, resultados ou o estado montado.
4. `--log-content` habilita o conteúdo nos logs de `debug`, para depuração local, com um aviso no início do processo.
5. Corpos de erro HTTP nunca incluem trechos do transcript.
6. Nenhuma telemetria: `ORT_DISABLE_TELEMETRY=1` definido pelo próprio processo; nenhuma chamada de rede além do loopback.
7. Transcripts de teste e avaliação versionados são sintéticos ou anonimizados.

## Alternativas consideradas

- **Token de autenticação no daemon** — protege contra outros processos locais, mas eles já têm acesso aos arquivos do usuário; custo de configuração sem ganho real no modelo de ameaça local. Reavaliar se `--allow-remote` passar a ser usado de verdade.
- **Unix domain socket** — `$.http.fetch` do Claude Code espera URL HTTP.

## Consequências

- O transcript fica na máquina, em memória, durante a compactação.
- Depurar decisões ruins exige reproduzir com `--log-content` ou com `fast-laya compact` sobre um transcript salvo localmente.
