# Especificação: interface em painéis no `classroom`

Destino: este repositório (`~/Documents/work/dev/gitlab-classroom`). Escrita
para ser implementada por outra sessão, sem contexto prévio desta.

Escrita em 07/10/2026, junto com a de mesmo nome no `diario`
(`docs/spec_tui_paineis.md`). As duas pedem a mesma linguagem visual que o
`painel` (`~/Documents/work/dev/painel`) já usa desde o commit `10224c0`, para
as três ferramentas parecerem uma só.

## Problema

A interface do `classroom` já é a mais completa das três: tela inicial com
pendências e exercícios, entregas por exercício, catálogo de exercícios,
alunos, equipes, tarefas em segundo plano e a tela de correção. Mas cada tela
é texto corrido: título em negrito, uma tabela e a linha de atalhos no fim.
O que está selecionado e o detalhe dele nunca aparecem lado a lado. Para ver
o comentário inteiro de uma correção, o commit avaliado e a devolutiva de um
aluno, é preciso trocar de tela ou ler o rodapé cortado.

O `painel` passou a usar painéis com borda, foco, detalhe que segue o cursor e
cores por sentido, no estilo do lazygit, e o professor quer o mesmo aqui e no
`diario`.

## O que muda e o que não muda

Muda o desenho de todas as telas, que passam a ser compostas de painéis.

Não muda:

- nenhuma operação. Tudo continua vindo de `internal/acoes`, como diz o
  comentário de `internal/tui/tui.go`; a interface só navega e desenha;
- as teclas que já existem, globais e de cada tela. Elas estão na mão do
  professor durante a correção;
- as tarefas longas em goroutine, com progresso e cancelamento
  (`internal/tui/tarefa.go`);
- a confirmação antes de operação de rede ou de escrita em lote;
- `classroom` sem subcomando abre a interface, e os subcomandos continuam
  valendo para script. O `painel` abre `classroom` por `tea.ExecProcess` na
  pasta da turma, e isso tem que continuar funcionando.

## Linguagem visual comum

Esta seção é igual nas especificações do `diario` e do `classroom`. A
implementação de referência está no `painel`:
`internal/tui/moldura.go` (borda, tabela, seleção, preenchimento) e
`internal/tui/view.go` (disposição, barra de título, barra de teclas, ajuda).

### Elementos

```
 diario  DS143 TADS4N · ESTRUTURAS DE DADOS II · 2026-02 · semana 10 de 17
╭─[1] Aulas──────────────────────────────╮╭─[4] Aula de 30/09 (qua)──────────────────────────╮
│ data   dia  situação   chamada  horas   ││ tema   Árvores AVL                               │
│ 23/09  qua  realizada  ✓        2       ││ ...                                              │
╰────────────────────────────── 9 de 17 ─╯╰──────────────────────────────────────────────────╯
 tab painel   j/k move   c chamada   ? ajuda   q sai
```

- **Barra de título**, na primeira linha: um bloco `estMarca` com o nome da
  ferramenta (" diario ", " classroom ") e, depois dele, sobre fundo
  discreto, o contexto separado por " · ".
- **Painéis** com borda arredondada (`╭╮╰╯`) e o título na borda de cima, no
  formato `[n] Título`. O número é a tecla que leva o foco ao painel. A borda
  de baixo leva, à direita, a posição na lista ("9 de 17") ou "rola" quando o
  conteúdo não cabe.
- **Foco**: um painel por vez. Borda e título do painel em foco em azul
  (cor 39); os demais em cinza. `tab` e `shift+tab` passam de painel em
  painel; os números vão direto.
- **Seleção**: a linha selecionada é pintada de ponta a ponta, sem as cores
  das células. Fundo forte no painel em foco, discreto nos outros, para se ver
  o que o detalhe está mostrando mesmo com o foco em outro lugar.
- **Detalhe que segue o cursor**: mover a seleção numa lista troca o painel
  de detalhe na hora, sem `Enter`.
- **Barra de teclas**, na última linha: tecla em azul e negrito, rótulo em
  cinza, separados por três espaços. Muda com o painel em foco e com o que
  está selecionado. Mensagem de estado (erro, aviso, pergunta de confirmação)
  ocupa essa linha até a próxima tecla.
- **Ajuda**: `?` abre uma caixa centralizada com todas as teclas, agrupadas
  por tela; qualquer tecla fecha, e a tecla que fecha não age por baixo.
- **Tela estreita**: abaixo de 100 colunas, os painéis se empilham e o
  detalhe ocupa o corpo inteiro quando está em foco. Abaixo de 40 por 12, a
  tela pede para aumentar o terminal.

### Cores

As mesmas da paleta de `internal/tui` de hoje, que já é a do `painel` e do
`classroom`, com o papel de cada uma:

| Papel | Cor | Uso |
|---|---|---|
| acento | 39 | foco, títulos de seção, teclas |
| ok | 35 | presente, em dia, aprovado |
| atenção | 214 | justificada, em risco, pendente, aviso |
| erro | 203 | falta, reprovado, atrasado, erro |
| fraco | adaptativa 243/245 | rótulos, valores secundários |
| borda | adaptativa 238/250 | borda dos painéis sem foco |

Cor pelo sentido do valor, nunca por coluna: a mesma coluna "situação" é
verde numa linha e vermelha na outra. Fundo de seleção e de barras usa
`lipgloss.AdaptiveColor`, porque o cinza que se lê em terminal escuro some no
claro.

Símbolos, sempre com o mesmo sentido nas três ferramentas: `✓` em dia, `✖`
crítico ou falta, `▲` atenção, `●` pendente, `·` nada a registrar, `○` item
de lista em aberto.

### Código

Os pacotes `internal/` não se importam entre módulos, e as três ferramentas
não têm módulo comum. A decisão é copiar `moldura.go` do `painel` para
o pacote de interface daqui, com os nomes e o comportamento iguais, e um comentário
no topo dizendo de onde veio. Duzentas linhas copiadas custam menos que um
quarto repositório; se as cópias começarem a divergir, extrair um módulo
passa a valer a pena.

### Teste de tamanho

Toda tela, em todo painel em foco, ocupa o terminal exato: tantas linhas
quanto a altura e cada linha com a largura exata, medida por
`ansi.StringWidth`. Linha com uma coluna a mais quebra no terminal e
desalinha todas as bordas abaixo dela. O `painel` testa isso em
`TestTelaOcupaOTerminalExato`, em 140x40, 100x30, 99x30, 80x24, 60x20 e
40x12; o mesmo teste, com os mesmos tamanhos, entra aqui para cada tela.

## Onde fica no `classroom`

`internal/tui/estilos.go` já tem a paleta e a função `janela`; a moldura entra
ao lado, e `internal/correcao` passa a usá-la também. A correção é pacote à
parte e não importa `internal/tui` hoje; a moldura vai para um pacote novo,
`internal/moldura`, que os dois importam, para não criar dependência da
correção com o resto da interface.

## Telas e abas

As telas de hoje continuam sendo telas, com as mesmas teclas globais. O que
muda é que o nome delas aparece como abas na barra de título, com a atual
destacada, como as abas do lazygit:

```
 classroom  p Painel  x Exercícios  a Alunos  e Equipes  t Tarefas   DS122 TADSN2A · 2026-02   coletando 12/31
```

- A aba atual em azul e negrito; as outras em cinza, com a tecla na frente.
- À direita, o contexto da turma e, quando há tarefa em curso, o progresso
  dela ("coletando 12/31", "verificando 3/30"). O progresso fica à vista em
  qualquer tela, e não só na de tarefas.
- A correção e as entregas de um exercício não são abas: abrem a partir do
  painel e voltam com `esc`, como hoje.

## Painel (tela inicial)

```
 classroom  p Painel  x Exercícios  a Alunos  e Equipes  t Tarefas   DS122 TADSN2A · 2026-02
╭─[1] Exercícios──────────────────────────────────────────────────╮╭─[2] html · HTML e formulários─────────╮
│ id         prazo  entregues  atras  sem  corrig  a corrig  verif ││ prazo       05/09, venceu há 32 dias  │
│ prepare    01/09         32      0    0      32         0      ✓ ││ peso        1, categoria exercicio    │
│ html       05/09         25      3    2      20         5   ▲ 1  ││ suíte       html-check, 28 aprovadas  │
│ js         04/10         32      0    0       0        32      · ││                                       │
│ dom        08/10          -      -    -       -         -      · ││ ENTREGAS                              │
╰────────────────────────────────────────────────────── 2 de 8 ─╯│ ✓ entregue             25             │
╭─[3] Pendências (4)──────────────────────────────────────────────╮│ ▲ só depois do prazo    3             │
│ ▲ 3 alunos sem grupo em ordem no GitLab                         ││ ✖ sem fork              2             │
│ ▲ html: 5 entregas sem nota                                     ││ ...                                   │
╰─────────────────────────────────────────────────────────────────╯╰───────────────────────────────────────╯
 enter entregas   n corrige   c coleta   E exporta   S sincroniza   C coleta tudo   ? ajuda   q sai
```

**[1] Exercícios**: os de `Panorama.Exercicios`, em ordem de prazo, com as
contagens que `classroom resumo` publica. Cores pelo sentido: entregas sem
nota em laranja; exercício vencido e nunca coletado em vermelho; verificação
feita sobre commit antigo com `▲` e a contagem; exercício ainda no prazo em
fraco. Com mais de uma categoria (`Panorama.VariasCategorias`), uma coluna de
categoria.

**[2] Detalhe do exercício**: prazo e quanto falta ou passou, peso,
categoria, repositório, suíte e imagem, a contagem por situação de entrega
(`ResumoExercicio.Situacoes`) uma por linha com o símbolo e a cor da
situação, correção (notas lançadas, sem nota), verificações por resultado,
entregas compartilhadas e devolutivas pendentes.

**[3] Pendências**: `Panorama.Pendencias()`, na mesma ordem e com o mesmo
texto de hoje, uma por linha com `▲`. Sem pendência, "✓ nada pendente" em
verde. `Enter` numa pendência de exercício seleciona o exercício em [1].

**[4] Tarefa**: só aparece com tarefa em curso ou terminada há pouco, embaixo,
na largura toda, com as últimas linhas do registro dela. `t` continua levando
à tela de tarefas completa.

Disposição em 100 colunas ou mais: [1] e [3] empilhados à esquerda, com a
largura da tabela de exercícios; [2] à direita; [4] embaixo quando existe.

## Entregas de um exercício

```
╭─[1] Entregas · html (32)─────────────────────────────────────────╮╭─[2] ALUNA FICTICIA─────────────────────────╮
│   aluno                situação         commits  atraso  verif  nota ││ GRR20990002                                │
│   ALUNA FICTICIA       entregue               7       -   ✓ 8/8   95 ││ fork   ds122-2026-2-n-grr20990002/html     │
│ d ALUNO FICTICIO       entregue (+2d)         4      2d   ▲ 6/8    - ││ commit avaliado  a1b2c3d, 04/09 22:10      │
│   OUTRO FICTICIO       sem fork               -       -       -    - ││ último commit    e4f5a6b, 06/09 10:02      │
╰──────────────────────────────────────────────────────── 1 de 32 ─╯│                                            │
                                                                    │ CORREÇÃO                                   │
                                                                    │ nota 95                                    │
                                                                    │ comentário, inteiro, quebrado na largura   │
                                                                    │                                            │
                                                                    │ DEVOLUTIVA  publicada em 08/09, issue #3   │
                                                                    ╰────────────────────────────────────────────╯
 n corrige   c coleta   l clona   v verifica   o editor   w GitLab   V vincula   s ordem   / filtra   esc volta
```

- [1] é a lista de hoje, com as mesmas colunas e a mesma marca `d` para
  entrega em dupla. A situação na cor de `corDaSituacao`, a verificação na de
  `corDaVerificacao`.
- [2] é novo: tudo o que hoje está espalhado entre o rodapé e outras telas.
  Fork, commit avaliado e último commit com data, atraso, a equipe quando a
  entrega é compartilhada, o resultado da suíte com o detalhe do erro, a nota,
  o comentário inteiro quebrado na largura, e a devolutiva: publicada (com
  data e número da issue), desatualizada (`Devolutiva.Desatualizada`, em
  laranja) ou não publicada.
- A escolha do dono do fork em `V` continua como hoje, com a pergunta na
  barra de teclas.

## Correção

A tela de `internal/correcao` ganha a mesma divisão: [1] lista de alunos com
situação, verificação e nota; [2] a entrega do aluno selecionado, como em
Entregas, com o comentário inteiro. A barra de título leva o que hoje é o
cabeçalho da tela ("Correção de html · prazo 05/09 · nota de 0 a 100 · 20 de
32 com nota lançada").

A edição de nota e de comentário acontece dentro de [2], no lugar do valor,
com o cursor de texto de hoje. As teclas não mudam.

## Exercícios, Alunos, Equipes e Tarefas

Cada uma vira lista à esquerda e detalhe à direita:

| Tela | [1] lista | [2] detalhe |
|---|---|---|
| Exercícios (`x`) | a tabela de hoje | todos os campos do exercício; a edição de campo (`T`, `D`, `P`, `V`, `I`, `K`) acontece aqui, no lugar do valor |
| Alunos (`a`) | a lista de hoje, com o filtro `P` | conta, grupo esperado e grupo encontrado, situação do cadastro (`SituacaoConta`), com o mesmo texto que a tela de alunos usa hoje, e a situação de entrega do aluno em cada exercício ativo |
| Equipes (`e`) | os vínculos de hoje | dono do fork, integrantes, origem do vínculo (gitlab ou manual) e data |
| Tarefas (`t`) | as tarefas da sessão, com estado | o registro inteiro da selecionada, com rolagem |

## Teclas

As globais de hoje continuam: `p`, `x`, `a`, `e`, `t` trocam de tela; `S`
sincroniza; `C` coleta tudo; `r` relê do disco; `?` ajuda; `q` volta ou sai;
`esc` volta. As de cada tela também.

Entram as da linguagem comum, que não colidem com nenhuma existente:

| Tecla | Faz |
|---|---|
| `tab`, `shift+tab` | foco no painel seguinte, anterior |
| `1` a `4` | foco direto no painel |
| `pgup`, `pgdown` no detalhe | rola o detalhe |

`j`/`k`, `g`/`G` e `/` continuam agindo na lista; com o foco no detalhe, `j`
e `k` rolam o detalhe. A ajuda deixa de ser a tela `idAjuda` e vira a caixa
sobre a tela atual, com as teclas agrupadas por tela.

## Testes

- O teste de tamanho, para cada tela (painel, entregas, correção, exercícios,
  alunos, equipes, tarefas) em cada painel em foco, com e sem tarefa em curso.
- Teste de modelo: o detalhe segue o cursor em entregas e na correção; `tab`
  passa pelos painéis; `Enter` numa pendência seleciona o exercício; o
  progresso de tarefa aparece na barra de título em qualquer tela.
- Os testes atuais de `internal/tui` e `internal/correcao` continuam passando
  sem mudar o que conferem. Teste que conferia texto exato de desenho muda
  para o desenho novo; teste que conferia operação, gravação ou chamada à API
  não muda.
- Nenhum dado de aluno real, como no resto do repositório.

## Fases

| Fase | Entrega |
|---|---|
| 1 | `internal/moldura` copiada do `painel`, com o teste de tamanho e a ajuda em caixa |
| 2 | painel e entregas em painéis, com abas e progresso na barra de título |
| 3 | correção em painéis |
| 4 | exercícios, alunos, equipes e tarefas em painéis |

A fase 2 é a que mais muda o dia a dia: é onde o professor passa da visão da
turma para a de um exercício e de um aluno.

## Fora de escopo

- Operação nova. Esta especificação é só de interface.
- Mouse.
- Visão de várias turmas. Essa é a do `painel`.

## Decisões em aberto

- Se [4] Tarefa no painel aparece sempre, vazia sem tarefa, ou só com tarefa,
  como proposto.
- Extrair a moldura num módulo comum às três ferramentas, se as cópias
  divergirem.
