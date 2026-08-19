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
	if strings.Contains(buf.String(), "Ana Souza") {
		t.Errorf("com --identificacao grr o nome não deveria aparecer:\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), "| GRR20259001 | ok |") {
		t.Errorf("linha por GRR ausente:\n%s", buf.String())
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
