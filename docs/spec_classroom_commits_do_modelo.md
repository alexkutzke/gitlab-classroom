# Especificação: commits do modelo não podem incluir refs de merge request

Destino: repositório `gitlab-classroom` (`~/work/dev/gitlab-classroom`).
Escrita para ser implementada por outra sessão, sem contexto prévio desta.

## Problema

A coleta classifica como `fork_sem_commit` a entrega de aluno que abriu um
merge request do fork dele para o repositório-modelo. Os commits do aluno
existem, estão no ramo `main` do fork e dentro do prazo, e mesmo assim o
`classroom` conta zero commits próprios.

Caso que revelou a falha, em 18/09/2026, na turma `ds122_n`:

```
trabalho1;GRR20261406;fork_sem_commit;dss122-2026-2-n-grr20261406/ds122-project-2026-2;;;0;;;0;...
```

O fork tem 7 commits do aluno, o último em 12/09/2026 20:26, antes do prazo de
13/09. O aluno também abriu o MR !1 de `dss122-2026-2-n-grr20261406/ds122-project-2026-2`
para `ds122-alexkutzke/ds122-project-2026-2`.

## Causa

`internal/coleta/coleta.go`, `shasDoModelo`, monta o conjunto de SHAs do
repositório-modelo assim:

```go
commits, err := c.Cliente.Commits(caminho, "", true)
```

O terceiro argumento vira `opt.All = true` em `internal/gitlab/gitlab.go:273`,
`clienteAPI.Commits`. Na API do GitLab, `all=true` percorre **todas as refs**
do repositório, e um projeto que recebeu merge request guarda
`refs/merge-requests/<iid>/head` e `refs/merge-requests/<iid>/merge`. Essas
refs apontam para os commits do fork de origem, ou seja, para os commits do
aluno.

Resultado: os commits do aluno entram no conjunto "commits do modelo",
`semOsDoModelo` subtrai todos, `proprios` fica vazio e a entrega é marcada
`fork_sem_commit`.

Confirmação pelo lado do git, sem API:

```
$ git ls-remote git@gitlab.com:ds122-alexkutzke/ds122-project-2026-2.git
8c3428ac...  refs/heads/main
b6f0d5ed...  refs/merge-requests/1/head
31344c6c...  refs/merge-requests/1/merge

$ git ls-remote git@gitlab.com:dss122-2026-2-n-grr20261406/ds122-project-2026-2.git
b6f0d5ed...  refs/heads/main
```

O `b6f0d5ed` é o último commit do aluno e está nas duas listas.

A falha atinge o histórico inteiro do MR, não só o commit de topo: a ref
`head` alcança todos os ancestrais do ramo enviado, e todos são subtraídos.

## Alcance verificado

Varredura de todos os registros `fork_sem_commit` das duas turmas contra os
heads das refs de merge request dos cinco repositórios-modelo, em 18/09/2026.
Três registros errados, todos pela mesma causa:

| Turma | Exercício | Aluno | Situação real |
|---|---|---|---|
| n | `trabalho1` | GRR20261406 | `entregue`, 7 commits, último 12/09 20:26, no prazo |
| n | `prepare` | GRR20261406 | fork com commits de 19/08, no prazo (trabalho em dupla, autoria de GRR20263633) |
| n | `css` | GRR20261426 | `sem_commit_no_prazo`: 2 commits, 09/09 e 13/09, prazo 03/09 |

Os demais `fork_sem_commit` conferem: o topo do fork é commit do próprio
modelo, às vezes de uma versão anterior dele (`40616c5` no `prepare`,
`46c338a` no `html`), e nesses casos o aluno de fato não commitou.

## Correção pedida

`shasDoModelo` passa a montar o conjunto a partir dos **ramos** do modelo, e
não de todas as refs.

1. Nova operação na interface `gitlab.Cliente`
   (`internal/gitlab/gitlab.go:58`):

   ```go
   // Ramos lista os ramos de um projeto. Serve para varrer o histórico do
   // repositório-modelo sem passar pelas refs de merge request, que trazem
   // commits de fork de aluno para dentro do modelo.
   Ramos(projeto string) ([]string, error)
   ```

   Implementação em `clienteAPI` sobre `Branches.ListBranches`, paginada
   igual às demais listagens, com cache próprio (`ramos *cache[[]string]`,
   ao lado de `commits`) e limpeza em `Renovar`.

2. `shasDoModelo` passa a pedir os ramos do modelo e, para cada ramo,
   `Commits(caminho, ramo, false)`, unindo os SHAs num só conjunto. O
   `todos=true` sai dessa chamada.

3. Modelo sem ramo algum, ou repositório vazio, continua devolvendo conjunto
   vazio sem erro, como hoje quando `Commits` responde 404.

O custo em requisições fica quase igual: o cache do cliente já guarda a
resposta por chave, e um repositório-modelo tem um ou dois ramos. A varredura
por ramo continua limitada por `paginaMaxima`.

## O que não muda

- A chamada com `todos=true` sobre o **fork do aluno**, em `coletarNoProjeto`,
  fica como está. Lá o objetivo é justamente achar trabalho fora do ramo
  padrão, e uma ref de merge request no fork do aluno aponta para commit dele,
  que deve mesmo ser contado. É o que produz o detalhe
  `"N commit(s) fora do ramo main"`.
- A regra de prazo, a classificação em dupla e a escolha do commit avaliado
  continuam iguais. A correção só muda quais SHAs entram no conjunto do
  modelo.

## Risco aceito

Commit que esteve no ramo do modelo e saiu dele depois, por rebase ou
force-push, deixa de constar no conjunto e passaria a contar como trabalho do
aluno no fork que o herdou. Não vale acrescentar as tags só por isso: o
histórico dos modelos da disciplina é linear e não sofre reescrita, e o dia em
que sofrer, o efeito é um falso positivo visível na correção, e não uma
entrega perdida em silêncio.

## Testes

Padrão do repositório: teste de tabela sobre cliente dublado, sem falar com o
gitlab.com.

- `internal/coleta/coleta_test.go`: caso novo em que o cliente falso devolve,
  para o modelo, um ramo `main` com os commits do enunciado, e para o fork do
  aluno esses mesmos commits mais dois do aluno. A situação esperada é
  `entregue`, com `Commits == 2`. O teste tem que falhar contra o código
  atual, e falha se o dublê de `Commits` com `todos=true` devolver os commits
  do aluno junto com os do modelo, que é como a API se comporta hoje.
- Manter `TestForkSemCommitDoAluno` passando: fork sem commit próprio continua
  `fork_sem_commit`.
- Os dublês de `gitlab.Cliente` em `internal/coleta/coleta_test.go` e
  `internal/tui/tarefa_test.go` precisam da nova operação `Ramos`.

## Como verificar depois de implementar

Nas pastas `ds122_n/` e `ds122_t/`, com o token no chaveiro:

```
classroom coletar --exercicio trabalho1 --detalhado --dry-run
```

O aluno GRR20261406 tem que aparecer como `entregue`, com 7 commits e último
commit em 12/09/2026. Depois, `classroom coletar` sem `--dry-run` nas duas
turmas regrava `entregas.csv` e deixa os três registros da tabela acima na
situação certa. A coleta não toca nas notas já lançadas.
