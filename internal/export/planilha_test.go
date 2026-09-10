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

// turmaParaConsolidar tem pesos diferentes, um aluno cancelado e um aluno sem
// nenhum exercício vencido, que são os casos que a consolidação precisa
// distinguir.
func turmaParaConsolidar() *turma.Turma {
	t := turmaComNotas()
	t.Alunos = append(t.Alunos,
		turma.Aluno{GRR: "GRR20259003", Nome: "Carla Dias", Situacao: turma.Cancelado},
		turma.Aluno{GRR: "GRR20259004", Nome: "Diego Alves", Situacao: turma.Ativo})
	t.Notas = append(t.Notas,
		turma.Nota{Exercicio: "prepare", GRR: "GRR20259003", Valor: 100},
		turma.Nota{Exercicio: "html", GRR: "GRR20259003", Valor: 100})
	return t
}

func TestNotasConsolidadasCSV(t *testing.T) {
	casos := []struct {
		nome            string
		hoje            turma.Data
		somenteLancadas bool
		quer            []string
		naoQuer         []string
	}{
		{
			nome: "média ponderada e não aritmética",
			hoje: turma.NovaData(2026, time.September, 20),
			// Ana tirou 100 em prepare (peso 1) e 80 em html (peso 3): a média
			// aritmética daria 90, e a ponderada dá 85.
			quer: []string{
				"GRR20259001;85;média de 2 exercícios: prepare, html",
				"GRR20259002;15;média de 2 exercícios: prepare, html",
			},
		},
		{
			nome: "aluno sem exercício vencido fica de fora",
			// Antes de qualquer prazo, Diego não tem nota nem exercício
			// vencido: média zero por ausência de exercício não é nota zero.
			hoje: turma.NovaData(2026, time.August, 1),
			quer: []string{
				"GRR20259001;85;média de 2 exercícios: prepare, html",
				"GRR20259002;60;média de 1 exercício: prepare",
			},
			naoQuer: []string{"GRR20259004"},
		},
		{
			// html venceu e Bruno não tem nota nele: sem --somente-lancadas o
			// denominador tem os dois exercícios, com ela tem só prepare.
			nome: "somente-lancadas muda o denominador",
			hoje: turma.NovaData(2026, time.September, 20),
			quer: []string{
				"GRR20259002;15;média de 2 exercícios: prepare, html",
				// Diego não tem nota, mas os dois prazos venceram: zero
				// lançado, e não aluno ausente do arquivo.
				"GRR20259004;0;média de 2 exercícios: prepare, html",
			},
		},
		{
			nome:            "somente-lancadas ligada sobre o mesmo conjunto",
			hoje:            turma.NovaData(2026, time.September, 20),
			somenteLancadas: true,
			quer:            []string{"GRR20259002;60;média de 1 exercício: prepare"},
		},
		{
			nome:    "aluno cancelado fora da saída",
			hoje:    turma.NovaData(2026, time.September, 20),
			naoQuer: []string{"GRR20259003", "Carla"},
		},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			var buf bytes.Buffer
			o := OpcoesNotas{Hoje: c.hoje, SomenteLancadas: c.somenteLancadas}
			if err := NotasConsolidadasCSV(&buf, turmaParaConsolidar(), o); err != nil {
				t.Fatal(err)
			}
			saida := buf.String()
			if !strings.HasPrefix(saida, "grr;nota;observacao\n") {
				t.Errorf("cabeçalho inesperado:\n%s", saida)
			}
			for _, q := range c.quer {
				if !strings.Contains(saida, q) {
					t.Errorf("faltou %q:\n%s", q, saida)
				}
			}
			for _, n := range c.naoQuer {
				if strings.Contains(saida, n) {
					t.Errorf("não queria %q:\n%s", n, saida)
				}
			}
		})
	}
}

func TestNotasConsolidadasOrdemPorGRR(t *testing.T) {
	var buf bytes.Buffer
	tm := turmaParaConsolidar()
	// Ordem de cadastro invertida em relação ao GRR, para conferir que a saída
	// não herda a ordenação por nome de Ativos.
	tm.Alunos[0], tm.Alunos[1] = tm.Alunos[1], tm.Alunos[0]
	o := OpcoesNotas{Hoje: turma.NovaData(2026, time.September, 20)}
	if err := NotasConsolidadasCSV(&buf, tm, o); err != nil {
		t.Fatal(err)
	}
	if i, j := strings.Index(buf.String(), "GRR20259001"), strings.Index(buf.String(), "GRR20259002"); i > j {
		t.Errorf("linhas fora de ordem por GRR:\n%s", buf.String())
	}
}

func TestFormatarMedia(t *testing.T) {
	casos := []struct {
		valor float64
		quer  string
	}{
		{95, "95"},
		{97.5, "97.5"},
		{250.0 / 3.0, "83.33"},
		{0, "0"},
	}
	for _, c := range casos {
		if got := formatarMedia(c.valor); got != c.quer {
			t.Errorf("formatarMedia(%v) = %q, queria %q", c.valor, got, c.quer)
		}
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

// turmaComTrabalho junta os exercícios em sala e as três partes do trabalho,
// entregues no mesmo repositório e com pesos 30, 30 e 40.
func turmaComTrabalho() *turma.Turma {
	tm := turmaComNotas()
	tm.Exercicios = append(tm.Exercicios,
		turma.Exercicio{ID: "trabalho1", Repo: "ds122-trabalho", Prazo: turma.NovaData(2026, time.September, 30),
			Peso: 30, Categoria: "trabalho", Situacao: turma.ExercicioAtivo},
		turma.Exercicio{ID: "trabalho2", Repo: "ds122-trabalho", Prazo: turma.NovaData(2026, time.October, 28),
			Peso: 30, Categoria: "trabalho", Situacao: turma.ExercicioAtivo},
		turma.Exercicio{ID: "trabalho3", Repo: "ds122-trabalho", Prazo: turma.NovaData(2026, time.November, 25),
			Peso: 40, Categoria: "trabalho", Situacao: turma.ExercicioAtivo})
	tm.Notas = append(tm.Notas,
		turma.Nota{Exercicio: "trabalho1", GRR: "GRR20259001", Valor: 90},
		turma.Nota{Exercicio: "trabalho2", GRR: "GRR20259001", Valor: 80},
		turma.Nota{Exercicio: "trabalho3", GRR: "GRR20259001", Valor: 70})
	return tm
}

func TestNotasCSVSeparaAMediaPorCategoria(t *testing.T) {
	var buf bytes.Buffer
	// Depois do último prazo do trabalho, tudo já venceu.
	o := OpcoesNotas{Hoje: turma.NovaData(2026, time.December, 1)}
	if err := NotasCSV(&buf, turmaComTrabalho(), o); err != nil {
		t.Fatal(err)
	}
	saida := buf.String()

	if !strings.Contains(saida, "Média (exercicio);Média (trabalho)") {
		t.Errorf("faltou uma média por categoria:\n%s", saida)
	}
	// Ana: 85 nos exercícios, e (90*30 + 80*30 + 70*40) / 100 = 79 no trabalho.
	if !strings.Contains(saida, ";85.0;79.0") {
		t.Errorf("médias de Ana inesperadas:\n%s", saida)
	}
	// Bruno não tem nota no trabalho, e as três partes venceram: zero.
	if !strings.Contains(saida, ";15.0;0.0") {
		t.Errorf("médias de Bruno inesperadas:\n%s", saida)
	}
}

func TestNotasCSVComUmaCategoriaMantemAColunaMedia(t *testing.T) {
	var buf bytes.Buffer
	o := OpcoesNotas{Hoje: turma.NovaData(2026, time.September, 20)}
	if err := NotasCSV(&buf, turmaComNotas(), o); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), ";Média\n") {
		t.Errorf("a turma sem trabalho deveria manter a coluna Média sem sufixo:\n%s", buf.String())
	}
}

func TestConsolidarComDuasCategoriasEhErro(t *testing.T) {
	var buf bytes.Buffer
	o := OpcoesNotas{Hoje: turma.NovaData(2026, time.December, 1)}
	err := NotasConsolidadasCSV(&buf, turmaComTrabalho(), o)
	if err == nil {
		t.Fatal("consolidar exercício junto de trabalho lançaria a nota errada nas duas avaliações")
	}
	if !strings.Contains(err.Error(), "trabalho") {
		t.Errorf("a mensagem deveria dizer quais categorias se misturaram: %v", err)
	}
}

func TestConsolidarPorCategoria(t *testing.T) {
	casos := []struct {
		categoria string
		quer      string
		naoQuer   string
	}{
		{
			categoria: "trabalho",
			quer:      "GRR20259001;79;média de 3 partes: trabalho1, trabalho2, trabalho3",
			naoQuer:   "prepare",
		},
		{
			categoria: "exercicio",
			quer:      "GRR20259001;85;média de 2 exercícios: prepare, html",
			naoQuer:   "trabalho",
		},
	}
	for _, c := range casos {
		t.Run(c.categoria, func(t *testing.T) {
			var buf bytes.Buffer
			o := OpcoesNotas{Hoje: turma.NovaData(2026, time.December, 1), Categoria: c.categoria}
			if err := NotasConsolidadasCSV(&buf, turmaComTrabalho(), o); err != nil {
				t.Fatal(err)
			}
			saida := buf.String()
			if !strings.Contains(saida, c.quer) {
				t.Errorf("faltou %q:\n%s", c.quer, saida)
			}
			if strings.Contains(saida, c.naoQuer) {
				t.Errorf("não queria %q:\n%s", c.naoQuer, saida)
			}
		})
	}
}

func TestMediaSemExercicioConsideradoFicaVazia(t *testing.T) {
	var buf bytes.Buffer
	// Antes do primeiro prazo do trabalho: Bruno não tem nota em parte
	// nenhuma, e nenhuma venceu.
	o := OpcoesNotas{Hoje: turma.NovaData(2026, time.September, 20)}
	if err := NotasCSV(&buf, turmaComTrabalho(), o); err != nil {
		t.Fatal(err)
	}
	saida := buf.String()
	if !strings.Contains(saida, "GRR20259002;Bruno Lima;60;;;;;15.0;\n") {
		t.Errorf("média de trabalho de Bruno deveria ficar vazia, e não zero:\n%s", saida)
	}
	// Ana tem as três notas lançadas, então a média dela sai mesmo com os
	// prazos em aberto.
	if !strings.Contains(saida, ";85.0;79.0") {
		t.Errorf("médias de Ana inesperadas:\n%s", saida)
	}
}
