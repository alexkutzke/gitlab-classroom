package export

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/alexkutzke/gitlab-classroom/internal/turma"
)

func turmaExemplo() *turma.Turma {
	return &turma.Turma{
		Config: turma.Config{Codigo: "DS122", Turma: "TADSN2A", Semestre: "2026-02"},
		Alunos: []turma.Aluno{
			{GRR: "GRR20259001", Nome: "Ana Souza", Situacao: turma.Ativo},
			{GRR: "GRR20259002", Nome: "Bruno Lima", Situacao: turma.Ativo},
			{GRR: "GRR20259003", Nome: "Carla Dias", Situacao: turma.Cancelado},
		},
		Exercicios: []turma.Exercicio{{
			ID: "html", Repo: "ds122-html-assignment", Titulo: "HTML",
			Prazo: turma.NovaData(2026, time.September, 5), Situacao: turma.ExercicioAtivo,
		}},
		Entregas: []turma.Entrega{
			{Exercicio: "html", GRR: "GRR20259001", Situacao: turma.Entregue},
			{Exercicio: "html", GRR: "GRR20259002", Situacao: turma.SemCommitNoPrazo, AtrasoDias: 2},
		},
	}
}

func TestCelula(t *testing.T) {
	casos := []struct {
		nome   string
		e      turma.Entrega
		existe bool
		quer   string
	}{
		{"sem coleta", turma.Entrega{}, false, "-"},
		{"entregue", turma.Entrega{Situacao: turma.Entregue}, true, "ok"},
		{"entregue com atraso", turma.Entrega{Situacao: turma.Entregue, AtrasoDias: 1}, true, "ok (mexeu +1d)"},
		{"fora do prazo", turma.Entrega{Situacao: turma.SemCommitNoPrazo, AtrasoDias: 3}, true, "fora do prazo (+3d)"},
		{"sem commit, nada mesmo", turma.Entrega{Situacao: turma.ForkSemCommit}, true, "sem commit"},
		{"sem commit, fork vazio", turma.Entrega{Situacao: turma.ForkSemCommit, Detalhe: "repositório vazio"}, true, "sem commit (fork vazio)"},
		{"sem commit, ramo errado", turma.Entrega{Situacao: turma.ForkSemCommit, Detalhe: "2 commit(s) fora do ramo main"}, true, "sem commit (ramo errado)"},
		{"entregue em dupla", turma.Entrega{Situacao: turma.Entregue, Detalhe: "entrega compartilhada, fork de Ana Souza"}, true, "ok (dupla)"},
		{"atrasado em dupla", turma.Entrega{Situacao: turma.Entregue, AtrasoDias: 1, Detalhe: "entrega compartilhada, fork de Ana Souza"}, true, "ok (mexeu +1d) (dupla)"},
		{"sem commit em dupla", turma.Entrega{Situacao: turma.ForkSemCommit, Detalhe: "entrega compartilhada, fork de Ana Souza"}, true, "sem commit (dupla)"},
		{"sem fork", turma.Entrega{Situacao: turma.SemFork}, true, "sem fork"},
		{"sem conta", turma.Entrega{Situacao: turma.SemConta}, true, "sem conta"},
		{"sem acesso", turma.Entrega{Situacao: turma.SemAcesso}, true, "sem acesso"},
		{"sem grupo", turma.Entrega{Situacao: turma.GrupoInvisivel}, true, "sem grupo"},
		{"erro", turma.Entrega{Situacao: turma.Erro}, true, "erro"},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if got := celula(c.e, c.existe); got != c.quer {
				t.Errorf("celula = %q, queria %q", got, c.quer)
			}
		})
	}
}

func TestMarkdownDuplaNaoNomeiaOColegaNaTabelaPublica(t *testing.T) {
	tur := turmaExemplo()
	tur.Entregas = []turma.Entrega{
		{Exercicio: "html", GRR: "GRR20259001", Situacao: turma.Entregue},
		{Exercicio: "html", GRR: "GRR20259002", Situacao: turma.Entregue,
			Detalhe: "entrega compartilhada, fork de Ana Souza"},
	}
	var buf bytes.Buffer
	if err := Markdown(&buf, tur, Opcoes{Identificacao: PorGRR}); err != nil {
		t.Fatal(err)
	}
	saida := buf.String()
	if !strings.Contains(saida, "| GRR20259002 | ok (dupla) |") {
		t.Errorf("marcador de dupla ausente:\n%s", saida)
	}
	if strings.Contains(saida, "Ana Souza") {
		t.Errorf("nome do colega vazou para a tabela publicada por GRR:\n%s", saida)
	}
}

func TestMarkdown(t *testing.T) {
	var buf bytes.Buffer
	momento := time.Date(2026, 8, 19, 21, 30, 0, 0, time.Local)
	if err := Markdown(&buf, turmaExemplo(), Opcoes{Momento: momento}); err != nil {
		t.Fatal(err)
	}
	saida := buf.String()

	for _, quer := range []string{
		"| Ana Souza | ok |",
		"| Bruno Lima | fora do prazo (+2d) |",
		"HTML<br>05/09",
		"19/08/2026 21:30",
	} {
		if !strings.Contains(saida, quer) {
			t.Errorf("saída não contém %q:\n%s", quer, saida)
		}
	}
	if strings.Contains(saida, "Carla Dias") {
		t.Errorf("aluno cancelado não deveria entrar na tabela:\n%s", saida)
	}
}

func TestMarkdownPorGRR(t *testing.T) {
	var buf bytes.Buffer
	if err := Markdown(&buf, turmaExemplo(), Opcoes{Identificacao: PorGRR}); err != nil {
		t.Fatal(err)
	}
	saida := buf.String()
	if strings.Contains(saida, "Ana Souza") {
		t.Errorf("com --identificacao grr o nome não deveria aparecer:\n%s", saida)
	}
	if !strings.Contains(saida, "| GRR20259001 | ok |") {
		t.Errorf("linha por GRR ausente:\n%s", saida)
	}
	if !strings.Contains(saida, "| GRR |") {
		t.Errorf("a coluna deveria se chamar GRR:\n%s", saida)
	}
	if !strings.Contains(saida, "alexkutzke") {
		t.Errorf("a tabela publicada deveria dizer como corrigir o grupo:\n%s", saida)
	}
}

func TestMarkdownPorGRROrdenaPorGRR(t *testing.T) {
	tur := turmaExemplo()
	// Bruno vem antes de Ana por GRR e depois por nome: a ordem escolhida
	// muda conforme a identificação.
	tur.Alunos[0].GRR, tur.Alunos[1].GRR = "GRR20259002", "GRR20259001"
	tur.Entregas = nil

	var buf bytes.Buffer
	if err := Markdown(&buf, tur, Opcoes{Identificacao: PorGRR}); err != nil {
		t.Fatal(err)
	}
	primeiro := strings.Index(buf.String(), "GRR20259001")
	segundo := strings.Index(buf.String(), "GRR20259002")
	if primeiro > segundo {
		t.Errorf("linhas não saíram em ordem de GRR:\n%s", buf.String())
	}
}

func TestMarkdownSemColeta(t *testing.T) {
	tur := turmaExemplo()
	tur.Entregas = nil
	var buf bytes.Buffer
	if err := Markdown(&buf, tur, Opcoes{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "| Ana Souza | - |") {
		t.Errorf("sem coleta a célula deveria ficar vazia:\n%s", buf.String())
	}
}

func TestTituloSegueACategoriaPublicada(t *testing.T) {
	var buf bytes.Buffer
	tm := turmaComTrabalho()
	o := Opcoes{Exercicios: tm.ExerciciosDaCategoria("trabalho"), Momento: time.Now()}
	if err := Markdown(&buf, tm, o); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(buf.String(), "# Entrega do trabalho,") {
		t.Errorf("título inesperado:\n%s", primeiraLinha(buf.String()))
	}

	buf.Reset()
	if err := Markdown(&buf, tm, Opcoes{Momento: time.Now()}); err != nil {
		t.Fatal(err)
	}
	// Com as duas categorias na mesma tabela, o título geral continua valendo.
	if !strings.HasPrefix(buf.String(), "# Entrega dos exercícios,") {
		t.Errorf("título inesperado:\n%s", primeiraLinha(buf.String()))
	}
}

func primeiraLinha(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
