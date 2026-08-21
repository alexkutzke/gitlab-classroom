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

## A interface interativa

`classroom`, sem subcomando, abre a interface de terminal. Os subcomandos
continuam valendo e são o caminho para scripts; fora de um terminal, o binário
sem argumento imprime a ajuda em vez de tentar desenhar tela.

```bash
cd ~/.../2026-02/ds122/ds122_n
classroom
```

A tela inicial é o painel: o que falta fazer e como anda cada exercício.
`enter` abre um exercício e mostra a turma linha a linha, com situação da
entrega, commits, resultado da suíte, marca de entrega em dupla e a nota.

| Tecla | Onde | Ação |
|---|---|---|
| `j` `k`, setas, `g` `G`, `pgup` `pgdown` | todas | navega |
| `enter` | painel, exercícios | abre |
| `esc` `q` | todas | volta; no painel, sai |
| `p` `x` `a` `e` `t` `?` | todas | painel, exercícios, alunos, equipes, tarefas, ajuda |
| `r` | todas | relê os arquivos do disco |
| `C` `S` | todas | coleta todos os exercícios; sincroniza o cadastro |
| `c` `l` `v` | exercício | coleta, clona, verifica |
| `n` `N` | exercício | corrige; só quem ainda não tem nota |
| `o` `w` | exercício | abre o clone no `$EDITOR`; abre o projeto no GitLab |
| `V` `X` | exercício | vincula e desvincula entrega em dupla |
| `s` `/` | exercício | ordem (nome, situação, nota) e filtro |
| `n` `T` `D` `P` `V` `I` `A` `z` | exercícios | cadastra, edita campos, arquiva, mostra arquivados |
| `d` | equipes | desfaz o vínculo |
| `E` | painel | exporta o relatório e a planilha de notas |

As operações de rede rodam em segundo plano, com barra de progresso, e `esc`
cancela. Coleta cancelada não grava nada: aplicar o que veio pela metade
apagaria a entrega de quem não chegou a ser visitado. O que cada operação
apurou fica na tela de tarefas, inclusive os erros por aluno, que de outro
modo sumiriam com a barra de progresso.

A tela de correção é a mesma do `classroom corrigir`, embutida como subtela:
as teclas e as regras de nota são idênticas.

Ação difícil de desfazer pede confirmação: arquivar exercício e desfazer
vínculo de equipe.

## Uso pela linha de comando

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

### Baixar os forks para corrigir

```bash
classroom clonar --exercicio html
classroom clonar --exercicio html --so-entregues
```

Clona, ou atualiza se já existir, o fork de cada aluno e deixa o clone no
commit avaliado, sob o ramo local `entrega/<exercicio>`. Um ramo, e não um
checkout solto, porque HEAD destacado silencioso era o defeito do script
antigo: quem abrisse a pasta depois não sabia em que ponto do histórico estava
olhando.

Os clones ficam em `entregas/<exercicio>/<grr>-<nome>/`, fora do
`.classroom/`. A autenticação é por SSH, com a chave que o git já usa.

```bash
classroom abrir --exercicio html --grr GRR20259001          # no $EDITOR
classroom abrir --exercicio html --grr GRR20259001 --web    # no GitLab
```

### Corrigir e lançar nota

```bash
classroom corrigir --exercicio html
classroom corrigir --exercicio html --sem-nota
```

Abre a lista da turma com a situação apurada pela coleta, o resultado da
verificação automática, se houver, e a nota já lançada. A escala vai de 0 até
`nota_maxima` do `config.toml`, que o `init` deixa em 100.

| Tecla | Ação |
|---|---|
| `j` `k` ou setas | move o cursor |
| `D` | liga ou desliga repetir a nota nos integrantes da mesma entrega |
| `0`-`9` | começa a digitar a nota do aluno sob o cursor |
| `n` | edita a nota |
| `c` | edita o comentário devolvido ao aluno |
| `r` | repete a última nota lançada, com o comentário dela |
| `x` | apaga a nota |
| `o` | abre o clone no `$EDITOR` |
| `/` | filtra por nome |
| `enter` | grava |
| `q` `esc` | sai sem gravar |

Refazer a correção carrega o que estava gravado, em vez de duplicar. Para uma
nota isolada, sem abrir a interface:

```bash
classroom nota --exercicio html --grr GRR20259001 --valor 90 --comentario "faltou o rodapé"
classroom nota --exercicio html --grr GRR20259001 --remover
```

### Planilha de notas

```bash
classroom notas
classroom notas --somente-lancadas
classroom notas --csv -o -
```

Uma coluna por exercício e a média ponderada pelos pesos. Exercício com prazo
vencido e sem nota conta zero na média, porque quem não entregou tirou zero;
exercício com prazo em aberto fica de fora até vencer. Com
`--somente-lancadas`, a média considera apenas o que já foi corrigido, que é a
leitura útil no meio do semestre. Célula vazia é exercício sem nota lançada,
que continua sendo diferente de nota zero.

### Verificação automática

Opcional, e por exercício, porque nem todo enunciado é testável.

```bash
classroom exercicios editar --id html \
    --verificacao './verifica.sh' --imagem docker.io/library/node:22-alpine
classroom verificar --exercicio html
classroom verificar --exercicio html --grr GRR20259001
```

O comando roda dentro do clone de cada aluno, em contêiner (`podman`, ou
`docker` na falta dele) **sem rede**, com o repositório montado somente para
leitura, `no-new-privileges`, teto de memória, de processos e de tempo. Rodar
código de aluno é o ponto de risco desta ferramenta, e `--sem-sandbox` executa
direto na máquina apenas depois de um `sim` digitado.

A suíte informa quantos casos passaram imprimindo uma linha
`RESULTADO: 7/10`; sem ela, vale o código de saída, onde zero aprova. A saída
completa de cada execução fica em `entregas/<exercicio>/.logs/<grr>.log`, e no
`verificacoes.csv` entra só o resumo, junto do commit verificado. Resultado
apurado sobre um commit diferente do da entrega atual aparece marcado com `!`
na tela de correção.

Exercício sem suíte cadastrada fica como `sem_suite` e não sofre nada por
isso: significa que a correção é toda à mão.

### Entregas em dupla

Nas tarefas em dupla só um dos integrantes bifurca, no grupo dele, e adiciona
o colega como membro do projeto. Procurar apenas no grupo de cada aluno
marcaria o colega como quem não entregou, o que é falso e chega até ele pelo
relatório publicado.

A coleta resolve isso sozinha: percorre os forks que os alunos têm nos próprios
grupos, lê os membros de cada um e, para o aluno sem fork, procura um fork onde
ele foi adicionado. O resultado fica em `equipes.csv`.

```bash
classroom equipes
classroom equipes vincular --exercicio html --grr GRR20259002 --dono GRR20259001
classroom equipes desvincular --exercicio html --grr GRR20259002
```

O `vincular` cobre a dupla que trabalhou junto sem adicionar o colega ao
projeto, caso em que não há o que descobrir. Vínculo cadastrado à mão não é
apagado pela coleta e vence o que a API disser sobre o mesmo aluno, pela mesma
razão que separa `entregas.csv` de `notas.csv`.

Quem tem fork próprio é avaliado por ele, mesmo sendo membro do fork de outro:
duas entregas separadas continuam sendo duas entregas.

O que muda no resto da ferramenta quando a entrega é de dois:

- **clone**: um fork, um clone. O colega aponta para a pasta do dono em vez de
  ganhar uma cópia;
- **verificação**: a suíte roda uma vez e o resultado é gravado para cada
  integrante, então o relatório continua tendo uma linha por aluno;
- **nota**: lançar a nota de um integrante lança a dos demais, tanto na
  interface quanto em `classroom nota`. A tecla `D` na interface, e
  `--so-este` no comando, desligam isso quando a intenção for avaliar alguém à
  parte.

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
    ├── notas.csv            # o que o professor decidiu
    ├── verificacoes.csv     # o que a suíte automatizada apurou
    └── equipes.csv          # quem entregou no fork de quem
```

Os clones dos forks ficam fora do `.classroom/`, em
`entregas/<exercicio>/<grr>-<nome>/`, porque são volumosos e descartáveis.

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
| `internal/repo` | clones locais dos forks, com o git do sistema |
| `internal/acoes` | as operações de estado, compartilhadas pela TUI e pelo CLI |
| `internal/tui` | interface interativa (Bubble Tea) |
| `internal/correcao` | tela de correção, usada solta e embutida na TUI |
| `internal/verificacao` | execução da suíte automatizada em contêiner |
| `internal/relatorio` | saídas de terminal |
| `internal/export` | tabela de entregas em markdown e planilha de notas |
| `internal/cli` | árvore de comandos (Cobra) |

Nenhum teste toca o gitlab.com: a camada de API fica atrás da interface
`gitlab.Cliente`, dublada nos testes. Os testes de clone usam repositórios
locais criados na hora, e os de verificação rodam sem contêiner, para não
depender de imagem baixada.
