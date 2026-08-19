package cli

import (
	"fmt"
	"os"
	"strings"
	"time"

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
	c.AddCommand(cmdAlunosEditar())
	return c
}

func cmdAlunosEditar() *cobra.Command {
	var grr, usuario, grupo, obs string

	c := &cobra.Command{
		Use:   "editar",
		Short: "Ajusta o usuário ou o grupo de um aluno no GitLab",
		Long: "O usuário de cada aluno é, por convenção, o GRR em minúsculas, e o grupo\n" +
			"segue o padrão do config.toml. Este comando cobre as exceções: o aluno\n" +
			"que não conseguiu criar a conta com o GRR e usou outro login, ou que\n" +
			"batizou o grupo de um jeito que a busca automática não encontra.\n\n" +
			"Nome, e-mail e situação não se editam aqui: vêm do SIGA pelo .diario/ e\n" +
			"seriam sobrescritos no próximo sync.",
		Example: "  classroom alunos editar --grr GRR20259001 --usuario ana.souza\n" +
			"  classroom alunos editar --grr GRR20259001 --grupo ds122-noturno-ana",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, t, err := abrir()
			if err != nil {
				return err
			}
			a, ok := t.AlunoPorGRR(grr)
			if !ok {
				return fmt.Errorf("aluno %q não encontrado no cadastro", grr)
			}

			mudou := false
			if cmd.Flags().Changed("usuario") {
				a.Usuario = turma.UsuarioGitLab(usuario)
				mudou = true
			}
			if cmd.Flags().Changed("grupo") {
				a.Grupo = strings.TrimSpace(grupo)
				mudou = true
			}
			if cmd.Flags().Changed("obs") {
				a.Observacao = obs
			}
			if mudou {
				// O que foi apurado antes deixa de valer: a próxima coleta ou
				// sync precisa conferir a conta de novo.
				a.SituacaoConta = turma.ContaDesconhecida
				a.VerificadoEm = time.Time{}
			}

			if err := s.Gravar(t); err != nil {
				return err
			}
			fmt.Printf("%s %s: usuário %s, grupo %s\n",
				a.GRR, a.Nome, a.UsuarioEsperado(), ouTraco(a.Grupo))
			if mudou {
				fmt.Println("Rode `classroom sync` para confirmar o acesso ao grupo.")
			}
			return nil
		},
	}
	c.Flags().StringVar(&grr, "grr", "", "aluno a alterar")
	c.Flags().StringVar(&usuario, "usuario", "", "login no GitLab, quando não for o GRR")
	c.Flags().StringVar(&grupo, "grupo", "", "caminho do grupo, quando a busca automática não o achar")
	c.Flags().StringVar(&obs, "obs", "", "anotação livre sobre o caso")
	c.MarkFlagRequired("grr")
	return c
}

func ouTraco(s string) string {
	if s == "" {
		return "-"
	}
	return s
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
