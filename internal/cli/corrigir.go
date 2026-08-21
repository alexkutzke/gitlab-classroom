package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

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

			entregas := t.EntregasDoExercicio(e.ID)
			verificacoes := t.VerificacoesDoExercicio(e.ID)
			var itens []correcao.Item
			for _, a := range t.Ativos() {
				en := entregas[a.GRR]
				if soEntregues && en.Situacao != turma.Entregue {
					continue
				}
				item := correcao.Item{
					Aluno:       a,
					Entrega:     en,
					Verificacao: verificacoes[a.GRR],
					Dir:         dirDaEntrega(t, s.Pasta(), e.ID, a),
				}
				if equipe := t.Equipe(e.ID, a.GRR); len(equipe) > 1 {
					item.Equipe = nomesDaEquipe(t, equipe, a.GRR)
				}
				if n, ok := t.Nota(e.ID, a.GRR); ok {
					if semNota {
						continue
					}
					item.Preencher(*n)
				}
				itens = append(itens, item)
			}
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

			for _, n := range res.Notas {
				t.RegistrarNota(n)
			}
			for _, grr := range res.Removidas {
				t.RemoverNota(e.ID, grr)
			}
			if err := s.Gravar(t); err != nil {
				return err
			}
			fmt.Printf("%d nota(s) lançada(s), %d apagada(s) em %s.\n",
				len(res.Notas), len(res.Removidas), e.ID)
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

// nomesDaEquipe devolve os nomes dos demais integrantes da entrega.
func nomesDaEquipe(t *turma.Turma, equipe []string, exceto string) []string {
	var out []string
	for _, grr := range equipe {
		if grr == exceto {
			continue
		}
		if a, ok := t.AlunoPorGRR(grr); ok {
			out = append(out, primeiroNome(a.Nome))
			continue
		}
		out = append(out, grr)
	}
	return out
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

			// A entrega em dupla é um trabalho só: por padrão a nota vale
			// para todos os integrantes, como na interface de correção.
			alvos := []string{a.GRR}
			if !soEste {
				alvos = t.Equipe(e.ID, a.GRR)
			}

			if remover {
				apagadas := 0
				for _, alvo := range alvos {
					if t.RemoverNota(e.ID, alvo) {
						apagadas++
					}
				}
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
			for _, alvo := range alvos {
				n := turma.Nota{Exercicio: e.ID, GRR: alvo, Valor: valor, Comentario: comentario}
				if anterior, ok := t.Nota(e.ID, alvo); ok && !cmd.Flags().Changed("comentario") {
					n.Comentario = anterior.Comentario
				}
				n.CorrigidoEm = agora()
				t.RegistrarNota(n)
			}
			if err := s.Gravar(t); err != nil {
				return err
			}
			if len(alvos) > 1 {
				fmt.Printf("%g em %s para a entrega de %s.\n",
					valor, e.ID, strings.Join(nomesDaEquipe(t, alvos, ""), " e "))
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
				if exercicios, err = escolherExercicios(t, ids); err != nil {
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
