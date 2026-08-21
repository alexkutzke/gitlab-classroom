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
				return fmt.Errorf("exercício %q não encontrado", id)
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
				return fmt.Errorf("exercício %q não encontrado", id)
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
	var saida string
	var emCSV, somenteLancadas bool

	c := &cobra.Command{
		Use:   "notas",
		Short: "Gera a planilha de notas dos exercícios",
		Long: "Uma coluna por exercício e a média ponderada pelos pesos. Exercício com\n" +
			"prazo vencido e sem nota conta como zero na média, porque quem não\n" +
			"entregou tirou zero; exercício com prazo em aberto fica de fora até\n" +
			"vencer. Com --somente-lancadas, a média considera apenas o que já foi\n" +
			"corrigido.",
		Example: "  classroom notas\n" +
			"  classroom notas --somente-lancadas\n" +
			"  classroom notas --csv -o -",
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
				SomenteLancadas: somenteLancadas,
				Hoje:            turma.Hoje(),
			}

			if emCSV {
				if saida == "" || saida == "-" {
					return export.NotasCSV(os.Stdout, t, opts)
				}
				f, err := os.Create(saida)
				if err != nil {
					return err
				}
				defer f.Close()
				if err := export.NotasCSV(f, t, opts); err != nil {
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
	return c
}

func nomePadraoNotas(c turma.Config) string {
	partes := []string{c.Codigo}
	if c.Turma != "" {
		partes = append(partes, c.Turma)
	}
	return strings.Join(partes, "_") + "_notas_exercicios.xlsx"
}
