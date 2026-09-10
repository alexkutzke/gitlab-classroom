package export

import (
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"

	"github.com/alexkutzke/gitlab-classroom/internal/turma"
)

// OpcoesNotas controla a planilha de notas.
type OpcoesNotas struct {
	Exercicios []turma.Exercicio
	// Categoria restringe a saída a um grupo de exercícios, o que separa a
	// média dos exercícios em sala da das partes do trabalho. Vazia inclui
	// todas as categorias.
	Categoria string
	// SomenteLancadas tira do denominador da média os exercícios ainda sem
	// nota, em vez de contá-los como zero.
	SomenteLancadas bool
	Hoje            turma.Data
}

// exerciciosDe resolve quais exercícios entram na saída: os informados, ou
// todos os ativos, filtrados pela categoria quando houver uma.
func exerciciosDe(t *turma.Turma, o OpcoesNotas) []turma.Exercicio {
	exs := o.Exercicios
	if len(exs) == 0 {
		exs = t.ExerciciosAtivos()
	}
	if strings.TrimSpace(o.Categoria) == "" {
		return exs
	}
	var out []turma.Exercicio
	for _, e := range exs {
		if strings.EqualFold(e.CategoriaDe(), o.Categoria) {
			out = append(out, e)
		}
	}
	return out
}

// daCategoria filtra o conjunto já escolhido, para calcular a média de cada
// categoria em separado.
func daCategoria(exs []turma.Exercicio, categoria string) []turma.Exercicio {
	var out []turma.Exercicio
	for _, e := range exs {
		if e.CategoriaDe() == categoria {
			out = append(out, e)
		}
	}
	return out
}

// linhasNotas monta a tabela usada pelos dois formatos de saída.
func linhasNotas(t *turma.Turma, o OpcoesNotas) (cabecalho []string, linhas [][]string) {
	exs := exerciciosDe(t, o)
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

	// Uma coluna de média por categoria. Exercício em sala e parte de
	// trabalho viram avaliações diferentes no diario, e uma média só sobre os
	// dois não serve para lançar nem para conferir nada.
	categorias := turma.CategoriasAtivas(exs)
	medias := make(map[string]map[string]string, len(categorias))
	for _, c := range categorias {
		porAluno := map[string]string{}
		for _, r := range t.Resultados(daCategoria(exs, c), o.SomenteLancadas, o.Hoje) {
			// Média de quem ainda não tem nenhum exercício considerado fica
			// vazia, e não zero: antes do primeiro prazo da categoria, zero
			// seria um veredito que ninguém deu. É a mesma distinção das
			// células de nota.
			if len(r.Considerados) == 0 {
				continue
			}
			porAluno[r.Aluno.GRR] = strconv.FormatFloat(r.Media, 'f', 1, 64)
		}
		medias[c] = porAluno
		rotulo := "Média"
		if len(categorias) > 1 {
			rotulo += fmt.Sprintf(" (%s)", c)
		}
		cabecalho = append(cabecalho, rotulo)
	}

	for _, r := range t.Resultados(exs, o.SomenteLancadas, o.Hoje) {
		linha := []string{r.Aluno.GRR, r.Aluno.Nome}
		for _, e := range exs {
			if v, ok := r.Notas[e.ID]; ok {
				linha = append(linha, strconv.FormatFloat(v, 'f', -1, 64))
				continue
			}
			linha = append(linha, "")
		}
		for _, c := range categorias {
			linha = append(linha, medias[c][r.Aluno.GRR])
		}
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

// NotasConsolidadasCSV escreve uma linha por aluno com a média ponderada dos
// exercícios, no formato canônico que o `diario notas --de` importa sem
// conversor: grr;nota;observacao.
//
// Aluno sem nenhum exercício considerado fica de fora do arquivo. Média zero
// por falta de exercício vencido não é nota zero, e nota ausente no diario é
// diferente de zero lançado.
//
// As linhas saem por GRR, e não por nome, para o diff entre execuções
// continuar legível quando a turma muda.
func NotasConsolidadasCSV(w io.Writer, t *turma.Turma, o OpcoesNotas) error {
	exs := exerciciosDe(t, o)
	if o.Hoje.IsZero() {
		o.Hoje = turma.Hoje()
	}
	categorias := turma.CategoriasAtivas(exs)
	if len(categorias) > 1 {
		// Uma consolidação vira uma avaliação do diario. Misturar exercício
		// em sala com parte de trabalho lançaria a nota errada nas duas.
		return fmt.Errorf("a consolidação mistura as categorias %s: escolha uma",
			strings.Join(categorias, " e "))
	}
	categoria := turma.CategoriaExercicio
	if len(categorias) == 1 {
		categoria = categorias[0]
	}

	resultados := t.Resultados(exs, o.SomenteLancadas, o.Hoje)
	sort.Slice(resultados, func(i, j int) bool {
		return resultados[i].Aluno.GRR < resultados[j].Aluno.GRR
	})

	cw := csv.NewWriter(w)
	cw.Comma = ';'
	if err := cw.Write([]string{"grr", "nota", "observacao"}); err != nil {
		return err
	}
	for _, r := range resultados {
		if len(r.Considerados) == 0 {
			continue
		}
		linha := []string{r.Aluno.GRR, formatarMedia(r.Media), observacaoMedia(categoria, r.Considerados)}
		if err := cw.Write(linha); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// formatarMedia arredonda para duas casas e tira os zeros à direita: 95,
// 97.5, 83.33.
func formatarMedia(v float64) string {
	return strconv.FormatFloat(math.Round(v*100)/100, 'f', -1, 64)
}

// observacaoMedia registra como o número foi formado, para o aluno que
// questionar a nota no diario saber o que entrou no denominador.
//
// O `;` é trocado por `,` porque é o separador do arquivo.
func observacaoMedia(categoria string, considerados []string) string {
	singular, plural := "exercício", "exercícios"
	if categoria != turma.CategoriaExercicio {
		// Fora dos exercícios em sala, o que entra na média são as partes de
		// uma avaliação só, como as três etapas do trabalho.
		singular, plural = "parte", "partes"
	}
	rotulo := plural
	if len(considerados) == 1 {
		rotulo = singular
	}
	texto := fmt.Sprintf("média de %d %s: %s",
		len(considerados), rotulo, strings.Join(considerados, ", "))
	return strings.ReplaceAll(texto, ";", ",")
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
