package export

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"

	"github.com/alexkutzke/gitlab-classroom/internal/turma"
)

func turmaComNotas() *turma.Turma {
	return &turma.Turma{
		Config: turma.Config{Codigo: "DS122", Turma: "TADSN2A", Semestre: "2026-02",
			Disciplina: "DESENVOLVIMENTO WEB I"},
		Alunos: []turma.Aluno{
			{GRR: "GRR20259001", Nome: "Ana Souza", Situacao: turma.Ativo},
			{GRR: "GRR20259002", Nome: "Bruno Lima", Situacao: turma.Ativo},
		},
		Exercicios: []turma.Exercicio{
			{ID: "prepare", Repo: "r1", Prazo: turma.NovaData(2026, time.August, 15),
				Peso: 1, Situacao: turma.ExercicioAtivo},
			{ID: "html", Repo: "r2", Prazo: turma.NovaData(2026, time.September, 5),
				Peso: 3, Situacao: turma.ExercicioAtivo},
		},
		Notas: []turma.Nota{
			{Exercicio: "prepare", GRR: "GRR20259001", Valor: 100},
			{Exercicio: "html", GRR: "GRR20259001", Valor: 80},
			{Exercicio: "prepare", GRR: "GRR20259002", Valor: 60},
		},
	}
}

func TestNotasCSV(t *testing.T) {
	var buf bytes.Buffer
	o := OpcoesNotas{Hoje: turma.NovaData(2026, time.September, 20)}
	if err := NotasCSV(&buf, turmaComNotas(), o); err != nil {
		t.Fatal(err)
	}
	saida := buf.String()

	if !strings.Contains(saida, "GRR;Nome;prepare;html (peso 3);Média") {
		t.Errorf("cabeçalho inesperado:\n%s", saida)
	}
	if !strings.Contains(saida, "GRR20259001;Ana Souza;100;80;85.0") {
		t.Errorf("linha de Ana inesperada:\n%s", saida)
	}
	// Bruno não tem nota em html, e a célula fica vazia mesmo contando zero
	// na média: vazio e zero são coisas diferentes.
	if !strings.Contains(saida, "GRR20259002;Bruno Lima;60;;15.0") {
		t.Errorf("linha de Bruno inesperada:\n%s", saida)
	}
}

func TestNotasXLSX(t *testing.T) {
	caminho := filepath.Join(t.TempDir(), "notas.xlsx")
	o := OpcoesNotas{Hoje: turma.NovaData(2026, time.September, 20)}
	if err := NotasXLSX(caminho, turmaComNotas(), o); err != nil {
		t.Fatal(err)
	}

	f, err := excelize.OpenFile(caminho)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	if got, _ := f.GetCellValue("Notas", "A3"); got != "GRR" {
		t.Errorf("A3 = %q, queria GRR", got)
	}
	if got, _ := f.GetCellValue("Notas", "B4"); got != "Ana Souza" {
		t.Errorf("B4 = %q, queria Ana Souza", got)
	}
	// A média entra como número, e não como texto, para a planilha ordenar e
	// somar sem conversão.
	if got, _ := f.GetCellValue("Notas", "E4"); got != "85" {
		t.Errorf("média de Ana = %q, queria 85", got)
	}
	// Número em xlsx é a célula sem atributo de tipo, que o excelize devolve
	// como CellTypeUnset; o que não pode acontecer é virar texto.
	tipo, _ := f.GetCellType("Notas", "E4")
	if tipo == excelize.CellTypeInlineString || tipo == excelize.CellTypeSharedString {
		t.Errorf("a média foi gravada como texto (%v)", tipo)
	}
	if got, _ := f.GetCellValue("Notas", "D5"); got != "" {
		t.Errorf("célula sem nota deveria ficar vazia, veio %q", got)
	}
}
