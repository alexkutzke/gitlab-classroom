package coleta

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	gl "github.com/alexkutzke/gitlab-classroom/internal/gitlab"
	"github.com/alexkutzke/gitlab-classroom/internal/turma"
)

// A reentrega de uma parte do trabalho é coletada de um ramo próprio do fork,
// porque o ramo padrão já avançou para a parte seguinte, no mesmo repositório.

func exercicioComRamo() turma.Exercicio {
	e := exercicioExemplo()
	e.Ramo = "reentrega"
	return e
}

func coletarComRamo(t *testing.T, c *clienteFalso, alunos ...turma.Aluno) Resultado {
	t.Helper()
	if len(alunos) == 0 {
		alunos = []turma.Aluno{ana()}
	}
	col := &Coletor{Cliente: c, Config: configExemplo()}
	res, err := col.Coletar(context.Background(), alunos, []turma.Exercicio{exercicioComRamo()})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestComRamoAEntregaSaiDoRamoEOPadraoNaoConta(t *testing.T) {
	c := baseFalsa()
	fork := grupoAna + "/ds122-html-assignment"
	c.ramos = map[string][]string{fork: {"main", "reentrega"}}
	// O ramo padrão já tem a parte seguinte, com commit dentro do prazo
	// prorrogado: não pode virar a entrega da reentrega.
	c.commits[fork] = []gl.Commit{
		{SHA: "php", Data: time.Date(2026, 9, 5, 20, 0, 0, 0, time.Local)},
		{SHA: "e2", Data: time.Date(2026, 9, 1, 10, 0, 0, 0, time.Local)},
		{SHA: "modelo1", Data: time.Date(2026, 8, 1, 10, 0, 0, 0, time.Local)},
	}
	c.commitsRef = map[string][]gl.Commit{
		fork + "|reentrega|false": {
			{SHA: "correcao", Data: time.Date(2026, 9, 4, 18, 0, 0, 0, time.Local)},
			{SHA: "e2", Data: time.Date(2026, 9, 1, 10, 0, 0, 0, time.Local)},
			{SHA: "modelo1", Data: time.Date(2026, 8, 1, 10, 0, 0, 0, time.Local)},
		},
	}
	e := entregaDe(t, coletarComRamo(t, c), "GRR20259001")

	if e.Situacao != turma.Entregue || e.Commit != "correcao" {
		t.Errorf("entrega = %+v, queria o commit mais recente do ramo reentrega", e)
	}
	if e.Commits != 2 {
		t.Errorf("commits = %d, queria os 2 do aluno no ramo", e.Commits)
	}
}

func TestComRamoOPrazoContinuaValendo(t *testing.T) {
	c := baseFalsa()
	fork := grupoAna + "/ds122-html-assignment"
	c.ramos = map[string][]string{fork: {"main", "reentrega"}}
	c.commitsRef = map[string][]gl.Commit{
		fork + "|reentrega|false": {
			{SHA: "tarde", Data: time.Date(2026, 9, 6, 0, 10, 0, 0, time.Local)},
			{SHA: "e2", Data: time.Date(2026, 9, 1, 10, 0, 0, 0, time.Local)},
		},
	}
	e := entregaDe(t, coletarComRamo(t, c), "GRR20259001")
	if e.Situacao != turma.Entregue || e.Commit != "e2" || e.UltimoCommit != "tarde" || e.AtrasoDias != 1 {
		t.Errorf("entrega = %+v, queria e2 até as 23h59 do prazo e o atraso registrado", e)
	}
}

func TestForkSemORamoFicaComoSemRamo(t *testing.T) {
	c := baseFalsa()
	fork := grupoAna + "/ds122-html-assignment"
	c.ramos = map[string][]string{fork: {"main"}}
	c.commits[fork] = []gl.Commit{
		{SHA: "e2", Data: time.Date(2026, 9, 1, 10, 0, 0, 0, time.Local)},
	}
	e := entregaDe(t, coletarComRamo(t, c), "GRR20259001")

	if e.Situacao != turma.SemRamo {
		t.Fatalf("situação = %v, queria sem_ramo", e.Situacao)
	}
	if !strings.Contains(e.Detalhe, "reentrega") {
		t.Errorf("detalhe = %q, queria o nome do ramo esperado", e.Detalhe)
	}
	if e.Projeto != fork || e.Commit != "" {
		t.Errorf("entrega = %+v: o fork fica registrado, sem commit avaliado", e)
	}
}

func TestRamoSoComCommitsDoModeloEhForkSemCommit(t *testing.T) {
	c := baseFalsa()
	fork := grupoAna + "/ds122-html-assignment"
	c.ramos = map[string][]string{fork: {"main", "reentrega"}}
	c.commitsRef = map[string][]gl.Commit{
		fork + "|reentrega|false": {{SHA: "modelo1", Data: time.Date(2026, 8, 1, 10, 0, 0, 0, time.Local)}},
		fork + "||true": {
			{SHA: "e2", Data: time.Date(2026, 9, 1, 10, 0, 0, 0, time.Local)},
			{SHA: "modelo1", Data: time.Date(2026, 8, 1, 10, 0, 0, 0, time.Local)},
		},
	}
	e := entregaDe(t, coletarComRamo(t, c), "GRR20259001")
	if e.Situacao != turma.ForkSemCommit {
		t.Fatalf("situação = %v, queria fork_sem_commit: o ramo existe", e.Situacao)
	}
	if !strings.Contains(e.Detalhe, "fora do ramo reentrega") {
		t.Errorf("detalhe = %q, queria apontar os commits fora do ramo pedido", e.Detalhe)
	}
}

func TestFalhaAoListarRamosViraErro(t *testing.T) {
	c := &clienteRamosComFalha{baseFalsa()}
	col := &Coletor{Cliente: c, Config: configExemplo()}
	res, err := col.Coletar(context.Background(), []turma.Aluno{ana()}, []turma.Exercicio{exercicioComRamo()})
	if err != nil {
		t.Fatal(err)
	}
	if e := entregaDe(t, res, "GRR20259001"); e.Situacao != turma.Erro {
		t.Errorf("situação = %v, queria erro: falha de API não é sem_ramo", e.Situacao)
	}
}

// clienteRamosComFalha só falha na lista de ramos do fork; a do modelo, que a
// coleta lê antes, continua respondendo.
type clienteRamosComFalha struct{ *clienteFalso }

func (c *clienteRamosComFalha) Ramos(projeto string) ([]string, error) {
	if strings.HasPrefix(projeto, grupoAna) {
		return nil, errors.New("503 Service Unavailable")
	}
	return c.clienteFalso.Ramos(projeto)
}

// Na reentrega em dupla, o ramo pode existir só no fork de quem bifurcou. O
// colega com fork próprio sem o ramo é avaliado pelo fork da dupla.
func TestRamoNoForkDaDuplaVenceOForkProprioSemRamo(t *testing.T) {
	c := duplaFalsa()
	forkAna := grupoAna + "/ds122-html-assignment"
	forkBruno := grupoBruno + "/ds122-html-assignment"
	c.projetos[grupoBruno] = []gl.Projeto{{ID: 20, Caminho: "ds122-html-assignment",
		Completo: forkBruno, RamoPadrao: "main", ForkDe: "ds122-alexkutzke/ds122-html-assignment"}}
	c.ramos = map[string][]string{forkAna: {"main", "reentrega"}, forkBruno: {"main"}}
	c.commitsRef = map[string][]gl.Commit{
		forkAna + "|reentrega|false": {{SHA: "correcao", Data: time.Date(2026, 9, 4, 18, 0, 0, 0, time.Local)}},
	}
	res := coletarComRamo(t, c, ana(), bruno())

	b := entregaDe(t, res, "GRR20259002")
	if b.Situacao != turma.Entregue || b.Projeto != forkAna {
		t.Errorf("entrega de Bruno = %+v, queria a do fork de Ana", b)
	}
}
