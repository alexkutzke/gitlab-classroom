package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/alexkutzke/gitlab-classroom/internal/acoes"
	"github.com/alexkutzke/gitlab-classroom/internal/turma"
)

func cmdVerificar() *cobra.Command {
	var id, grr, imagem, runtime string
	var segundos int
	var semSandbox, sim, escrita, dryRun bool

	c := &cobra.Command{
		Use:   "verificar",
		Short: "Roda a suíte automatizada do exercício sobre os clones",
		Long: "Executa, para cada aluno, o comando cadastrado em --verificacao do\n" +
			"exercício, dentro do clone local. Exercício sem suíte cadastrada não é\n" +
			"verificado, e isso não prejudica nota: significa que a correção é toda\n" +
			"à mão.\n\n" +
			"A execução padrão é em contêiner, sem rede, com o clone montado somente\n" +
			"para leitura e com teto de memória, de processos e de tempo. Rodar código\n" +
			"de aluno direto na máquina é o que --sem-sandbox faz, e ele pede\n" +
			"confirmação.\n\n" +
			"A suíte informa quantos casos passaram imprimindo uma linha no formato\n" +
			"RESULTADO: 7/10. Sem essa linha, vale o código de saída: zero aprova.",
		Example: "  classroom verificar --exercicio html\n" +
			"  classroom verificar --exercicio html --imagem docker.io/library/node:22-alpine\n" +
			"  classroom verificar --exercicio html --grr GRR20259001",
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
			if !e.TemSuite() {
				return fmt.Errorf(
					"exercício %s não tem suíte; cadastre com `classroom exercicios editar --id %s --verificacao <comando>`",
					e.ID, e.ID)
			}

			if semSandbox && !sim {
				if err := confirmarExecucaoDireta(e); err != nil {
					return err
				}
			}

			res, err := acoes.Verificar(t, s.Pasta(), *e, acoes.OpcoesVerificacao{
				GRR:         grr,
				Imagem:      imagem,
				Runtime:     runtime,
				TempoLimite: time.Duration(segundos) * time.Second,
				SemSandbox:  semSandbox,
				Escrita:     escrita,
			}, progressoTerminal())
			if err != nil {
				return err
			}
			limparProgresso()

			for _, v := range res.Problemas {
				a, _ := t.AlunoPorGRR(v.GRR)
				fmt.Printf("  %-40s %s\n", nomeOuGRR(a, v.GRR), v.Resumo())
			}
			contagem := res.Contagem
			fmt.Printf("%d aprovado(s), %d reprovado(s), %d sem clone, %d com erro.\n",
				contagem[turma.Aprovado], contagem[turma.Reprovado],
				contagem[turma.SemClone], contagem[turma.ErroVerificacao])
			fmt.Printf("Saída completa em %s\n", res.PastaLogs)

			if dryRun {
				fmt.Println("Nada foi gravado (--dry-run).")
				return nil
			}
			return s.Gravar(t)
		},
	}
	c.Flags().StringVar(&id, "exercicio", "", "exercício a verificar")
	c.Flags().StringVar(&grr, "grr", "", "verificar só um aluno")
	c.Flags().StringVar(&imagem, "imagem", "", "imagem do contêiner (padrão: a do exercício ou a do config.toml)")
	c.Flags().StringVar(&runtime, "runtime", "", "programa de contêiner (padrão: podman, depois docker)")
	c.Flags().IntVar(&segundos, "tempo", 0, "tempo limite de cada execução, em segundos")
	c.Flags().BoolVar(&semSandbox, "sem-sandbox", false, "rodar direto na máquina, sem contêiner")
	c.Flags().BoolVar(&sim, "sim", false, "confirmar de antemão a execução sem sandbox")
	c.Flags().BoolVar(&escrita, "escrita", false, "montar o clone com permissão de escrita")
	c.Flags().BoolVar(&dryRun, "dry-run", false, "mostrar o resultado sem gravar")
	c.MarkFlagRequired("exercicio")
	return c
}

// confirmarExecucaoDireta exige um sim digitado antes de rodar código de aluno
// fora do contêiner.
func confirmarExecucaoDireta(e *turma.Exercicio) error {
	info, err := os.Stdin.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice == 0 {
		return fmt.Errorf("--sem-sandbox fora de um terminal exige --sim")
	}
	fmt.Printf("O comando %q de %s vai rodar direto nesta máquina, com os seus\n"+
		"privilégios e acesso à rede, sobre código escrito pelos alunos.\n",
		e.Verificacao, e.ID)
	fmt.Print("Digite sim para continuar: ")

	linha, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return err
	}
	if strings.TrimSpace(strings.ToLower(linha)) != "sim" {
		return fmt.Errorf("execução cancelada")
	}
	return nil
}
