package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/alexkutzke/gitlab-classroom/internal/coleta"
	"github.com/alexkutzke/gitlab-classroom/internal/relatorio"
	"github.com/alexkutzke/gitlab-classroom/internal/turma"
)

func cmdColetar() *cobra.Command {
	var ids []string
	var dryRun, detalhado bool

	c := &cobra.Command{
		Use:   "coletar",
		Short: "Percorre o GitLab e classifica a entrega de cada aluno",
		Long: "Para cada aluno e cada exercício, localiza o fork, separa os commits do\n" +
			"aluno dos que vieram do repositório-modelo e decide a situação da entrega\n" +
			"pelo commit mais recente até o prazo.\n\n" +
			"Sem --exercicio, coleta todos os exercícios ativos. A coleta regrava as\n" +
			"entregas dos exercícios coletados e nunca toca nas notas lançadas.",
		Example: "  classroom coletar\n" +
			"  classroom coletar --exercicio html\n" +
			"  classroom coletar --exercicio html --exercicio prepare --detalhado",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, t, err := abrir()
			if err != nil {
				return err
			}

			exercicios, err := escolherExercicios(t, ids)
			if err != nil {
				return err
			}
			alunos := t.Ativos()
			if len(alunos) == 0 {
				return fmt.Errorf("nenhum aluno ativo: rode `classroom sync` para importar o cadastro")
			}

			cli, err := cliente(t.Config)
			if err != nil {
				return err
			}
			col := &coleta.Coletor{
				Cliente:   cli,
				Config:    t.Config,
				Progresso: progressoTerminal(len(alunos)),
			}
			res, err := col.Coletar(alunos, exercicios)
			if err != nil {
				return err
			}
			fmt.Print("\r\033[K")

			aplicarAlunos(t, res.Alunos)
			for _, e := range exercicios {
				var doExercicio []turma.Entrega
				for _, en := range res.Entregas {
					if en.Exercicio == e.ID {
						doExercicio = append(doExercicio, en)
					}
				}
				t.SubstituirEntregas(e.ID, doExercicio)

				var vinculos []turma.Vinculo
				for _, v := range res.Vinculos {
					if v.Exercicio == e.ID {
						vinculos = append(vinculos, v)
					}
				}
				t.SubstituirVinculosDescobertos(e.ID, vinculos)
			}
			relatarEquipes(t, exercicios)

			for _, e := range exercicios {
				entregas := t.EntregasDoExercicio(e.ID)
				if detalhado {
					relatorio.Entregas(os.Stdout, t, e, t.Ativos(), entregas)
					fmt.Println()
					continue
				}
				fmt.Printf("%s  prazo %s\n  ", e.ID, e.Prazo.String())
				relatorio.Resumo(os.Stdout, entregas)
			}

			if dryRun {
				fmt.Println("\nNada foi gravado (--dry-run).")
				return nil
			}
			return s.Gravar(t)
		},
	}
	c.Flags().StringArrayVar(&ids, "exercicio", nil, "exercício a coletar (repetível; padrão: todos os ativos)")
	c.Flags().BoolVar(&detalhado, "detalhado", false, "listar aluno a aluno em vez do resumo")
	c.Flags().BoolVar(&dryRun, "dry-run", false, "mostrar o resultado sem gravar")
	return c
}

// relatarEquipes avisa quais entregas são de mais de um aluno, porque isso
// muda o que o professor vai abrir e quantas notas vai lançar.
func relatarEquipes(t *turma.Turma, exercicios []turma.Exercicio) {
	for _, e := range exercicios {
		vinculos := t.VinculosDoExercicio(e.ID)
		if len(vinculos) == 0 {
			continue
		}
		fmt.Printf("Entregas compartilhadas em %s:\n", e.ID)
		for _, v := range vinculos {
			integrante, _ := t.AlunoPorGRR(v.GRR)
			dono, _ := t.AlunoPorGRR(v.Dono)
			marca := ""
			if v.Origem == turma.VinculoManual {
				marca = " (cadastrada à mão)"
			}
			fmt.Printf("  %s entregou no fork de %s%s\n",
				nomeOuGRR(integrante, v.GRR), nomeOuGRR(dono, v.Dono), marca)
		}
	}
}

func nomeOuGRR(a *turma.Aluno, grr string) string {
	if a == nil {
		return grr
	}
	return a.Nome
}

// escolherExercicios resolve os ids informados, ou devolve todos os ativos.
func escolherExercicios(t *turma.Turma, ids []string) ([]turma.Exercicio, error) {
	if len(ids) == 0 {
		es := t.ExerciciosAtivos()
		if len(es) == 0 {
			return nil, fmt.Errorf("nenhum exercício cadastrado: use `classroom exercicios add`")
		}
		return es, nil
	}
	var out []turma.Exercicio
	for _, id := range ids {
		e, ok := t.Exercicio(id)
		if !ok {
			return nil, fmt.Errorf("exercício %q não encontrado", id)
		}
		out = append(out, *e)
	}
	return out, nil
}
