# Especificação: `classroom devolutiva`

Destino: repositório `gitlab-classroom` (`~/work/dev/gitlab-classroom`).
Escrita para ser implementada por outra sessão, sem contexto prévio desta.

## Problema

O comentário que o professor escreve ao corrigir fica em
`.classroom/notas.csv` e não chega ao aluno. Hoje ele só aparece na planilha do
professor e, quando muito, resumido no Moodle junto da nota. O aluno recebe um
número e não sabe o que errou.

A devolutiva escrita já existe e é específica: "faltou o `<label for>` nos três
campos de contato.html", "o README continua com o texto de fábrica". Falta
entregá-la onde ela faz sentido, que é ao lado do código a que se refere.

O objetivo é publicar cada comentário como uma **issue no fork do próprio
aluno**, e guardar o que foi publicado para não duplicar na rodada seguinte.

## Decisões do professor, tomadas em 22/09/2026

1. **A issue não leva nota.** Só o texto da devolutiva. O registro da nota
   continua sendo o `diario` e o UFPR Virtual. Publicar a nota no GitLab criaria
   um segundo lugar onde ela existe, que precisaria acompanhar toda recorreção.
2. **Entrega compartilhada não recebe tratamento especial.** A issue vai para o
   fork onde a entrega está, e quem tem acesso a esse fork lê. O caso do
   trabalho individual entregue em dupla é anomalia do semestre, não regra a
   codificar.
3. **Quem não entregou não recebe issue.** Sem fork não há onde publicar, e
   esses alunos são tratados por e-mail, caso a caso.
4. **A issue fica aberta.** O professor informa no corpo o prazo para comentários
   e não fecha a issue ao publicar.

## O que já existe e não deve ser reimplementado

| Onde | O que faz |
|---|---|
| `.classroom/notas.csv` | `exercicio;grr;nota;comentario;corrigido_em`, o texto a publicar |
| `.classroom/entregas.csv` | coluna `projeto` com o caminho completo do fork de cada aluno em cada exercício, e `commit` com o commit avaliado |
| `.classroom/alunos.csv` | coluna `usuario`, o login do aluno no GitLab |
| `internal/gitlab/gitlab.go`, `Cliente` | interface que a coleta consome, com cache por operação e tradução de erro |
| `internal/cli/cli.go`, `abrir` e `cliente` | abertura do store e do cliente com o token resolvido |
| `internal/store/store.go` | leitura e gravação de CSV com separador `;`, coluna por nome, gravação atômica |
| `internal/cli/equipes.go` | formato de comando cobra com subcomando, para copiar a forma |

## Antes de começar

O repositório tem trabalho não commitado em `internal/gitlab/gitlab.go` e em
`internal/coleta/coleta.go`, da especificação
`spec_classroom_commits_do_modelo.md`. Esta mudança acrescenta um método à
mesma interface `Cliente`. Implemente depois que aquilo estiver commitado, ou
em ramo próprio, para não misturar as duas coisas no mesmo arquivo.

## Comportamento pedido

### Comando

```bash
classroom devolutiva --exercicio lab01              # ensaio: mostra o que faria
classroom devolutiva --exercicio lab01 --aplicar    # publica
classroom devolutiva --exercicio lab01 --grr GRR20259001 --aplicar
classroom devolutiva                                # lista o que já foi publicado
```

| Flag | Efeito |
|---|---|
| `--exercicio` | repetível, como nos demais comandos; sem ela, o comando só lista |
| `--grr` | repetível, restringe a um ou mais alunos |
| `--aplicar` | sem ela, nada é enviado ao GitLab. **O ensaio é o padrão** |
| `--refazer` | republica quem já tem issue, como comentário novo na issue existente |
| `--prazo` | data até quando o aluno pode comentar, escrita no corpo da issue |

### Quem entra

Um aluno entra na rodada quando, para aquele exercício, tem **nota lançada com
comentário não vazio** e **projeto conhecido** em `entregas.csv`. Os demais
aparecem no resumo do final, com o motivo: sem nota, sem comentário, sem fork.

### Corpo da issue

Título: `Devolutiva: <título do exercício>`, ou `Devolutiva: <id>` quando o
exercício não tem título.

```markdown
@<usuario>, segue a devolutiva da sua entrega.

<comentário da correção, como está em notas.csv>

Commit avaliado: <sha curto>
Comentários até <prazo>. Responda aqui mesmo se discordar de algum ponto ou
quiser entender melhor a correção.
```

A menção ao usuário no corpo é o que garante a notificação. Não atribua a issue
nem aplique etiqueta: atribuir exige o ID numérico do aluno, que custa uma
requisição a mais, e a etiqueta precisaria ser criada em cada projeto.

Quando o aluno não tem `usuario` no cadastro, publique sem a menção.

### Estado gravado

Arquivo novo `.classroom/devolutivas.csv`:

```
exercicio;grr;projeto;issue;url;publicado_em;hash
lab01;GRR20259001;ds122-2026-2-n-grr20259001/ds122-lab-01-assignment;3;https://gitlab.com/.../issues/3;2026-09-22T19:10:04-03:00;9f2a1c
```

- `issue` é o `iid`, que é o número visto na interface, e não o `id` global;
- `hash` são os primeiros seis dígitos do SHA-256 do comentário publicado. É o
  que permite dizer, na rodada seguinte, que o professor mudou a devolutiva
  depois de publicá-la;
- o arquivo segue as regras dos demais: separador `;`, coluna por nome, ordem de
  escrita estável, gravação atômica;
- **a coleta nunca toca neste arquivo**, pela mesma razão que não toca em
  `notas.csv`: um é o que o GitLab diz, o outro é o que o professor fez.

### Idempotência

Na segunda execução sobre o mesmo exercício:

- aluno com linha em `devolutivas.csv` e `hash` igual ao comentário atual fica de
  fora, e o resumo diz "já publicada";
- aluno com `hash` diferente entra na lista de **devolutiva desatualizada**, e só
  é republicado com `--refazer`, que acrescenta um comentário na issue existente
  (`POST /projects/:id/issues/:iid/notes`) em vez de abrir outra;
- aluno sem linha no arquivo, mas com issue de mesmo título já aberta no projeto,
  é reconhecido pela consulta de issues e a linha é reconstruída, sem publicar
  nada. É a defesa para o arquivo perdido ou a issue criada à mão.

## Mudanças na interface `Cliente`

```go
// Issue é uma issue do GitLab, do jeito que interessa aqui.
type Issue struct {
    IID   int64
    Titulo string
    URL   string
}

// IssuesDoProjeto lista as issues abertas de um projeto, para reconhecer a
// devolutiva já publicada sem depender do arquivo local.
IssuesDoProjeto(projeto string) ([]Issue, error)

// CriarIssue abre uma issue no projeto e devolve o que foi criado.
CriarIssue(projeto, titulo, corpo string) (Issue, error)

// ComentarIssue acrescenta um comentário a uma issue existente.
ComentarIssue(projeto string, iid int64, corpo string) error
```

`IssuesDoProjeto` entra no cache por operação, como as demais listagens.
`CriarIssue` e `ComentarIssue` são escrita e não são memorizadas.

O professor entra no grupo do aluno como `reporter`, e reporter abre e comenta
issue. Se a API recusar por permissão, o erro precisa dizer isso com todas as
letras, e não devolver "404 project not found", que é o que o gitlab.com
responde quando falta acesso.

## Testes

Nenhum teste toca o gitlab.com. O pacote de coleta já tem um cliente falso;
siga o mesmo caminho. Casos que precisam existir:

1. ensaio não chama `CriarIssue` nenhuma vez;
2. aluno com nota e comentário e fork recebe uma issue, e a linha aparece em
   `devolutivas.csv` com o `iid` devolvido;
3. segunda execução sem `--refazer` não publica nada;
4. comentário alterado depois de publicado aparece como desatualizado e só é
   republicado com `--refazer`, e aí como comentário, não como issue nova;
5. aluno sem fork, sem nota ou com comentário vazio fica de fora, e o resumo diz
   qual dos três motivos;
6. issue de mesmo título já existente no projeto reconstrói a linha sem
   publicar;
7. entrega compartilhada gera uma issue só, no fork onde a entrega está.

## O que não fazer

- não publicar nota, nem no título nem no corpo;
- não fechar a issue depois de publicar;
- não escrever em `notas.csv`, `entregas.csv` ou `verificacoes.csv`;
- não criar etiqueta nem milestone nos projetos dos alunos;
- não publicar nada sem `--aplicar`.

## Documentação a atualizar junto

- `README.md` do `gitlab-classroom`, na lista de comandos e no ciclo de
  correção;
- `CLAUDE.md` do mesmo repositório, na árvore de `.classroom/`, que passa a ter
  `devolutivas.csv`;
- a skill `correcao-ds122`, em `~/.claude/skills/correcao-ds122/SKILL.md`, na
  seção da justificativa devolvida ao aluno.
