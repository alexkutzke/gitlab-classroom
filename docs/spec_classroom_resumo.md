# Especificação: `classroom resumo [--json]`

Destino: este repositório (`~/Documents/work/dev/gitlab-classroom`). Escrita
para ser implementada por outra sessão, sem contexto prévio desta.

Origem: fase 0 de `~/Documents/work/dev/painel/docs/especificacao.md`. O
contrato abaixo foi combinado lá em 06/10/2026 e refinado aqui com o que o
código do `classroom` mostrou. Depois de implementado, a referência do
contrato passa a ser este repositório: o `painel` lê, e quem muda o formato é
o `classroom`.

## Problema

O `painel` é uma CLI nova que mostra como está o semestre inteiro: as quatro
turmas do `diario`, as entregas do GitLab e as orientações de TCC. Ele só lê,
e não pode recalcular situação de entrega, fila de correção ou devolutiva
pendente, porque uma segunda definição acabaria divergindo da que o
`classroom` usa.

Os pacotes `internal/` deste módulo não podem ser importados por outro módulo
Go. A fronteira é, por isso, um comando que imprime JSON: o `painel` executa
`classroom resumo --json` com o diretório de trabalho na pasta da turma, nas
turmas que têm `.classroom/`.

Na visão global, o `painel` mostra só o total por corrigir e a idade da
coleta de cada turma. O detalhe por exercício aparece na visão de uma turma,
`painel turma`.

## O que já existe e não deve ser reimplementado

| Onde | O que faz |
|---|---|
| `internal/acoes/panorama.go`, `PanoramaDe` | por exercício ativo: contagem por situação de entrega, notas lançadas, entregas sem nota e verificações sobre commit antigo, em ordem de prazo |
| `internal/acoes/panorama.go`, `Panorama.ContasPendentes` | alunos ativos com cadastro fora de `ok` |
| `internal/turma/turma.go`, `SituacaoEntrega` e `SituacaoConta` | as situações, com o motivo de cada uma nos comentários |
| `internal/turma/turma.go`, `Verificacao.Desatualizada` | verificação feita sobre commit diferente do da entrega |
| `internal/turma/turma.go`, `Devolutiva.Desatualizada` | devolutiva publicada com comentário diferente do atual |
| `internal/acoes/devolutiva.go`, `planejarDevolutivas` | o que `classroom devolutiva` publicaria, pularia ou acusaria como desatualizado, com a equipe agrupada por fork |
| `internal/turma/turma.go`, `Entrega.ColetadoEm` | instante da coleta de cada entrega |

`PanoramaDe` já faz quase toda a conta: é o que `classroom status` e a tela
inicial da TUI mostram. O `resumo` expõe o mesmo panorama em JSON, com as
duas contagens que faltam (instante da coleta e devolutivas pendentes). Se
alguma definição abaixo parecer pedir regra nova, a definição está errada e
deve ser corrigida aqui, não contornada no código.

## Comportamento pedido

Comando novo `classroom resumo`, sem argumentos, em `internal/cli/consulta.go`,
ao lado de `status`.

Flags:

- `--json`: imprime o contrato abaixo na saída padrão, indentado.

Não há `--hoje`. Nenhum campo do contrato depende da data do dia: o `painel`
calcula quanto tempo passou do prazo e da coleta com as datas que recebe.

Sem `--json`, imprime as mesmas contagens em texto, uma linha por exercício.

O comando não acessa o GitLab: lê só o que está em `.classroom/`. Coletar
continua sendo `classroom coletar`, disparado pelo professor.

Saída de erro segue o padrão do `main` (`erro: ...` na saída de erro, código
1). O `painel` mostra a primeira linha dessa mensagem como alerta da turma.

## Contrato, versão 1

```json
{
  "versao": 1,
  "gerado_em": "2026-10-06T14:00:00-03:00",
  "coletado_em": "2026-10-04T09:12:00-03:00",
  "cadastro": {
    "ativos": 30, "sem_conta": 1, "sem_acesso": 1, "grupo_invisivel": 0,
    "grupo_divergente": 2, "sem_reconciliacao": 0
  },
  "exercicios": [
    {
      "id": "html", "titulo": "Exercício HTML", "categoria": "exercicio",
      "prazo": "2026-09-10", "peso": 1,
      "entregues": 25, "atrasadas": 3, "sem_entrega": 2, "erros": 0,
      "corrigidas": 20, "por_corrigir": 5,
      "verificacoes_desatualizadas": 1, "devolutivas_pendentes": 2
    }
  ]
}
```

Formatos: data como `"AAAA-MM-DD"`; instante em RFC 3339 com fuso. Lista
vazia sai como `[]`, nunca `null`.

### Definições

"Aluno ativo" é o de `Turma.Ativos()`. Entrega em equipe conta uma vez por
aluno, como em `PanoramaDe`, com uma exceção registrada em
`devolutivas_pendentes`.

`coletado_em`: o maior `ColetadoEm` entre as entregas de `entregas.csv`.
Ausente quando nunca houve coleta. É dele que o `painel` mede se a coleta
está velha, e por isso não pode ser a data de modificação do arquivo, que
muda com a sincronização do Nextcloud.

`cadastro`: contagem dos alunos ativos por `SituacaoConta`.

- `sem_conta`, `sem_acesso`, `grupo_invisivel`, `grupo_divergente`: as
  situações de mesmo nome.
- `sem_reconciliacao`: `ContaDesconhecida`, o aluno que ainda não passou por
  `classroom sync`.
- `ativos`: todos os alunos ativos, inclusive os `ok`.

`exercicios`: um por exercício de `ExerciciosAtivos()`, na ordem de prazo de
`PanoramaDe`. Situação `arquivado` fica de fora.

- `id`, `titulo`, `categoria`, `prazo`, `peso`: os campos de
  `exercicios.csv`.
- `entregues`, `atrasadas`, `sem_entrega` e `erros` são disjuntos, um por
  situação de entrega, tirados de `ResumoExercicio.Situacoes`:
  - `entregues`: `entregue`, isto é, há commit do aluno no prazo, ainda que
    haja outros depois;
  - `atrasadas`: `sem_commit_no_prazo`, todos os commits do aluno vieram
    depois do prazo;
  - `sem_entrega`: `sem_fork` mais `fork_sem_commit`;
  - `erros`: `erro`, falha de rede ou de API, que não é veredito sobre o
    aluno.

  As situações de cadastro (`sem_conta`, `sem_acesso`, `grupo_invisivel`)
  ficam em `cadastro`, e não aqui, para a falta de entrega não se misturar com
  a falta de acesso. Por isso, e porque aluno sem linha em `entregas.csv` não
  entra em nenhuma, as quatro contagens não somam necessariamente
  `cadastro.ativos`.
- `corrigidas`: `ResumoExercicio.Notas`, alunos ativos com nota lançada.
- `por_corrigir`: `ResumoExercicio.SemNota`, entrega `entregue` sem nota. É a
  mesma conta que `classroom status` usa para "entrega sem nota". Se a regra
  de correção passar a incluir a entrega atrasada, ela muda em `PanoramaDe`, e
  o `resumo` acompanha sem mudar a versão.
- `verificacoes_desatualizadas`: `ResumoExercicio.VerificacoesVelhas`, a
  mesma condição que a TUI de correção marca com `!`.
- `devolutivas_pendentes`: o que `classroom devolutiva` sem `--aplicar`
  apontaria como a publicar ou como desatualizado, isto é, nota com
  comentário e fork encontrado, sem devolutiva publicada ou com
  `Devolutiva.Desatualizada`. Conta uma vez por fork, porque a devolutiva de
  uma equipe é uma issue só; é a exceção à contagem por aluno. Nota sem
  comentário não conta, porque não há o que publicar. Se `planejarDevolutivas`
  depender do cliente do GitLab, separar a parte que só lê `.classroom/` numa
  função própria e usá-la nos dois lugares.

### Versão

`versao` muda quando um campo existente muda de sentido ou some. Campo novo
não muda a versão, e o `painel` ignora o que não conhece. Diante de versão
desconhecida, o `painel` mostra erro naquela turma em vez de números.

## Onde fica o código

O cálculo vai para `internal/acoes`, ao lado de `PanoramaDe`, numa função
pura que recebe a turma e devolve o resumo. O comando só serializa.

## Testes

- Teste de mesa, com nome de caso em pt-BR, para cada definição que tem
  borda: entrega `entregue` com atraso contada em `entregues`; entrega em
  dupla contada uma vez por aluno em `entregues` e uma vez só em
  `devolutivas_pendentes`; devolutiva com comentário alterado depois de
  publicada; nota sem comentário fora das pendentes; aluno com
  `grupo_divergente`; turma nunca coletada sem `coletado_em`; exercício
  `arquivado` fora da lista.
- Arquivo dourado: uma turma fictícia em `testdata`, saída de
  `resumo --json` comparada com `testdata/resumo.golden.json`, com
  `gerado_em` fixado no teste. Uma flag de teste `-atualizar` regrava o
  arquivo. Esse arquivo é o que o `painel` copia para os testes dele, e por
  isso precisa cobrir todos os campos do contrato.
- Nenhum dado de aluno real, como no resto do repositório.

## Fora de escopo

- Coleta no GitLab e qualquer escrita em `.classroom/`.
- Nome de aluno. O contrato leva só contagens; o detalhe por aluno continua
  em `classroom status` e na TUI.
- Limiar de alerta, como "entrega sem nota há mais de 10 dias do prazo" ou
  "coleta com mais de 72 horas". Isso é critério de acompanhamento, e fica na
  configuração do `painel`.
