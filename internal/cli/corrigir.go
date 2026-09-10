package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alexkutzke/gitlab-classroom/internal/acoes"
	"github.com/alexkutzke/gitlab-classroom/internal/correcao"
	"github.com/alexkutzke/gitlab-classroom/internal/export"
	"github.com/alexkutzke/gitlab-classroom/internal/turma"
)

func cmdCorrigir() *cobra.Command {
	var id string
	var soEntregues, semNota, semPropagar bool

	c := &cobra.Command{
		Use:   "corrigir",
		Short: "Lança as notas de um exercício, aluno a aluno",
		Long: "Abre a lista da turma com a situação que a coleta apurou e a nota já\n" +
			"lançada, se houver. A nota vai de 0 até o valor de nota_maxima no\n" +
			"config.toml.\n\n" +
			"Refazer a correção carrega o que estava gravado, em vez de duplicar.",
		Example: "  classroom corrigir --exercicio html\n" +
			"  classroom corrigir --exercicio html --sem-nota",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, t, err := abrir()
			if err != nil {
				return err
			}
			e, ok := t.Exercicio(id)
			if !ok {
				return acoes.ErroExercicio(t, id)
			}

			itens := acoes.ItensDeCorrecao(t, s.Pasta(), *e,
				acoes.FiltroCorrecao{SoEntregues: soEntregues, SemNota: semNota})
			if len(itens) == 0 {
				return fmt.Errorf("nenhum aluno a corrigir em %s com esses filtros", e.ID)
			}

			res, err := correcao.Executar(correcao.Opcoes{
				Exercicio:  *e,
				NotaMaxima: t.Config.NotaMaxima,
				Itens:      itens,
				Propagar:   !semPropagar,
			})
			if err != nil {
				return err
			}
			if !res.Salvar {
				fmt.Println("Correção cancelada, nada foi gravado.")
				return nil
			}

			lancadas, apagadas := acoes.AplicarCorrecao(t, e.ID, res)
			if err := s.Gravar(t); err != nil {
				return err
			}
			fmt.Printf("%d nota(s) lançada(s), %d apagada(s) em %s.\n", lancadas, apagadas, e.ID)
			return nil
		},
	}
	c.Flags().StringVar(&id, "exercicio", "", "exercício a corrigir")
	c.Flags().BoolVar(&semPropagar, "sem-propagar", false,
		"não repetir a nota nos demais integrantes da entrega em dupla")
	c.Flags().BoolVar(&soEntregues, "so-entregues", false, "listar só quem entregou no prazo")
	c.Flags().BoolVar(&semNota, "sem-nota", false, "listar só quem ainda não tem nota")
	c.MarkFlagRequired("exercicio")
	return c
}

func cmdNota() *cobra.Command {
	var id, grr, comentario string
	var valor float64
	var remover, soEste bool

	c := &cobra.Command{
		Use:   "nota",
		Short: "Lança ou apaga a nota de um aluno, sem abrir a interface",
		Example: "  classroom nota --exercicio html --grr GRR20259001 --valor 90\n" +
			"  classroom nota --exercicio html --grr GRR20259001 --remover",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, t, err := abrir()
			if err != nil {
				return err
			}
			e, ok := t.Exercicio(id)
			if !ok {
				return acoes.ErroExercicio(t, id)
			}
			a, ok := t.AlunoPorGRR(grr)
			if !ok {
				return fmt.Errorf("aluno %q não encontrado", grr)
			}

			if remover {
				apagadas := acoes.ApagarNota(t, e.ID, a.GRR, soEste)
				if apagadas == 0 {
					return fmt.Errorf("%s não tem nota em %s", a.GRR, e.ID)
				}
				if err := s.Gravar(t); err != nil {
					return err
				}
				fmt.Printf("%d nota(s) apagada(s) em %s.\n", apagadas, e.ID)
				return nil
			}

			if !cmd.Flags().Changed("valor") {
				return fmt.Errorf("informe --valor ou --remover")
			}
			if valor < 0 || valor > t.Config.NotaMaxima {
				return fmt.Errorf("nota %g fora da escala 0 a %g", valor, t.Config.NotaMaxima)
			}
			alvos := acoes.LancarNota(t, e.ID, a.GRR, valor, comentario,
				cmd.Flags().Changed("comentario"), soEste)
			if err := s.Gravar(t); err != nil {
				return err
			}
			if len(alvos) > 1 {
				fmt.Printf("%g em %s para a entrega de %s.\n",
					valor, e.ID, strings.Join(acoes.NomesDaEquipe(t, alvos, ""), " e "))
				return nil
			}
			fmt.Printf("%s em %s: %g.\n", a.Nome, e.ID, valor)
			return nil
		},
	}
	c.Flags().StringVar(&id, "exercicio", "", "exercício")
	c.Flags().StringVar(&grr, "grr", "", "aluno")
	c.Flags().Float64Var(&valor, "valor", 0, "nota, de 0 até nota_maxima")
	c.Flags().StringVar(&comentario, "comentario", "", "comentário devolvido ao aluno")
	c.Flags().BoolVar(&remover, "remover", false, "apagar a nota lançada")
	c.Flags().BoolVar(&soEste, "so-este", false,
		"lançar só para este aluno, sem repetir nos demais integrantes da entrega")
	c.MarkFlagRequired("exercicio")
	c.MarkFlagRequired("grr")
	return c
}

func cmdNotas() *cobra.Command {
	var ids []string
	var saida, categoria string
	var emCSV, somenteLancadas, consolidar bool

	c := &cobra.Command{
		Use:   "notas",
		Short: "Gera a planilha de notas dos exercícios, ou a nota consolidada para o diario",
		Long: "Uma coluna por exercício e a média ponderada pelos pesos. Exercício com\n" +
			"prazo vencido e sem nota conta como zero na média, porque quem não\n" +
			"entregou tirou zero; exercício com prazo em aberto fica de fora até\n" +
			"vencer. Com --somente-lancadas, a média considera apenas o que já foi\n" +
			"corrigido.\n\n" +
			"Cada categoria de exercício ganha sua coluna de média, porque exercício\n" +
			"em sala e parte de trabalho viram avaliações diferentes no diario.\n" +
			"--categoria restringe a saída a uma delas.\n\n" +
			"Com --consolidar, a saída vira um CSV de uma linha por aluno ativo\n" +
			"(grr;nota;observacao), no formato que o `diario notas --de` importa sem\n" +
			"conversor. Aluno sem exercício considerado fica de fora do arquivo,\n" +
			"porque nota ausente no diario é diferente de zero lançado; por isso,\n" +
			"não usar --ausentes nao-entregue ao importar. A média sai na escala de\n" +
			"nota_maxima, e a avaliação do diario precisa ter o mesmo máximo.",
		Example: "  classroom notas\n" +
			"  classroom notas --somente-lancadas\n" +
			"  classroom notas --csv -o -\n" +
			"  classroom notas --consolidar --categoria exercicio -o exercicios_consolidado.csv\n" +
			"  diario notas exercicios --de exercicios_consolidado.csv --conferir\n" +
			"  classroom notas --consolidar --categoria trabalho -o trabalho_consolidado.csv",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, t, err := abrir()
			if err != nil {
				return err
			}
			var exercicios []turma.Exercicio
			if len(ids) > 0 {
				if exercicios, err = acoes.EscolherExercicios(t, ids); err != nil {
					return err
				}
			}
			opts := export.OpcoesNotas{
				Exercicios:      exercicios,
				Categoria:       categoria,
				SomenteLancadas: somenteLancadas,
				Hoje:            turma.Hoje(),
			}
			if categoria != "" && len(t.ExerciciosDaCategoria(categoria)) == 0 {
				return fmt.Errorf("nenhum exercício ativo na categoria %q: as em uso são %s",
					categoria, strings.Join(turma.CategoriasAtivas(t.ExerciciosAtivos()), ", "))
			}

			if consolidar || emCSV {
				escrever := export.NotasCSV
				if consolidar {
					// A consolidação é sempre CSV: --csv junto não é erro, mas
					// um destino .xlsx é, porque o arquivo sairia com o nome
					// errado para o formato.
					if strings.EqualFold(filepath.Ext(saida), ".xlsx") {
						return fmt.Errorf("a consolidação sai em CSV, e %s tem extensão .xlsx", saida)
					}
					// Cada consolidação vira uma avaliação do diario, e o
					// engano de somar trabalho com exercício só apareceria
					// depois da nota lançada.
					if cats := turma.CategoriasAtivas(exerciciosEscolhidos(t, exercicios, categoria)); len(cats) > 1 {
						return fmt.Errorf("há mais de uma categoria de exercício (%s): informe --categoria",
							strings.Join(cats, ", "))
					}
					escrever = export.NotasConsolidadasCSV
				}
				if saida == "" || saida == "-" {
					return escrever(os.Stdout, t, opts)
				}
				f, err := os.Create(saida)
				if err != nil {
					return err
				}
				defer f.Close()
				if err := escrever(f, t, opts); err != nil {
					return err
				}
				fmt.Printf("Notas gravadas em %s\n", saida)
				return nil
			}

			if saida == "" {
				saida = filepath.Join(s.Pasta(), nomePadraoNotas(t.Config))
			}
			if err := export.NotasXLSX(saida, t, opts); err != nil {
				return err
			}
			fmt.Printf("Planilha gravada em %s\n", saida)
			return nil
		},
	}
	c.Flags().StringArrayVar(&ids, "exercicio", nil, "exercício a incluir (repetível; padrão: todos os ativos)")
	c.Flags().StringVarP(&saida, "saida", "o", "", "arquivo de destino")
	c.Flags().BoolVar(&emCSV, "csv", false, "gerar CSV em vez de planilha")
	c.Flags().BoolVar(&somenteLancadas, "somente-lancadas", false, "média só sobre o que já tem nota")
	c.Flags().BoolVar(&consolidar, "consolidar", false,
		"gerar o CSV grr;nota;observacao que o diario importa (padrão: saída padrão)")
	c.Flags().StringVar(&categoria, "categoria", "",
		"restringir a uma categoria de exercício, como exercicio ou trabalho")
	return c
}

// exerciciosEscolhidos repete a resolução que o export faz, para a linha de
// comando conferir as categorias antes de escrever o arquivo.
func exerciciosEscolhidos(t *turma.Turma, exercicios []turma.Exercicio, categoria string) []turma.Exercicio {
	if len(exercicios) == 0 {
		return t.ExerciciosDaCategoria(categoria)
	}
	if categoria == "" {
		return exercicios
	}
	var out []turma.Exercicio
	for _, e := range exercicios {
		if strings.EqualFold(e.CategoriaDe(), categoria) {
			out = append(out, e)
		}
	}
	return out
}

func nomePadraoNotas(c turma.Config) string {
	partes := []string{c.Codigo}
	if c.Turma != "" {
		partes = append(partes, c.Turma)
	}
	return strings.Join(partes, "_") + "_notas_exercicios.xlsx"
}
