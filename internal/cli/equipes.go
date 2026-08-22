package cli

import (
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/alexkutzke/gitlab-classroom/internal/acoes"
	"github.com/alexkutzke/gitlab-classroom/internal/turma"
)

func cmdEquipes() *cobra.Command {
	var ids []string

	c := &cobra.Command{
		Use:   "equipes",
		Short: "Lista as entregas feitas por mais de um aluno",
		Long: "Nas tarefas em dupla só um dos integrantes faz o fork, e o outro entra\n" +
			"como membro do projeto. A coleta descobre isso sozinha pela lista de\n" +
			"membros de cada fork; este comando mostra o resultado e permite\n" +
			"corrigi-lo à mão.",
		Args:    cobra.NoArgs,
		Example: "  classroom equipes\n  classroom equipes vincular --exercicio html --grr GRR20259002 --dono GRR20259001",
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, t, err := abrir()
			if err != nil {
				return err
			}
			exercicios := t.ExerciciosAtivos()
			if len(ids) > 0 {
				if exercicios, err = acoes.EscolherExercicios(t, ids); err != nil {
					return err
				}
			}

			achou := false
			for _, e := range exercicios {
				vinculos := t.VinculosEmOrdem(e.ID)
				if len(vinculos) == 0 {
					continue
				}
				achou = true
				fmt.Printf("%s  %s\n", e.ID, e.Titulo)
				tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(tw, "  INTEGRANTE\tENTREGOU NO FORK DE\tORIGEM")
				for _, v := range vinculos {
					integrante, _ := t.AlunoPorGRR(v.GRR)
					dono, _ := t.AlunoPorGRR(v.Dono)
					fmt.Fprintf(tw, "  %s\t%s\t%s\n",
						nomeOuGRR(integrante, v.GRR), nomeOuGRR(dono, v.Dono), v.Origem)
				}
				tw.Flush()
				fmt.Println()
			}
			if !achou {
				fmt.Println("Nenhuma entrega compartilhada registrada.")
			}
			return nil
		},
	}
	c.Flags().StringArrayVar(&ids, "exercicio", nil, "exercício a listar (repetível; padrão: todos os ativos)")
	c.AddCommand(cmdEquipesVincular(), cmdEquipesDesvincular(), cmdEquipesDesconhecidos())
	return c
}

func cmdEquipesVincular() *cobra.Command {
	var id, grr, dono string

	c := &cobra.Command{
		Use:   "vincular",
		Short: "Registra à mão que um aluno entregou no fork de outro",
		Long: "Para a dupla que trabalhou junto sem adicionar o colega como membro do\n" +
			"projeto, caso em que a coleta não tem como descobrir sozinha.\n\n" +
			"Vínculo cadastrado aqui não é apagado pela coleta, e vence o que a API\n" +
			"tiver dito sobre o mesmo aluno.",
		Example: "  classroom equipes vincular --exercicio html --grr GRR20259002 --dono GRR20259001",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, t, err := abrir()
			if err != nil {
				return err
			}
			e, ok := t.Exercicio(id)
			if !ok {
				return fmt.Errorf("exercício %q não encontrado", id)
			}
			integrante, ok := t.AlunoPorGRR(grr)
			if !ok {
				return fmt.Errorf("aluno %q não encontrado", grr)
			}
			proprietario, ok := t.AlunoPorGRR(dono)
			if !ok {
				return fmt.Errorf("aluno %q não encontrado", dono)
			}
			if integrante.GRR == proprietario.GRR {
				return fmt.Errorf("o integrante e o dono do fork são o mesmo aluno")
			}
			// Vincular a quem também é integrante de outra entrega criaria
			// uma corrente sem dono definido.
			if d := t.Dono(e.ID, proprietario.GRR); d != proprietario.GRR {
				return fmt.Errorf("%s já entrega no fork de outro aluno; vincule ao dono do fork",
					proprietario.Nome)
			}

			t.RegistrarVinculo(turma.Vinculo{
				Exercicio: e.ID, GRR: integrante.GRR, Dono: proprietario.GRR,
				Origem: turma.VinculoManual, AtualizadoEm: time.Now(),
			})
			if err := s.Gravar(t); err != nil {
				return err
			}
			fmt.Printf("%s entrega no fork de %s em %s.\n",
				integrante.Nome, proprietario.Nome, e.ID)
			fmt.Printf("Recolete com `classroom coletar --exercicio %s` para atualizar a situação.\n", e.ID)
			return nil
		},
	}
	c.Flags().StringVar(&id, "exercicio", "", "exercício")
	c.Flags().StringVar(&grr, "grr", "", "aluno que não tem o fork")
	c.Flags().StringVar(&dono, "dono", "", "aluno em cujo grupo está o fork")
	c.MarkFlagRequired("exercicio")
	c.MarkFlagRequired("grr")
	c.MarkFlagRequired("dono")
	return c
}

func cmdEquipesDesvincular() *cobra.Command {
	var id, grr string

	c := &cobra.Command{
		Use:     "desvincular",
		Short:   "Desfaz o vínculo de um aluno com o fork de outro",
		Example: "  classroom equipes desvincular --exercicio html --grr GRR20259002",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, t, err := abrir()
			if err != nil {
				return err
			}
			e, ok := t.Exercicio(id)
			if !ok {
				return fmt.Errorf("exercício %q não encontrado", id)
			}
			if !t.RemoverVinculo(e.ID, grr) {
				return fmt.Errorf("%s não tem vínculo em %s", grr, e.ID)
			}
			if err := s.Gravar(t); err != nil {
				return err
			}
			fmt.Printf("Vínculo de %s em %s desfeito.\n", grr, e.ID)
			avisar("A próxima coleta pode recriá-lo se o aluno ainda for membro do fork.")
			return nil
		},
	}
	c.Flags().StringVar(&id, "exercicio", "", "exercício")
	c.Flags().StringVar(&grr, "grr", "", "aluno a desvincular")
	c.MarkFlagRequired("exercicio")
	c.MarkFlagRequired("grr")
	return c
}

func cmdEquipesDesconhecidos() *cobra.Command {
	var ids []string

	c := &cobra.Command{
		Use:   "desconhecidos",
		Short: "Lista membros de fork que não batem com nenhum aluno do cadastro",
		Long: "A entrega em dupla é descoberta pela lista de membros do fork, e o\n" +
			"membro é ligado ao aluno pelo login. O aluno que não conseguiu criar a\n" +
			"conta com o GRR e usou outro login não é reconhecido, e aparece no\n" +
			"relatório como quem não entregou.\n\n" +
			"Este comando varre os forks da turma e mostra os logins sem dono, com o\n" +
			"palpite de quem pode ser. Nada é gravado: para adotar o palpite, use\n" +
			"`classroom alunos editar --grr ... --usuario ...` e colete de novo.",
		Example: "  classroom equipes desconhecidos\n" +
			"  classroom equipes desconhecidos --exercicio html",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, t, err := abrir()
			if err != nil {
				return err
			}
			exercicios, err := acoes.EscolherExercicios(t, ids)
			if err != nil {
				return err
			}
			cli, err := cliente(t.Config)
			if err != nil {
				return err
			}

			achados, err := acoes.MembrosDesconhecidos(cmd.Context(), t, cli, exercicios, progressoTerminal())
			if err != nil {
				return err
			}
			limparProgresso()

			if len(achados) == 0 {
				fmt.Println("Todo membro de fork da turma corresponde a um aluno do cadastro.")
				return nil
			}

			tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "EXERCÍCIO\tLOGIN\tNOME NO GITLAB\tNO FORK DE\tPALPITE")
			for _, m := range achados {
				palpite, dono := "-", m.DonoNome
				if m.TemSugestao {
					palpite = m.Sugestao.GRR + "  " + m.Sugestao.Nome
					if m.Sugestao.GRR == m.DonoGRR {
						// Segunda conta do próprio dono do fork, e não dupla.
						dono = "o próprio"
					}
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
					m.Exercicio, m.Usuario, m.Nome, dono, palpite)
			}
			tw.Flush()

			fmt.Println()
			avisar("O palpite vem da semelhança de nome e não é confirmação. Confira antes de cadastrar:")
			visto := map[string]bool{}
			for _, m := range achados {
				chave := m.Sugestao.GRR + " " + m.Usuario
				if !m.TemSugestao || visto[chave] {
					continue
				}
				visto[chave] = true
				fmt.Printf("  classroom alunos editar --grr %s --usuario %s\n",
					m.Sugestao.GRR, m.Usuario)
			}
			return nil
		},
	}
	c.Flags().StringArrayVar(&ids, "exercicio", nil, "exercício a varrer (repetível; padrão: todos os ativos)")
	return c
}
