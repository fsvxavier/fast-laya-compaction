# 08 — Provisionamento

Dois caminhos ([ADR 0009](../adrs/0009-provisionamento-script-e-docker.md)): script local (principal) e Docker (alternativo). Os dois produzem os mesmos três artefatos:

| Artefato | Versão | Origem |
|---|---|---|
| `libonnxruntime.so` | 1.29.0, linux-x64, CPU | release oficial do ONNX Runtime no GitHub |
| `libtokenizers.a` (estática) | `daulet/tokenizers` v1.27.0 | release do GitHub; verificada na Fase 0; ligada em build com `CGO_LDFLAGS=-L<dir>` |
| bundle `laya-multilingual-v1` | `convaiinnovations/laya-multilingual@052592a15…` + SDK `NandhaKishorM/laya@573e5b62…` | exportado localmente pelo runbook `official-export-v1` do `laya-go`, via `uv`; `bundle_id = sha256:ca06dcb1…` (Fase 0) |

Todas as versões e SHA-256 ficam em `scripts/versions.env`, a única fonte de verdade, lida pelo script e pelo Dockerfile.

## Requisitos da máquina

- Linux x86-64 ou WSL2 (Ubuntu 22.04+ testado na Fase 0).
- 4 vCPUs e 2 GiB de RAM livres para o daemon; ~4 GiB livres durante a exportação.
- ~1,4 GiB de disco para bibliotecas e bundle (medido: bundle 1,29 GiB); ~2 GiB temporários para o venv (~1 GB) e o checkpoint baixado (~0,7 GB). Pico de RAM do daemon medido: 1,96 GB.
- Go 1.26.6+ com cgo (`gcc`) para compilar com `-tags laya_native` (o `laya-go` v0.2.6 exige 1.26.6).
- `uv` **somente** para exportar o bundle (ele baixa o Python 3.13 e o torch CPU do lockfile). `go-task` é dispensável. Nada disso é necessário em tempo de execução.
- Acesso à internet só durante o provisionamento (Hugging Face e GitHub). Depois, tudo roda offline.

## Caminho principal: `make provision`

`scripts/provision.sh`, idempotente — cada passo é pulado se o artefato já existe com o SHA-256 esperado:

```
1. checa pré-requisitos (uname = Linux x86_64, gcc, go, python3 ≥ 3.10, task, curl, sha256sum)
2. baixa ONNX Runtime 1.29.0 → ~/.local/share/fast-laya/lib/        (verifica SHA-256)
3. baixa libtokenizers.a       → ~/.local/share/fast-laya/lib/        (verifica SHA-256)
4. baixa o checkpoint (revisão fixa, SHA-256 por arquivo) e o SDK NandhaKishorM/laya (commit fixo) → ~/.cache/fast-laya/
5. clona metalagman/laya-go na tag fixada → ~/.cache/fast-laya/laya-go; `uv sync --frozen` do tools/export
6. roda `uv run … laya_export.cli … export` (equivale a `task bundle:export`) e `bundlecheck` (Fase 0: ~4 min, sem GPU)
7. copia o bundle verificado → ~/.local/share/fast-laya/bundle/laya-multilingual-v1/
8. grava ~/.local/share/fast-laya/.env
9. imprime o BundleID (sha256 do manifest.json) e o próximo comando
```

Opções: `--prefix DIR`, `--keep-cache` (não apaga venv e checkpoint), `--offline` (falha se precisar baixar algo).

Comandos relacionados no `Makefile`:

| Alvo | Faz |
|---|---|
| `make provision` | roda o script |
| `make build` | `CGO_ENABLED=1 go build -tags laya_native -o bin/fast-laya ./cmd/fast-laya` |
| `make build-stub` | build sem cgo (para testes; `serve` responde 503) |
| `make serve` | carrega o `.env` e roda `bin/fast-laya serve` |
| `make doctor` | `fast-laya doctor`: confere as três entradas, valida o bundle, carrega o modelo, faz uma inferência de teste e imprime latência |
| `make test` | `go test ./...` (sem cgo, sem modelo) |
| `make test-native` | `go test -tags laya_native ./internal/layaasker/...` com o `.env` |

### Rodando como serviço do usuário (opcional)

O README trará uma unidade `systemd --user` de exemplo (`~/.config/systemd/user/fast-laya.service`) com `EnvironmentFile=` apontando para o `.env`. No WSL2 isso exige `systemd=true` em `/etc/wsl.conf`; sem systemd, `make serve` em um terminal.

## Caminho alternativo: Docker

`Dockerfile` multi-stage:

```dockerfile
# 1. exporter — Python + go-task; gera o bundle
FROM python:3.12-slim AS exporter
# instala task, clona laya-go na tag fixada, roda o runbook official-export-v1,
# deixa o bundle verificado em /out/bundle

# 2. libs — baixa ONNX Runtime e libtokenizers, verifica SHA-256 → /out/lib

# 3. builder — golang com gcc; CGO_ENABLED=1 go build -tags laya_native

# 4. runtime — debian:bookworm-slim, usuário não-root
COPY --from=builder /out/fast-laya /usr/local/bin/
COPY --from=libs    /out/lib       /opt/fast-laya/lib
COPY --from=exporter /out/bundle   /opt/fast-laya/bundle
ENV LAYA_ONNXRUNTIME_LIBRARY=/opt/fast-laya/lib/libonnxruntime.so.1.29.0 \
    LAYA_TOKENIZERS_LIBRARY=/opt/fast-laya/lib/libtokenizers.a \
    LAYA_BUNDLE_DIR=/opt/fast-laya/bundle \
    FAST_LAYA_ADDR=0.0.0.0:8610 FAST_LAYA_ALLOW_REMOTE=true
EXPOSE 8610
HEALTHCHECK CMD ["fast-laya", "doctor", "--ready-only"]
ENTRYPOINT ["fast-laya", "serve"]
```

Dentro do container o daemon escuta em `0.0.0.0` (necessário para o mapeamento de porta), por isso `ALLOW_REMOTE=true`. A proteção passa a ser o mapeamento **só em loopback** no host:

```sh
docker build -t fast-laya:0.1.0 .
docker run -d --name fast-laya --cpus 4 --memory 2g -p 127.0.0.1:8610:8610 fast-laya:0.1.0
```

Imagem esperada: ~1,6 GB (bundle ~1,2 GiB + ORT + binário). O README deve alertar contra `-p 8610:8610` sem o `127.0.0.1`.
