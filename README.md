# gitlab-classroom

Coleta e acompanhamento das entregas de exercícios feitas pelos alunos no
gitlab.com, em linha de comando.

Cada aluno mantém um grupo privado com os *forks* dos repositórios de
exercício da disciplina. O `classroom` percorre esses grupos, separa os commits
do aluno dos que vieram do repositório-modelo, decide a situação da entrega
pelo prazo e guarda tudo em arquivos texto dentro da pasta da turma.

## Instalação

```bash
go install github.com/alexkutzke/gitlab-classroom/cmd/classroom@latest
```

O binário se chama `classroom`. Dentro do repositório, `go install ./cmd/classroom`.

## Token de acesso

A coleta usa a API do GitLab. Gere um token pessoal com escopo `read_api` em
<https://gitlab.com/-/user_settings/personal_access_tokens> e forneça-o de uma
destas formas, nesta ordem de prioridade:

```bash
classroom coletar --token glpat-...
export GITLAB_TOKEN=glpat-...
secret-tool store --label='GitLab classroom' service gitlab user alexkutzke
```

Também vale apontar um arquivo em `token_arquivo`, no `config.toml`. O token
nunca é gravado dentro de `.classroom/`.

```bash
classroom token    # diz de onde veio o token e testa a conexão
```

## Uso

### Criar a turma

Rode na pasta da turma, a mesma que já tem o `.diario/`:

```bash
cd ~/.../2026-02/ds122/ds122_n
classroom init
```

Disciplina, turma e semestre são lidos do `.diario/turma.toml`; o cadastro de
alunos, do `.diario/alunos.csv`, que já vem do SIGA. O turno sai do sufixo da
pasta (`_n` ou `_t`). As opções `--codigo`, `--semestre`, `--turno`,
`--namespace` e `--padrao-grupo` entram só para corrigir ou completar.

O padrão de nome do grupo de cada aluno fica em `config.toml`:

```toml
padrao_grupo = "{codigo}-{ano}-{periodo}-{turno}-{grr}"
```

que resolve para `ds122-2026-2-n-grr20249999`, o nome pedido à turma nas
instruções de submissão.

### Cadastrar os exercícios

```bash
classroom exercicios add --repo ds122-prepare-assignment --prazo 2026-08-15
classroom exercicios add --repo ds122-html-assignment --prazo 2026-09-05 \
                         --titulo "HTML e CSS" --peso 2
classroom exercicios
```

O `--repo` é o nome do projeto-modelo dentro do namespace da disciplina, que é
também o nome do fork do aluno. O apelido curto sai do nome do repositório
(`ds122-html-assignment` vira `html`); `--id` muda isso.

Exercício que saiu do cronograma vai para `classroom exercicios arquivar --id X`,
o que o tira das coletas sem apagar o que já foi coletado.

### Reconciliar as contas

```bash
classroom sync
classroom sync --dry-run
classroom sync --sem-gitlab    # só reimportar o cadastro do diario
```

Reimporta o cadastro e procura o grupo de cada aluno no GitLab. Nome, e-mail e
situação vêm do SIGA e sobrescrevem o que estava aqui; usuário, grupo e
situação da conta são apurados contra o GitLab. Ninguém é apagado: quem sai da
lista do SIGA vira `cancelado` e conserva as entregas coletadas.

O que o sync distingue, e que a mensagem única "fork não encontrado" escondia:

| Situação | O que aconteceu |
|---|---|
| `ok` | grupo com o nome do padrão, professor associado |
| `grupo_divergente` | grupo acessível, com outro nome; o relatório mostra qual |
| `sem_acesso` | grupo existe e é visível, mas sem o professor como reporter |
| `sem_conta` | nenhum usuário com o GRR no gitlab.com |
| `grupo_invisivel` | o grupo não foi criado, ou foi criado privado sem compartilhar. A API não separa os dois casos |

### Coletar as entregas

```bash
classroom coletar
classroom coletar --exercicio html --detalhado
classroom coletar --exercicio html --dry-run
```

Para cada aluno e cada exercício: localiza o fork no grupo, lista os commits do
ramo padrão, descarta os que também existem no repositório-modelo e avalia o
commit mais recente até as 23h59 do dia do prazo.

O que fica registrado por entrega: situação, commit avaliado, número de commits
do aluno, último commit (mesmo depois do prazo) e o atraso em dias. Distinguir
"não fez" de "fez depois" importa porque os dois casos pedem encaminhamentos
diferentes.

Aluno que trabalhou fora do ramo padrão aparece como `fork_sem_commit` com o
detalhe dizendo quantos commits existem em outros ramos, em vez de sumir do
relatório.

A coleta regrava as entregas dos exercícios coletados e **nunca toca nas notas
lançadas**: são arquivos separados justamente por isso.

### Consultar

```bash
classroom status              # panorama: cadastro pendente e cada exercício
classroom alunos              # o cadastro, com grupo e situação da conta
classroom alunos silva        # busca por nome, GRR ou e-mail
```

### Aluno com login fora da convenção

O login de cada aluno no GitLab é o GRR em minúsculas, e o grupo segue o
padrão do `config.toml`. Para as exceções, o aluno que não conseguiu criar a
conta com o GRR e usou outro nome, ou que batizou o grupo de um jeito que a
busca não acha:

```bash
classroom alunos editar --grr GRR20259001 --usuario ana.souza
classroom alunos editar --grr GRR20259001 --grupo ds122-noturno-ana \
                        --obs "GitLab recusou o cadastro com o GRR"
```

Com o login cadastrado, a coleta passa a procurar o grupo pelos dois nomes, o
do padrão com o GRR e o do padrão com o login, e a checagem de conta usa o
login. O grupo fixado com `--grupo` tem prioridade sobre os dois.

Nome, e-mail e situação não se editam por aqui: vêm do SIGA e voltariam no
próximo `sync`.

### Gerar a tabela de entregas

Duas saídas do mesmo comando, para públicos diferentes.

Para o professor, com o nome dos alunos:

```bash
classroom relatorio
```

Para publicar no material da disciplina, identificada por GRR e ordenada por
GRR:

```bash
classroom relatorio --identificacao grr -o ../ds122-alexkutzke/src/entregas.md
```

A versão publicada não leva nome nenhum, sai em ordem de GRR (ordenar por nome
mostrando o GRR deixaria a posição na lista denunciando quem é quem) e fecha
com a instrução de conferir o nome do grupo e a associação do professor, que é
o que o aluno precisa fazer quando a linha dele diz `sem grupo`.

O arquivo novo em `src/` só aparece no mdBook depois de entrar no
`src/SUMMARY.md`.

## Onde ficam os dados

Um diretório `.classroom/` na pasta da turma, encontrado subindo a árvore de
diretórios a partir da pasta atual, do mesmo modo que o `git` acha o `.git`.

```
ds122_n/
├── .diario/                 # do diario, fonte do cadastro de alunos
└── .classroom/
    ├── config.toml          # turma, turno, padrão do grupo, namespace dos modelos
    ├── alunos.csv           # grr;nome;email;usuario;grupo;situacao;situacao_conta;...
    ├── exercicios.csv       # id;repo;titulo;prazo;peso;verificacao;situacao
    ├── entregas.csv         # o que o GitLab diz
    └── notas.csv            # o que o professor decidiu
```

Tudo é texto, editável à mão, gravado em ordem estável para o diff continuar
legível a cada execução, e escrito de forma atômica para uma interrupção não
deixar arquivo truncado numa pasta em sincronia.

As colunas dos CSV são localizadas pelo **nome no cabeçalho**, e não pela
posição, o que permite reordená-las na edição manual e mantém legíveis os
arquivos gravados por versões anteriores.

## Privacidade

O `.classroom/` contém nome, e-mail e GRR de alunos reais. O `.gitignore` deste
repositório bloqueia `.classroom/`, `entregas/` e `*.csv` para que dado de
aluno não seja versionado por acidente; as fixtures em `internal/*/testdata`
são anonimizadas.

## Desenvolvimento

```bash
go build ./...
go test ./...
go install .
```

| Pacote | Responsabilidade |
|---|---|
| `internal/turma` | modelo de domínio, regras de prazo e classificação |
| `internal/store` | descoberta do `.classroom/`, leitura e escrita dos arquivos |
| `internal/diario` | importação do cadastro mantido pela aplicação diario |
| `internal/gitlab` | acesso à API, atrás de uma interface |
| `internal/coleta` | percorre alunos e exercícios e classifica as entregas |
| `internal/relatorio` | saídas de terminal |
| `internal/export` | tabela de entregas em markdown |
| `internal/cli` | árvore de comandos (Cobra) |

Nenhum teste toca o gitlab.com: a camada de API fica atrás da interface
`gitlab.Cliente`, dublada nos testes.

O que ainda falta, na ordem prevista: clone e `fetch` paralelos dos forks,
interface interativa de correção com lançamento de nota, e execução da suíte
de verificação automática em contêiner. O plano completo está em `CLAUDE.md`.
