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
			login, err := cli.Eu()
			if err != nil {
				return err
			}
			fmt.Printf("Conexão com %s em ordem, autenticado como %s.\n", cfg, login)

			// Dentro de uma turma, vale conferir também o que a coleta usa: a
			// listagem dos grupos dela em que você é reporter.
			if _, t, err := abrir(); err == nil {
				if prefixo := t.Config.PrefixoGrupo(); prefixo != "" {
					grupos, err := cli.GruposComAcesso(prefixo)
					if err != nil {
						return err
					}
					fmt.Printf("Grupos de %s com o seu acesso: %d.\n", prefixo, len(grupos))
				}
			}
			return nil
		},
	}
}
