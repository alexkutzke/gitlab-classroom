package export

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"

	"github.com/xuri/excelize/v2"

	"github.com/alexkutzke/gitlab-classroom/internal/turma"
)

// OpcoesNotas controla a planilha de notas.
type OpcoesNotas struct {
	Exercicios []turma.Exercicio
	// SomenteLancadas tira do denominador da média os exercícios ainda sem
	// nota, em vez de contá-los como zero.
	SomenteLancadas bool
	Hoje            turma.Data
}

// linhasNotas monta a tabela usada pelos dois formatos de saída.
func linhasNotas(t *turma.Turma, o OpcoesNotas) (cabecalho []string, linhas [][]string) {
	exs := o.Exercicios
	if len(exs) == 0 {
		exs = t.ExerciciosAtivos()
	}
	if o.Hoje.IsZero() {
		o.Hoje = turma.Hoje()
	}

	cabecalho = []string{"GRR", "Nome"}
	for _, e := range exs {
		rotulo := e.ID
		if e.Peso != 1 {
			rotulo += fmt.Sprintf(" (peso %g)", e.Peso)
		}
		cabecalho = append(cabecalho, rotulo)
	}
	cabecalho = append(cabecalho, "Média")

	for _, r := range t.Resultados(exs, o.SomenteLancadas, o.Hoje) {
		linha := []string{r.Aluno.GRR, r.Aluno.Nome}
		for _, e := range exs {
			if v, ok := r.Notas[e.ID]; ok {
				linha = append(linha, strconv.FormatFloat(v, 'f', -1, 64))
				continue
			}
			linha = append(linha, "")
		}
		linha = append(linha, strconv.FormatFloat(r.Media, 'f', 1, 64))
		linhas = append(linhas, linha)
	}
	return cabecalho, linhas
}

// NotasCSV escreve as notas em CSV, com o mesmo separador dos demais
// arquivos da ferramenta.
func NotasCSV(w io.Writer, t *turma.Turma, o OpcoesNotas) error {
	cabecalho, linhas := linhasNotas(t, o)
	cw := csv.NewWriter(w)
	cw.Comma = ';'
	if err := cw.Write(cabecalho); err != nil {
		return err
	}
	if err := cw.WriteAll(linhas); err != nil {
		return err
	}
	cw.Flush()
	return cw.Error()
}

// NotasXLSX grava a planilha de notas.
//
// Célula vazia é exercício sem nota lançada, que é diferente de zero. A média
// já leva o zero de quem não entregou, conforme a regra de Turma.Resultados.
func NotasXLSX(caminho string, t *turma.Turma, o OpcoesNotas) error {
	cabecalho, linhas := linhasNotas(t, o)

	f := excelize.NewFile()
	defer f.Close()
	aba := "Notas"
	indice, err := f.NewSheet(aba)
	if err != nil {
		return err
	}
	f.SetActiveSheet(indice)
	if err := f.DeleteSheet("Sheet1"); err != nil {
		return err
	}

	titulo := fmt.Sprintf("%s  %s  %s", t.Config.Descricao(), t.Config.Disciplina, t.Config.Semestre)
	if err := f.SetCellStr(aba, "A1", titulo); err != nil {
		return err
	}
	estiloTitulo, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true, Size: 12}})
	if err != nil {
		return err
	}
	if err := f.SetCellStyle(aba, "A1", "A1", estiloTitulo); err != nil {
		return err
	}

	estiloCabecalho, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true},
		Alignment: &excelize.Alignment{Horizontal: "center", WrapText: true},
	})
	if err != nil {
		return err
	}
	for i, c := range cabecalho {
		celula, err := excelize.CoordinatesToCellName(i+1, 3)
		if err != nil {
			return err
		}
		if err := f.SetCellStr(aba, celula, c); err != nil {
			return err
		}
		if err := f.SetCellStyle(aba, celula, celula, estiloCabecalho); err != nil {
			return err
		}
	}

	for l, linha := range linhas {
		for i, valor := range linha {
			celula, err := excelize.CoordinatesToCellName(i+1, l+4)
			if err != nil {
				return err
			}
			// As colunas de nota entram como número, para a planilha somar e
			// ordenar sem conversão manual.
			if i >= 2 && valor != "" {
				if v, err := strconv.ParseFloat(valor, 64); err == nil {
					if err := f.SetCellFloat(aba, celula, v, 1, 64); err != nil {
						return err
					}
					continue
				}
			}
			if err := f.SetCellStr(aba, celula, valor); err != nil {
				return err
			}
		}
	}

	larguraNome := 12.0
	for _, l := range linhas {
		if c := float64(len([]rune(l[1]))) + 2; c > larguraNome {
			larguraNome = c
		}
	}
	if err := f.SetColWidth(aba, "A", "A", 14); err != nil {
		return err
	}
	if err := f.SetColWidth(aba, "B", "B", larguraNome); err != nil {
		return err
	}
	if len(cabecalho) > 2 {
		ultima, err := excelize.ColumnNumberToName(len(cabecalho))
		if err != nil {
			return err
		}
		if err := f.SetColWidth(aba, "C", ultima, 12); err != nil {
			return err
		}
	}
	// Painéis congelados nos nomes, para a coluna do aluno continuar visível
	// numa turma com muitos exercícios.
	if err := f.SetPanes(aba, &excelize.Panes{
		Freeze: true, Split: false, XSplit: 2, YSplit: 3,
		TopLeftCell: "C4", ActivePane: "bottomRight",
	}); err != nil {
		return err
	}

	return f.SaveAs(caminho)
}
