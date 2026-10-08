# Glossário

| Termo | Significado |
|---|---|
| **Asker** | Interface do núcleo que responde perguntas sobre um estado. Equivale ao `JevAsker` do original. Implementações: adaptador Laya e `fakelaya`. |
| **Bundle** | Diretório com `manifest.json`, `model.onnx`, tokenizer e `rl_agent_config.json`, verificado por SHA-256. Formato v1 do `laya-go`. |
| **BundleID** | `sha256:` + SHA-256 do `manifest.json`. Identifica univocamente o bundle carregado. |
| **Candidata** | Tool call com resultado e não fixada; recebe perguntas. |
| **Compactação** | Reduzir o transcript apagando ou truncando tool calls/resultados, sem reescrever texto. |
| **Estado** | Objeto `{context, goal, history}` enviado ao modelo. Resultados de ferramentas aparecem só como nota de tamanho. |
| **Estágio** | Passo de redução do estado (`full` … `old calls merged`, e o novo `windowed`). |
| **Fallback** | O hook devolve o controle ao resumo padrão do Claude Code (`next(event)`). |
| **Fixada (pinned)** | Chamada na primeira mensagem ou nas `preserveRecentMessages` finais; nunca alterada. |
| **Jev** | Modelo de decisão da TypeSafe usado pelo original, acessado por API paga. |
| **Janela** | No estágio `windowed`, grupo de `windowCalls` chamadas com estado próprio. |
| **Laya** | Modelo de decisão não autoregressivo da ConvAI Innovations, Apache 2.0. |
| **Lote** | Conjunto de perguntas enviado numa inferência junto com o estado global (estágios 1–7). |
| **noul** | Pergunta binária; a resposta é P(true). |
| **Recurso** | Valor de `file_path`, `path`, `notebook_path`, `pattern`, `url` ou `command` no input de uma chamada; usado para achar chamadas posteriores relacionadas. |
| **Redução** | `(charsBefore − charsAfter) / charsBefore`. |
| **Session message** | Mensagem do transcript no formato do Claude Code (`role`, `text`, `toolUses`, `toolResults`). |
| **unfit** | Motivo de decisão novo: a chamada não coube em nenhuma janela ou o modelo rejeitou a entrada após as tentativas; é mantida sem consulta. |
