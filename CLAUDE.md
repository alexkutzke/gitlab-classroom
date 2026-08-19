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

Binário: `classroom`. Módulo: `github.com/alexkutzke/gitlab-classroom`.

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
    └── notas.csv            # exercicio;grr;nota;comentario;corrigido_em
```

Os clones ficam fora do `.classroom/`, em pasta configurável
(`entregas/<exercicio>/<grr>-<nome>/` por padrão), porque são volumosos e
descartáveis.

Regra que não pode ser quebrada: **a coleta regrava `entregas.csv` por
inteiro e nunca toca em `notas.csv`**. Um é o que o GitLab diz, o outro é o
que o professor decidiu. Foi para isso que os dois arquivos existem separados.

### Cadastro

O cadastro de alunos vem de `../.diario/alunos.csv`, que já é mantido a partir
das exportações do SIGA. Ler o arquivo direto, sem depender do código do
`diario`, tolerando colunas ausentes como o `store` do `diario` faz. O
`alunos.csv` daqui acrescenta o que é próprio do GitLab (usuário, grupo,
situação da conta) e preserva essas colunas ao reimportar.

Aluno que sai da turma vira `cancelado` e continua no arquivo com as entregas
já registradas, mesma regra do `diario`.

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

## Comandos previstos

| Comando | Papel |
|---|---|
| `classroom init` | cria o `.classroom/`, importa o cadastro do `.diario/` |
| `classroom sync` | reimporta o cadastro e reconcilia com o GitLab (grupo, associação, usuário) |
| `classroom exercicios` | lista, adiciona e edita exercícios (repo-modelo, prazo, peso, verificação) |
| `classroom coletar` | percorre alunos e exercícios, classifica a entrega, opcionalmente clona |
| `classroom verificar` | roda a suíte do exercício sobre os clones e grava o resultado |
| `classroom corrigir` | interface interativa de correção, lança nota e comentário |
| `classroom status` | panorama: quem não criou grupo, quem não entregou, prazos próximos |
| `classroom relatorio` | tabela de entregas em markdown, para publicar no material |
| `classroom notas` | planilha de notas por exercício e média |

## Classificação da entrega

Vocabulário fechado, gravado em `entregas.csv`:

`sem_grupo`, `sem_acesso`, `sem_fork`, `fork_sem_commit`, `entregue`,
`entregue_com_atraso`, `nao_avaliavel`.

Regras:

- O prazo é uma data. Vale até 23h59min59s daquele dia, no fuso local.
- A entrega é o commit mais recente até o prazo. Registrar também o último
  commit absoluto e o atraso em dias, porque "não fez" e "fez depois" pedem
  encaminhamentos diferentes.
- Commit do aluno é o que não veio do repositório-modelo, apurado por
  comparação com os commits do modelo (`git log modelo/main..HEAD`, ou o
  ponto de bifurcação pela API). Não usar filtro por nome do autor: o script
  antigo descartava commits com `grep -v Kutzke` e errava tanto com o aluno
  que configurou o git com o nome errado quanto com o modelo que recebeu
  commit de terceiro.
- O clone da entrega não fica em `HEAD` destacado silencioso. Marcar o commit
  da entrega com uma tag local `entrega/<exercicio>` e registrar o SHA no CSV.

## Verificação automática

Opcional por exercício, porque nem todo enunciado é testável. A coluna
`verificacao` de `exercicios.csv` aponta um comando do repositório-modelo
(por exemplo `verifica/run.sh`); vazia, o exercício é corrigido só à mão e o
relatório o mostra como `nao_avaliavel` na coluna de verificação, sem
prejudicar a nota.

Executar código de aluno é o ponto de risco desta aplicação. O padrão é rodar
em contêiner (`podman`, presente na máquina) com rede desligada, sistema de
arquivos somente leitura fora do diretório de trabalho, limite de tempo e de
memória. `--sem-sandbox` existe, exige confirmação e não é o padrão.

Saída completa em `entregas/<exercicio>/<grr>/.verificacao.log`; no CSV entra
apenas o resumo (aprovados, total, tempo).

Este item liga o plano de ensino ao código: quem declara uso de IA generativa
no trabalho prático é avaliado por suíte automatizada com teto de nota
reduzido. Antes de mexer nos pesos, conferir o valor vigente no
`plano_ensino_2026_2.md` da disciplina.

## Interface de correção

Espelha a `chamada` do `diario`: lista de alunos, cursor, filtro por nome,
gravação ao sair. Diferenças próprias:

- Mostra a situação da entrega, o atraso e o resultado da verificação ao lado
  do nome.
- Abre o clone do aluno no `$EDITOR` ou no navegador (`xdg-open`) sem perder
  o estado da correção.
- Lança nota (escala configurável) e comentário. O comentário é o texto que
  volta ao aluno, então precisa poder ser reaproveitado entre alunos.
- Refazer a correção carrega o que já estava gravado, em vez de duplicar.

## Fases de implementação

1. `store`, modelo, `init`, `sync`, `exercicios`, `coletar` só por API e
   `relatorio` em markdown. Ao fim desta fase os scripts em `old/` estão
   substituídos.
2. Clone e `fetch` paralelos, tag de entrega, abertura do repositório local.
3. `corrigir`, `notas.csv` e exportação da planilha.
4. `verificar` com sandbox.

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
go install .
```
