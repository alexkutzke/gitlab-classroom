// Package cli monta a árvore de comandos do classroom.
package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	gl "github.com/alexkutzke/gitlab-classroom/internal/gitlab"
	"github.com/alexkutzke/gitlab-classroom/internal/store"
	"github.com/alexkutzke/gitlab-classroom/internal/tui"
	"github.com/alexkutzke/gitlab-classroom/internal/turma"
)

var (
	dirFlag   string
	tokenFlag string
)

// Executar roda o comando raiz.
func Executar() error {
	raiz := &cobra.Command{
		Use:   "classroom",
		Short: "Coleta e acompanhamento das entregas de exercícios no GitLab",
		Long: "classroom acompanha os forks que os alunos fazem dos repositórios de\n" +
			"exercício no gitlab.com e guarda o resultado em arquivos texto dentro de\n" +
			".classroom/, na pasta da turma.\n\n" +
			"Sem subcomando, abre a interface interativa. Os subcomandos continuam\n" +
			"valendo e são o caminho para scripts.\n\n" +
			"Os comandos descobrem a turma subindo a árvore de diretórios a partir da\n" +
			"pasta atual, do mesmo modo que o git encontra o .git.",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return abrirInterface(cmd)
		},
	}
	raiz.PersistentFlags().StringVar(&dirFlag, "dir", "",
		"pasta da turma (padrão: procura .classroom/ a partir da pasta atual)")
	raiz.PersistentFlags().StringVar(&tokenFlag, "token", "",
		"token de acesso ao GitLab (padrão: GITLAB_TOKEN, chaveiro ou config)")

	raiz.AddCommand(
		cmdInit(),
		cmdSync(),
		cmdAlunos(),
		cmdExercicios(),
		cmdColetar(),
		cmdEquipes(),
		cmdClonar(),
		cmdAbrir(),
		cmdStatus(),
		cmdVerificar(),
		cmdCorrigir(),
		cmdDevolutiva(),
		cmdNota(),
		cmdNotas(),
		cmdRelatorio(),
		cmdToken(),
	)
	return raiz.Execute()
}

// abrir localiza a turma e carrega seus dados.
func abrir() (*store.Store, *turma.Turma, error) {
	inicio := dirFlag
	if inicio == "" {
		var err error
		if inicio, err = os.Getwd(); err != nil {
			return nil, nil, err
		}
	}
	s, err := store.Descobrir(inicio)
	if err != nil {
		return nil, nil, err
	}
	t, err := s.Carregar()
	if err != nil {
		return nil, nil, err
	}
	return s, t, nil
}

// pastaDestino devolve onde criar o .classroom/ no comando init.
func pastaDestino() (string, error) {
	if dirFlag != "" {
		return dirFlag, nil
	}
	return os.Getwd()
}

// cliente abre a conexão com o GitLab com o token resolvido.
func cliente(c turma.Config) (gl.Cliente, error) {
	token, err := gl.ResolverToken(tokenFlag, c.TokenArquivo)
	if err != nil {
		return nil, err
	}
	return gl.Novo(c.Host, token)
}

// agora existe para os testes poderem congelar o relógio no futuro.
var agora = time.Now

// abrirInterface sobe a TUI. Fora de um terminal, imprime a ajuda: sem isso,
// `classroom | less` e as invocações de script tentariam desenhar tela.
func abrirInterface(cmd *cobra.Command) error {
	info, err := os.Stdout.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice == 0 {
		return cmd.Help()
	}
	s, t, err := abrir()
	if err != nil {
		return err
	}
	return tui.Executar(s, t, func() (gl.Cliente, error) { return cliente(t.Config) })
}

func avisar(formato string, args ...any) {
	fmt.Fprintf(os.Stderr, formato+"\n", args...)
}
