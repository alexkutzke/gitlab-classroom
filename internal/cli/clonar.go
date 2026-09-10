package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alexkutzke/gitlab-classroom/internal/acoes"
	"github.com/alexkutzke/gitlab-classroom/internal/repo"
)

func cmdClonar() *cobra.Command {
	var ids []string
	var soEntregues bool

	c := &cobra.Command{
		Use:   "clonar",
		Short: "Baixa os forks dos alunos para corrigir",
		Long: "Clona, ou atualiza se já existir, o fork de cada aluno e deixa o clone\n" +
			"posicionado no commit avaliado, sob o ramo local entrega/<exercicio>.\n\n" +
			"Depende de uma coleta anterior: é ela que sabe onde está o fork e qual\n" +
			"commit vale. A autenticação é por SSH, com a chave que o git já usa.",
		Example: "  classroom clonar --exercicio html\n" +
			"  classroom clonar --exercicio html --so-entregues",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, t, err := abrir()
			if err != nil {
				return err
			}
			exercicios, err := acoes.EscolherExercicios(t, ids)
			if err != nil {
				return err
			}

			res, err := acoes.Clonar(cmd.Context(), t, s.Pasta(), exercicios,
				acoes.OpcoesClone{SoEntregues: soEntregues}, progressoTerminal())
			if err != nil {
				return err
			}
			limparProgresso()

			for _, f := range res.Falhas {
				fmt.Printf("%s %s: %v\n", f.GRR, primeiroNome(f.Nome), f.Erro)
			}
			fmt.Printf("%d clonado(s), %d atualizado(s), %d com falha.\n",
				res.Novos, res.Atualizados, len(res.Falhas))
			if res.Base != "" && res.Novos+res.Atualizados > 0 {
				fmt.Printf("Os clones estão em %s\n", res.Base)
			}
			return nil
		},
	}
	c.Flags().StringArrayVar(&ids, "exercicio", nil, "exercício a clonar (repetível; padrão: todos os ativos)")
	c.Flags().BoolVar(&soEntregues, "so-entregues", false, "ignorar quem não entregou no prazo")
	return c
}

func cmdAbrir() *cobra.Command {
	var id, grr string
	var web bool

	c := &cobra.Command{
		Use:   "abrir",
		Short: "Abre o clone de um aluno no editor, ou o projeto no navegador",
		Long: "Sem --web, abre a pasta do clone no $EDITOR. Com --web, abre a página do\n" +
			"projeto no GitLab.",
		Example: "  classroom abrir --exercicio html --grr GRR20259001\n" +
			"  classroom abrir --exercicio html --grr GRR20259001 --web",
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

			if web {
				en, ok := t.Entrega(e.ID, a.GRR)
				if !ok || en.Projeto == "" {
					return fmt.Errorf("nenhum fork coletado para %s em %s", a.GRR, e.ID)
				}
				url := strings.TrimSuffix(t.Config.Host, "/") + "/" + en.Projeto
				return AbrirNoNavegador(url)
			}

			dir := acoes.DirDaEntrega(t, s.Pasta(), e.ID, *a)
			if !repo.Existe(dir) {
				return fmt.Errorf("clone ausente em %s: rode `classroom clonar --exercicio %s`", dir, e.ID)
			}
			return AbrirNoEditor(dir)
		},
	}
	c.Flags().StringVar(&id, "exercicio", "", "exercício")
	c.Flags().StringVar(&grr, "grr", "", "aluno")
	c.Flags().BoolVar(&web, "web", false, "abrir a página do projeto no GitLab")
	c.MarkFlagRequired("exercicio")
	c.MarkFlagRequired("grr")
	return c
}

// AbrirNoEditor abre o caminho no $EDITOR, ou no gerenciador de arquivos do
// sistema quando não há editor configurado.
func AbrirNoEditor(caminho string) error {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor == "" {
		return AbrirNoNavegador(caminho)
	}
	partes := strings.Fields(editor)
	cmd := exec.Command(partes[0], append(partes[1:], filepath.Clean(caminho))...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

// AbrirNoNavegador entrega o alvo ao xdg-open.
func AbrirNoNavegador(alvo string) error {
	if _, err := exec.LookPath("xdg-open"); err != nil {
		fmt.Println(alvo)
		return nil
	}
	return exec.Command("xdg-open", alvo).Start()
}
