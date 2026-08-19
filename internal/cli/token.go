package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	gl "github.com/alexkutzke/gitlab-classroom/internal/gitlab"
)

func cmdToken() *cobra.Command {
	return &cobra.Command{
		Use:   "token",
		Short: "Mostra de onde vem o token de acesso e testa a conexão",
		Long: "Diz qual fonte forneceu o token e confirma que ele fala com o GitLab. O\n" +
			"valor do token nunca é exibido.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			fonte := "nenhuma"
			switch {
			case strings.TrimSpace(tokenFlag) != "":
				fonte = "opção --token"
			case strings.TrimSpace(os.Getenv(gl.VarAmbiente)) != "":
				fonte = "variável " + gl.VarAmbiente
			}

			cfg, arquivo := "", ""
			if _, t, err := abrir(); err == nil {
				cfg = t.Config.Host
				arquivo = t.Config.TokenArquivo
			}

			token, err := gl.ResolverToken(tokenFlag, arquivo)
			if err != nil {
				return err
			}
			if fonte == "nenhuma" {
				fonte = "chaveiro do sistema (secret-tool)"
				if arquivo != "" {
					fonte += " ou " + arquivo
				}
			}
			fmt.Printf("Token obtido de: %s\n", fonte)

			if cfg == "" {
				cfg = "https://gitlab.com"
			}
			cli, err := gl.Novo(cfg, token)
			if err != nil {
				return err
			}
			grupos, err := cli.GruposDoProfessor()
			if err != nil {
				return err
			}
			fmt.Printf("Conexão com %s em ordem: %d grupo(s) com acesso de reporter ou mais.\n",
				cfg, len(grupos))
			return nil
		},
	}
}
