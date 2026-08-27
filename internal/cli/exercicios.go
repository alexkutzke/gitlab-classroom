package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alexkutzke/gitlab-classroom/internal/relatorio"
	"github.com/alexkutzke/gitlab-classroom/internal/turma"
)

func cmdExercicios() *cobra.Command {
	var todos bool

	c := &cobra.Command{
		Use:     "exercicios",
		Short:   "Lista e mantém os exercícios da disciplina",
		Args:    cobra.NoArgs,
		Example: "  classroom exercicios\n  classroom exercicios add --repo ds122-html-assignment --prazo 2026-09-05",
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, t, err := abrir()
			if err != nil {
				return err
			}
			es := t.ExerciciosAtivos()
			if todos {
				es = t.Exercicios
			}
			if len(es) == 0 {
				fmt.Println("Nenhum exercício cadastrado. Use `classroom exercicios add`.")
				return nil
			}
			relatorio.Exercicios(os.Stdout, es)
			return nil
		},
	}
	c.Flags().BoolVar(&todos, "todos", false, "incluir os exercícios arquivados")
	c.AddCommand(cmdExerciciosAdd(), cmdExerciciosEditar(), cmdExerciciosArquivar())
	return c
}

func cmdExerciciosAdd() *cobra.Command {
	var id, repo, titulo, prazo, verificacao, imagem string
	var peso float64
	var ordem int

	c := &cobra.Command{
		Use:   "add",
		Short: "Cadastra um exercício",
		Long: "O repositório é o nome do projeto-modelo dentro do namespace da\n" +
			"disciplina, que é também o nome do fork de cada aluno. Sem --id, o\n" +
			"apelido curto é derivado do nome do repositório.",
		Example: "  classroom exercicios add --repo ds122-html-assignment --prazo 2026-09-05\n" +
			"  classroom exercicios add --repo ds122-prepare-assignment --prazo 2026-08-15 --id prepare",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, t, err := abrir()
			if err != nil {
				return err
			}
			p, err := turma.ParseData(prazo)
			if err != nil {
				return err
			}
			if id == "" {
				id = idDoRepo(repo, t.Config.Codigo)
			}
			if _, existe := t.Exercicio(id); existe {
				return fmt.Errorf("já existe exercício com id %q: use `classroom exercicios editar`", id)
			}
			e := turma.Exercicio{
				ID: id, Repo: repo, Titulo: titulo, Prazo: p, Peso: peso, Ordem: ordem,
				Verificacao: verificacao, Imagem: imagem, Situacao: turma.ExercicioAtivo,
			}
			if err := e.Validar(); err != nil {
				return err
			}
			t.RegistrarExercicio(e)
			if err := s.Gravar(t); err != nil {
				return err
			}
			fmt.Printf("Exercício %s cadastrado: %s, prazo %s.\n",
				e.ID, t.Config.CaminhoModelo(e.Repo), e.Prazo.String())
			return nil
		},
	}
	c.Flags().StringVar(&id, "id", "", "apelido curto usado na linha de comando")
	c.Flags().StringVar(&repo, "repo", "", "nome do repositório-modelo")
	c.Flags().StringVar(&titulo, "titulo", "", "título exibido nos relatórios")
	c.Flags().StringVar(&prazo, "prazo", "", "data de entrega, em AAAA-MM-DD")
	c.Flags().Float64Var(&peso, "peso", 1, "peso do exercício na média")
	c.Flags().IntVar(&ordem, "ordem", 0, "desempate na tabela quando dois exercícios têm o mesmo prazo")
	c.Flags().StringVar(&verificacao, "verificacao", "", "comando da suíte automatizada, relativo à raiz do repositório")
	c.Flags().StringVar(&imagem, "imagem", "", "imagem do contêiner onde a suíte roda")
	c.MarkFlagRequired("repo")
	c.MarkFlagRequired("prazo")
	return c
}

func cmdExerciciosEditar() *cobra.Command {
	var id, repo, titulo, prazo, verificacao, imagem string
	var peso float64
	var ordem int

	c := &cobra.Command{
		Use:     "editar",
		Short:   "Altera um exercício já cadastrado",
		Example: "  classroom exercicios editar --id html --prazo 2026-09-12",
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
			if cmd.Flags().Changed("repo") {
				e.Repo = repo
			}
			if cmd.Flags().Changed("titulo") {
				e.Titulo = titulo
			}
			if cmd.Flags().Changed("peso") {
				e.Peso = peso
			}
			if cmd.Flags().Changed("ordem") {
				e.Ordem = ordem
			}
			if cmd.Flags().Changed("verificacao") {
				e.Verificacao = verificacao
			}
			if cmd.Flags().Changed("imagem") {
				e.Imagem = imagem
			}
			if cmd.Flags().Changed("prazo") {
				p, err := turma.ParseData(prazo)
				if err != nil {
					return err
				}
				e.Prazo = p
				avisar("Prazo alterado: recolete com `classroom coletar --exercicio %s` para reclassificar as entregas.", e.ID)
			}
			if err := e.Validar(); err != nil {
				return err
			}
			if err := s.Gravar(t); err != nil {
				return err
			}
			fmt.Printf("Exercício %s atualizado.\n", e.ID)
			return nil
		},
	}
	c.Flags().StringVar(&id, "id", "", "exercício a alterar")
	c.Flags().StringVar(&repo, "repo", "", "nome do repositório-modelo")
	c.Flags().StringVar(&titulo, "titulo", "", "título exibido nos relatórios")
	c.Flags().StringVar(&prazo, "prazo", "", "data de entrega, em AAAA-MM-DD")
	c.Flags().Float64Var(&peso, "peso", 1, "peso do exercício na média")
	c.Flags().IntVar(&ordem, "ordem", 0, "desempate na tabela quando dois exercícios têm o mesmo prazo")
	c.Flags().StringVar(&verificacao, "verificacao", "", "comando da suíte automatizada")
	c.Flags().StringVar(&imagem, "imagem", "", "imagem do contêiner onde a suíte roda")
	c.MarkFlagRequired("id")
	return c
}

func cmdExerciciosArquivar() *cobra.Command {
	var id string
	var reativar bool

	c := &cobra.Command{
		Use:   "arquivar",
		Short: "Tira um exercício das coletas e relatórios correntes",
		Long: "O exercício continua no arquivo com as entregas já coletadas. Nada é\n" +
			"apagado: arquivar só o deixa de fora do que a ferramenta olha por padrão.",
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
			if reativar {
				e.Situacao = turma.ExercicioAtivo
			} else {
				e.Situacao = turma.ExercicioArquivado
			}
			if err := s.Gravar(t); err != nil {
				return err
			}
			fmt.Printf("Exercício %s agora está %s.\n", e.ID, e.Situacao)
			return nil
		},
	}
	c.Flags().StringVar(&id, "id", "", "exercício a arquivar")
	c.Flags().BoolVar(&reativar, "reativar", false, "voltar o exercício para ativo")
	c.MarkFlagRequired("id")
	return c
}

// idDoRepo deriva o apelido curto do nome do repositório, tirando o prefixo
// do código da disciplina e o sufixo -assignment, que se repetem em todos.
func idDoRepo(repo, codigo string) string {
	id := strings.ToLower(strings.TrimSpace(repo))
	id = strings.TrimPrefix(id, strings.ToLower(codigo)+"-")
	id = strings.TrimSuffix(id, "-assignment")
	return id
}
