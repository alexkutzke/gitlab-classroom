package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alexkutzke/gitlab-classroom/internal/relatorio"
	"github.com/alexkutzke/gitlab-classroom/internal/turma"
)

func cmdAlunos() *cobra.Command {
	var todos bool
	var termo string

	c := &cobra.Command{
		Use:     "alunos",
		Short:   "Lista o cadastro da turma",
		Args:    cobra.ArbitraryArgs,
		Example: "  classroom alunos\n  classroom alunos silva\n  classroom alunos --todos",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, t, err := abrir()
			if err != nil {
				return err
			}
			alunos := t.Ativos()
			if todos {
				alunos = t.Alunos
			}
			if termo == "" && len(args) > 0 {
				termo = strings.Join(args, " ")
			}
			alunos = turma.Busca(alunos, termo)
			if len(alunos) == 0 {
				fmt.Println("Nenhum aluno encontrado.")
				return nil
			}
			relatorio.Alunos(os.Stdout, alunos)
			return nil
		},
	}
	c.Flags().BoolVar(&todos, "todos", false, "incluir os alunos cancelados")
	c.Flags().StringVar(&termo, "busca", "", "filtro por nome, GRR ou e-mail")
	return c
}

func cmdStatus() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Panorama da turma: cadastro pendente e situação de cada exercício",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, t, err := abrir()
			if err != nil {
				return err
			}
			relatorio.Status(os.Stdout, t)
			return nil
		},
	}
}
