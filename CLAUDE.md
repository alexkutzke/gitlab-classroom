# CLAUDE.md

Orientações para o Claude Code trabalhar neste repositório.

## O que é

`gitlab-classroom` coleta, acompanha e corrige as entregas de exercícios e
trabalhos que os alunos de DS122 (TADS/SEPT-UFPR) submetem como *fork* de
repositórios-modelo no gitlab.com. Aplicação de linha de comando em Go, com
uma interface interativa de correção em terminal.

Substitui os três shell scripts em `old/`, mantidos apenas como registro do
comportamento anterior. Não os edite: o que eles faziam está descrito em
"Herança dos scripts antigos", abaixo.

Binário: `classroom`, em `cmd/classroom/`. O `main` fica lá, e não na raiz,
porque `go install` batiza o binário com o nome do último elemento do caminho.
Módulo: `github.com/alexkutzke/gitlab-classroom`.

## Aplicação de referência

`~/work/dev/diario` é a aplicação irmã (controle de frequência) e define as
convenções desta. Ao implementar qualquer coisa aqui, leia lá o equivalente
antes. O que se repete:

- Cobra para a árvore de comandos, Bubble Tea e Lipgloss para a interface
  interativa, BurntSushi/toml para configuração, excelize para planilha.
- Dados em arquivos texto dentro de um diretório oculto na pasta da turma,
  localizado subindo a árvore de diretórios a partir do diretório atual, do
  mesmo modo que o git encontra o `.git`.
- CSV com separador `;`, colunas localizadas pelo nome no cabeçalho e não pela
  posição, arquivo ausente lido como vazio, gravação atômica (arquivo
  temporário mais rename) e ordem de escrita estável, para o diff continuar
  legível numa pasta em sincronia.
- Código, comentários, mensagens de erro e documentação em português do
  Brasil. Comentário explica por que a regra existe, não o que a linha faz.
- Mensagem de commit de uma linha, sem corpo, sem rodapé de coautoria.

## Domínio

Cada aluno cria no gitlab.com um grupo privado `ds122-<ano>-<semestre>-<turno>-<grr>`
(exemplo: `ds122-2026-2-n-grr20249999`), adiciona `alexkutzke` como `reporter`
e faz o *fork* de cada repositório-modelo de `gitlab.com/ds122-alexkutzke`
para dentro desse grupo. O procedimento visto pelo aluno está em
`ds122-alexkutzke/src/instrucoes_submissao_tarefas_e_trabalhos.md`, na pasta
da disciplina.

Consequências que o código precisa respeitar:

- O nome de usuário do aluno é o GRR em minúsculas, e o e-mail é o
  institucional. É por aí que se liga a conta do GitLab ao cadastro do SIGA.
- O grupo é do aluno, não do professor. A ferramenta enxerga apenas os grupos
  em que `alexkutzke` foi adicionado. Quem não adicionou o professor é
  indistinguível, pela API, de quem não criou o grupo, e o relatório precisa
  dizer isso em vez de afirmar que não houve entrega.
- Errar o nome do grupo é o engano mais comum da turma. A busca por padrão de
  nome tem que ter uma passagem de reserva que procure o grupo do aluno por
  qualquer nome antes de declarar ausência.
- Trabalhos em grupo têm um *fork* só, com os colegas adicionados como membros
  do projeto. Uma entrega pode corresponder a mais de um aluno.

## Estado dos dados

Diretório `.classroom/` na pasta da turma, ao lado do `.diario/` já existente
(`~/Documents/work/teaching/graduacao/2026-02/ds122/ds122_n/`, e o mesmo para
`ds122_t/`).

```
ds122_n/
├── .diario/                 # da aplicação diario, fonte do cadastro
└── .classroom/
    ├── config.toml          # turma, turno, padrão do grupo, namespace dos modelos
    ├── alunos.csv           # grr;nome;email;usuario;grupo;situacao
    ├── exercicios.csv       # id;repo;titulo;prazo;peso;verificacao;situacao
    ├── entregas.csv         # estado coletado do GitLab
    ├── notas.csv            # exercicio;grr;nota;comentario;corrigido_em
    └── verificacoes.csv     # exercicio;grr;situacao;aprovados;total;commit;...
```

Os clones ficam fora do `.classroom/`, em pasta configurável
(`entregas/<exercicio>/<grr>-<nome>/` por padrão), porque são volumosos e
descartáveis.

Regra que não pode ser quebrada: **a coleta regrava `entregas.csv` por
inteiro e nunca toca em `notas.csv` nem em `verificacoes.csv`**. Um é o que o
GitLab diz, o segundo é o que o professor decidiu, o terceiro é o que a suíte
apurou. Foi para isso que os três arquivos existem separados.

`equipes.csv` tem uma linha por integrante que **não** é dono do fork: a
equipe de uma entrega é o dono mais quem aponta para ele. A coleta regrava só
as linhas de origem `gitlab`; as de origem `manual` são do professor e nunca
são tocadas, inclusive vencendo o que a API disser sobre o mesmo aluno.

`verificacoes.csv` guarda o commit verificado. Quando ele difere do commit da
entrega corrente, o resultado está velho, e a tela de correção marca isso com
`!` em vez de fingir que o veredito ainda vale.

### Cadastro

O cadastro de alunos vem de `../.diario/alunos.csv`, que já é mantido a partir
das exportações do SIGA. Ler o arquivo direto, sem depender do código do
`diario`, tolerando colunas ausentes como o `store` do `diario` faz. O
`alunos.csv` daqui acrescenta o que é próprio do GitLab (usuário, grupo,
situação da conta) e preserva essas colunas ao reimportar.

Aluno que sai da turma vira `cancelado` e continua no arquivo com as entregas
já registradas, mesma regra do `diario`.

## Custo das consultas

Duas armadilhas já cobraram caro, e o código carrega defesa contra as duas:

- **listagem completa de grupos.** Quem dá aula há alguns semestres acumula
  centenas de grupos (mais de quinhentos em 2026/2), e `GET /groups` com
  `min_access_level` leva cerca de quatro segundos por página. A coleta usa
  `GruposComAcesso` com o prefixo da turma, que traz os grupos dela em uma
  requisição, e cai para a checagem dirigida de associação (`MembroDoGrupo`)
  no que ficou de fora;
- **busca por texto.** Tem limite próprio no gitlab.com e degrada rápido
  quando se faz uma por aluno. Só sobrou uma busca por aluno ainda não
  resolvido, para achar o grupo com nome fora do padrão.

Todo cache do cliente faz busca única por chave: sem isso, os oito
trabalhadores partem juntos e cada um busca a mesma listagem. O tempo de vida
do cache é o de **uma operação**, e não o do cliente: `acoes.Coletar` e
`acoes.Sincronizar` chamam `Cliente.Renovar()` na entrada. Na linha de comando
isso é redundante, porque o processo morre a cada comando; na TUI o cliente
dura a sessão, e sem o descarte a segunda coleta responderia do cache,
escondendo o grupo ou o commit que apareceu desde a primeira.

Ordem em que a situação da conta é apurada, que também é ordem de custo:
listagem da turma, consulta direta ao grupo, associação dirigida, busca pelo
GRR e, por último, existência da conta. A busca vem antes da conta de
propósito: o aluno que não conseguiu criar a conta com o GRR ainda pode ter um
grupo com o GRR no nome, e perguntar primeiro pela conta esconderia a entrega
dele.

Entre os nomes candidatos, os do padrão vêm antes do grupo gravado no
cadastro. O aluno que erra o nome do grupo costuma criar um grupo novo com o
nome certo em vez de mudar a URL do primeiro, e os dois passam a existir:
insistir no gravado deixaria a coleta presa no grupo abandonado. Fixar um nome
fora do padrão com `alunos editar --grupo` continua valendo, porque nesse caso
não existe grupo com o nome esperado para competir.

## Acesso ao GitLab

Duas vias, com papéis distintos:

- **API** (`gitlab.com/gitlab-org/api/client-go`) para descobrir grupos e
  *forks*, conferir a associação de `alexkutzke` como reporter, listar commits
  com data e autor, e classificar a situação da entrega. É o caminho rápido, e
  o relatório de situação não precisa de mais nada.
- **git via SSH** (`ssh://git@gitlab.com/...`, chave já configurada em
  `~/.ssh/id_rsa`) para trazer o conteúdo quando for corrigir ou verificar.
  Executar o `git` do sistema por `os/exec`, sem biblioteca git em Go.

Token: procurado nesta ordem, e nunca gravado em `.classroom/`.

1. `--token`;
2. variável de ambiente `GITLAB_TOKEN`;
3. `secret-tool lookup service gitlab user alexkutzke` (o `secret-tool` está
   instalado nesta máquina);
4. caminho indicado em `token_arquivo` no `config.toml`.

Escopo necessário: `read_api`. Erro de autenticação precisa dizer qual escopo
falta e como gerar o token, não devolver `401` cru.

Chamada de API respeita o limite de requisições do gitlab.com: pool de
trabalhadores configurável (padrão 8), repetição com espera crescente em
`429`, e nada de uma requisição por aluno quando uma listagem paginada
resolve.

## Comandos

| Comando | Papel |
|---|---|
| `classroom` | sem subcomando, abre a interface interativa |
| `classroom init` | cria o `.classroom/`, importa o cadastro e a configuração do `.diario/` |
| `classroom sync` | reimporta o cadastro e reconcilia com o GitLab (grupo, associação, usuário) |
| `classroom token` | diz de onde vem o token e testa a conexão, sem exibir o valor |
| `classroom exercicios` | lista, adiciona, edita e arquiva exercícios |
| `classroom coletar` | percorre alunos e exercícios e classifica a entrega |
| `classroom status` | panorama: cadastro pendente e situação de cada exercício |
| `classroom alunos` | o cadastro, com grupo e situação da conta |
| `classroom alunos editar` | fixa o login ou o grupo de um aluno específico |
| `classroom equipes` | entregas feitas por mais de um aluno, com `vincular` e `desvincular` |
| `classroom relatorio` | tabela de entregas em markdown, para o professor ou para o material |
| `classroom clonar` | baixa os forks e posiciona cada clone no commit avaliado |
| `classroom abrir` | abre o clone no editor, ou o projeto no navegador |
| `classroom verificar` | roda a suíte do exercício sobre os clones, em contêiner |
| `classroom corrigir` | interface interativa de correção |
| `classroom nota` | lança ou apaga uma nota isolada |
| `classroom notas` | planilha de notas por exercício e média |

## Dois requisitos de origem

Pedidos do professor depois do plano inicial, ambos já no código.

**Relatório publicável.** `classroom relatorio --identificacao grr` produz a
tabela que vai para o material da disciplina: uma linha por aluno identificada
só pelo GRR, uma coluna por exercício, ordenada por GRR. A ordenação por nome
foi descartada de propósito na versão publicada, porque a posição na lista
denunciaria quem é quem. A saída termina com a instrução de conferir o nome do
grupo e a associação do professor, que é o que o aluno tem a fazer quando a
linha dele acusa `sem grupo`. O destino habitual é `src/` do mdBook, e o
arquivo precisa entrar no `src/SUMMARY.md` para aparecer no livro.

**Login fora da convenção.** O GRR em minúsculas é a convenção, não uma
garantia: alguns alunos não conseguem criar a conta com ele. A coluna `usuario`
de `alunos.csv` guarda o login real, e `classroom alunos editar --grr X
--usuario Y` a preenche. A partir daí a resolução do grupo tenta o padrão com
o GRR e o padrão com o login, a busca entre os grupos do professor procura os
dois, e a checagem de existência da conta usa o login. `--grupo` fixa um
caminho que ganha de tudo isso, para o grupo com nome que a busca não alcança.

Mudar login ou grupo zera a situação da conta: o que tinha sido apurado antes
deixa de valer e o próximo `sync` confere de novo.

## Entregas em dupla

As tarefas podem ser feitas em dupla, e aí existe um fork só, no grupo de um
dos dois, com o colega adicionado como membro do projeto (nunca do grupo). A
coleta tem uma fase própria para isso, entre resolver os grupos e classificar
as entregas: percorre os forks que os alunos têm nos próprios grupos, lê os
membros de cada um e casa com o cadastro.

Regras que o código precisa manter:

- fork próprio vence participação no fork alheio. Quem bifurcou é avaliado
  pelo que está no grupo dele;
- a varredura é pelos grupos da turma, nunca pelos forks do repositório
  modelo. O modelo acumula os forks de todos os semestres (mais de quatrocentos
  em 2026/2) e listá-los custava quarenta e cinco segundos para descartar quase
  tudo;
- falha na descoberta não interrompe a coleta. Sem ela, cada aluno é avaliado
  pelo próprio grupo, que era o comportamento anterior à fase 5;
- membro que não está no cadastro da turma é ignorado, o que já descarta o
  professor e eventuais monitores;
- aluno sem grupo visível ainda pode ter entregado no fork do colega, então o
  problema de conta não encerra a busca;
- um fork, um clone. O integrante que não é dono aponta para a pasta do dono,
  e a suíte de verificação roda uma vez, com o resultado gravado por aluno.

A nota continua sendo por aluno, porque é ela que entra na média, mas lançar a
de um integrante lança a dos demais por padrão, na interface e no comando
avulso. `D` e `--so-este` desligam isso.

## Interface interativa

`internal/tui`, em Bubble Tea. Três regras estruturais:

- **a TUI não tem lógica de domínio.** Toda operação passa por
  `internal/acoes`, o mesmo pacote que os subcomandos usam. Se uma regra
  precisar mudar, muda lá e os dois caminhos acompanham;
- **o que demora roda em goroutine** e conversa com o laço de eventos por um
  canal. O progresso é descartável (um `select` com `default`), porque perder
  um quadro é melhor que travar o trabalho. Enquanto a operação roda, só
  cancelar e sair valem;
- **cancelar não grava.** As três operações longas recebem `context.Context`;
  na interrupção, `acoes` devolve erro e nada é aplicado, senão a coleta pela
  metade apagaria a entrega de quem não foi visitado.

Telas: painel (pendências e resumo por exercício), exercício (a turma linha a
linha, de onde partem coleta, clone, verificação e correção), exercícios
(cadastro e edição), alunos, equipes, tarefas (registro das operações da
sessão) e ajuda.

A correção é a mesma `internal/correcao` do subcomando, embutida: o modelo
ganhou `Sessao`, e o campo `autonomo` decide se sair encerra o programa ou
devolve o controle à aplicação.

Os testes dirigem o modelo por mensagens de tecla e inspecionam a saída de
`View`, sem terminal e sem rede; as operações longas são exercitadas com um
cliente de GitLab dublado e um laço de eventos escrito à mão no teste.

## Classificação da entrega

Vocabulário fechado, gravado em `entregas.csv` e definido em
`internal/turma/turma.go`:

`sem_conta`, `sem_acesso`, `grupo_invisivel`, `sem_fork`, `fork_sem_commit`,
`sem_commit_no_prazo`, `entregue`, `erro`.

Duas escolhas que valem manter:

- `grupo_invisivel` cobre o caso genuinamente ambíguo. Grupo privado não
  compartilhado e grupo inexistente devolvem o mesmo 404, e o rótulo diz isso
  em vez de fingir que sabe qual dos dois é. O que dá para separar está
  separado: `sem_conta` vem da consulta ao usuário, `sem_acesso` do grupo que
  aparece sem o professor associado.
- `erro` é falha de rede ou de API, e não veredito sobre o aluno. Recoletar
  resolve, e por isso ele não se mistura às demais situações.
- `sem_acesso` bloqueia a entrega só quando o grupo também não abre. O grupo
  sem a associação do professor costuma continuar legível, porque o
  repositório é fork de um modelo da disciplina, e nesse caso a entrega conta:
  a falta do convite fica como pendência de cadastro, e não como entrega
  perdida.

Regras:

- O prazo é uma data. Vale até 23h59min59s daquele dia, no fuso local.
- A entrega é o commit mais recente até o prazo. O último commit absoluto e o
  atraso em dias ficam registrados também, porque "não fez" e "fez depois"
  pedem encaminhamentos diferentes.
- Commit do aluno é o que não existe no repositório-modelo, apurado por
  comparação de SHA. Não usar filtro por nome do autor: o script antigo
  descartava commits com `grep -v Kutzke` e errava tanto com o aluno que
  configurou o git com outro nome quanto com o modelo que recebeu commit de
  terceiro.
- Fork sem commit no ramo padrão é conferido de novo com todos os ramos antes
  de virar `fork_sem_commit`, e o número de commits fora do ramo entra no
  detalhe. Quem trabalhou numa branch não some do relatório.
- O clone da entrega (fase 2) não pode ficar em `HEAD` destacado silencioso:
  marcar o commit com uma tag local `entrega/<exercicio>` e registrar o SHA.

## Verificação automática

Opcional por exercício, porque nem todo enunciado é testável. A coluna
`verificacao` de `exercicios.csv` guarda o comando, executado com a raiz do
repositório como diretório de trabalho; vazia, o exercício é corrigido só à
mão e a verificação sai como `sem_suite`, sem prejuízo nenhum.

A coluna `imagem` diz em que contêiner a suíte roda; vazia, vale
`imagem_verificacao` do `config.toml`.

Executar código de aluno é o ponto de risco desta aplicação. O padrão é
`podman run` (ou `docker`, na falta dele) com `--network=none`, o clone
montado `ro`, `--security-opt=no-new-privileges`, `--memory`, `--pids-limit`,
`--cpus=1`, `/tmp` em tmpfs e tempo limite. `--escrita` troca a montagem para
`rw`; `--sem-sandbox` roda direto na máquina e exige um `sim` digitado, ou
`--sim` fora de terminal.

Contagem de casos: a suíte pode imprimir `RESULTADO: 7/10`, e a última
ocorrência é a que vale. Sem essa linha, decide o código de saída. Os códigos
125, 126 e 127 vindos do runtime viram `erro`, e não `reprovado`: significam
que o contêiner nem chegou a executar a suíte, o que é problema do exercício e
não do aluno.

Saída completa em `entregas/<exercicio>/.logs/<grr>.log`. No CSV entra o
resumo: veredito, aprovados, total, commit verificado e duração.

Este item liga o plano de ensino ao código: quem declara uso de IA generativa
no trabalho prático é avaliado por suíte automatizada com teto de nota
reduzido. Antes de mexer nos pesos, conferir o valor vigente no
`plano_ensino_2026_2.md` da disciplina.

## Interface de correção

`internal/correcao`, em Bubble Tea, no mesmo espírito da `chamada` do
`diario`: lista, cursor, filtro por nome, gravação ao sair com `enter` e saída
sem gravar com `q`.

O que é próprio daqui:

- cada linha mostra a situação da entrega, o atraso e o resultado da suíte,
  que é o contexto para decidir a nota;
- dígito começa a lançar a nota direto, sem passar por comando;
- `r` repete a última nota lançada com o comentário dela, que encurta a
  correção de uma turma inteira com o mesmo veredito;
- `o` abre o clone no `$EDITOR` via `tea.ExecProcess`, devolvendo o terminal à
  interface quando o editor fecha;
- só o que mudou na sessão é gravado, e nota apagada vira remoção explícita no
  arquivo.

O comentário é o texto devolvido ao aluno. Comentário sem nota não é gravado,
e a interface avisa isso em vez de perder o que foi digitado.

## Fases de implementação

As seis estão feitas.

1. `store`, modelo, `init`, `sync`, `exercicios`, `token`, `coletar` por API,
   `status`, `alunos` e `relatorio` em markdown. Substituiu os scripts de `old/`.
2. `clonar` e `abrir`: clone e `fetch` paralelos, clone posicionado no commit
   avaliado sob o ramo local `entrega/<exercicio>`.
3. `corrigir`, `nota` e `notas`: correção interativa, comentário devolvido ao
   aluno e planilha com média ponderada.
4. `verificar`: suíte automatizada em contêiner sem rede.
5. `equipes`: entregas em dupla descobertas pelos membros do fork, com
   cadastro manual, clone único e nota propagada.
6. `internal/tui`: interface interativa, com `internal/acoes` extraído para
   ser o caminho único das operações.

Cada fase entra com teste. Seguir o padrão do `diario`: testes de tabela sobre
os pacotes de domínio e de leitura de arquivo, com fixtures anonimizadas em
`internal/*/testdata`. Não há teste que fale com o gitlab.com; a camada de API
fica atrás de uma interface, dublada nos testes.

## Privacidade

Nome, e-mail, GRR e código de aluno não entram no repositório da ferramenta. O
`.gitignore` bloqueia `.classroom/`, a pasta de clones, `*.csv` e `*.xlsx`,
liberando explicitamente `internal/*/testdata/*.csv`, que é material
anonimizado. Mesma postura do `diario`.

## Herança dos scripts antigos

`old/ds122_check_all_exercises.sh`, `old/ds122_clone_exercise.sh` e
`old/ds122_create_exercises_output.sh` faziam, em série: ler um arquivo
`grr nome` e outro `exercicio prazo`, clonar cada *fork* por SSH, dar
`checkout` no último commit até o prazo, decidir a situação por `grep` no
nome do autor, e colar as colunas com `paste` numa tabela markdown.

O que a nova aplicação mantém: o formato de tabela markdown do relatório, o
recorte da entrega pelo prazo e a organização por exercício.

O que corrige: diagnóstico único para causas diferentes (grupo, associação e
*fork* eram todos "Fork não encontrado"), identificação do commit do aluno
por nome do autor, `HEAD` destacado deixado no clone, execução em série, e
cadastro de alunos digitado à mão.

## Desenvolvimento

```bash
go build ./...
go test ./...
go install ./cmd/classroom
```
