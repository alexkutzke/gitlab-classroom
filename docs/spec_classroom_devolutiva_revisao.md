# Especificação: revisão texto por texto em `classroom devolutiva`

Destino: repositório `gitlab-classroom` (`~/work/dev/gitlab-classroom`).
Escrita para ser implementada por outra sessão, sem contexto prévio desta.
Continua `spec_classroom_devolutiva.md`, já implementada.

## Problema

O ensaio de `classroom devolutiva` diz quem recebe issue e em qual projeto, mas
não mostra o texto que vai ser publicado. Ver o texto antes é justamente o que
o ensaio deveria permitir: o comentário foi escrito para a planilha do professor
e passa a ser mensagem pública para o aluno, e essa mudança de destinatário
costuma pedir ajuste de uma frase ou outra.

Falta também um caminho de revisão: escrever as devolutivas todas, e depois
passar por elas uma a uma, confirmando ou corrigindo antes de cada publicação.
Hoje só existem dois extremos, o ensaio que não publica nada e o `--aplicar` que
publica a turma inteira de uma vez.

## O que já existe e não deve ser reimplementado

| Onde | O que faz |
|---|---|
| `classroom devolutiva` | seleção de quem entra, montagem do corpo, publicação, `devolutivas.csv`, idempotência pelo hash |
| `internal/cli/corrigir.go` e `internal/correcao/` | a tela que percorre aluno a aluno, com edição de nota e de comentário. É o precedente de interação desta aplicação |
| `internal/store/store.go` | leitura e gravação de `notas.csv` |

## Comportamento pedido

### 1. `--texto` no ensaio

```bash
classroom devolutiva --exercicio lab01 --texto
```

Imprime, para cada aluno da rodada, o que seria publicado:

```
─────────────────────────────────────────────────────────────────────
JORGE GABRIEL MODROW  GRR20263366
ds122-2026-2-n-grr20263366/ds122-lab-01-assignment

Devolutiva: Laboratório 1

@grr20263366, segue a devolutiva da sua entrega.

bloco 2 e bloco 4 completos e corretos; no bloco 3 as fichas vão além
dos seis defeitos, mas só o viewport e o article fechado foram
corrigidos nos arquivos; no 1.4 o teste foi feito em ufprvirtual.ufpr.br,
e o enunciado pedia o site da disciplina

Commit avaliado: a1b2c3d
Comentários até 29/09/2026. Responda aqui mesmo se discordar de algum
ponto ou quiser entender melhor a correção.
```

Sem `--texto`, a saída continua a tabela de hoje. O texto sai quebrado em 72
colunas **apenas na tela**: o que vai para o GitLab é o markdown sem quebra
forçada, que o navegador reflui.

**Regra que sustenta o ensaio:** o corpo mostrado e o corpo publicado saem da
mesma função. Se houver dois caminhos de montagem, o ensaio deixa de valer como
conferência.

### 2. `--confirmar`, revisão texto por texto

```bash
classroom devolutiva --exercicio lab01 --aplicar --confirmar --prazo 2026-09-29
```

Percorre os alunos da rodada em ordem de nome, mostrando o mesmo bloco do
`--texto` e perguntando:

```
[s] publicar   [n] pular   [e] editar o comentário   [t] publicar todas as restantes   [q] sair
```

- **s** publica e vai para o próximo;
- **n** pula sem publicar e sem registrar nada;
- **e** abre o `$EDITOR` com **apenas o comentário**, sem o cabeçalho da menção
  nem o rodapé do prazo, que são montados na hora. Ao salvar, o texto novo é
  gravado em `notas.csv` e a tela volta a mostrar o bloco atualizado, ainda
  sem publicar;
- **t** publica o aluno atual e todos os restantes sem perguntar de novo;
- **q** encerra a rodada, preservando o que já foi publicado.

Sem `--confirmar`, `--aplicar` mantém o comportamento de hoje, que é o caminho
de script e o que roda sem terminal interativo.

`--confirmar` sem terminal interativo recusa com erro claro, em vez de travar
esperando leitura de um `stdin` que não existe.

### 3. Gravação incremental

`devolutivas.csv` é gravado **a cada publicação**, e não ao final da rodada.
Interromper com `q`, com `Ctrl+C` ou com queda de rede não pode perder o
registro do que já foi para o GitLab: sem isso, a rodada seguinte republica o
que já existe e o aluno recebe a devolutiva duas vezes.

### 4. Edição grava em `notas.csv`

O comentário editado na revisão vira o comentário da correção. Não existe
versão publicada diferente da versão guardada: é isso que faz o `hash` continuar
valendo como detector de devolutiva desatualizada.

O campo `corrigido_em` **não** muda ao editar só o comentário. Ele marca quando
a nota foi decidida, e a nota não mudou.

## Testes

Nenhum teste toca o gitlab.com nem abre editor de verdade.

1. `--texto` no ensaio não chama `CriarIssue` nenhuma vez e imprime um bloco por
   aluno da rodada;
2. o corpo impresso pelo ensaio é idêntico, caractere a caractere, ao corpo
   passado a `CriarIssue` quando a mesma rodada é publicada;
3. `s` publica e grava a linha; `n` não publica e não grava;
4. `q` no meio da rodada deixa em `devolutivas.csv` exatamente quem já foi
   publicado;
5. `t` publica o restante sem novas perguntas;
6. `e` com editor simulado grava o texto novo em `notas.csv` e o corpo publicado
   passa a ser o novo;
7. `--confirmar` sem terminal interativo devolve erro e não publica nada.

## O que não fazer

- não publicar nada fora do `--aplicar`, inclusive durante a revisão;
- não mudar o formato de `devolutivas.csv`;
- não alterar `corrigido_em` na edição de comentário;
- não montar o corpo em dois lugares.

## Documentação a atualizar junto

- `README.md` e `CLAUDE.md` do `gitlab-classroom`;
- a skill `correcao-ds122`, na seção "Publicar o comentário no fork do aluno",
  que passa a recomendar o ensaio com `--texto` e a primeira rodada com
  `--confirmar`.
