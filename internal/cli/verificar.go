package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/alexkutzke/gitlab-classroom/internal/repo"
	"github.com/alexkutzke/gitlab-classroom/internal/turma"
	"github.com/alexkutzke/gitlab-classroom/internal/verificacao"
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

			entregas := t.EntregasDoExercicio(e.ID)
			// Uma entrega em dupla é um fork só: a suíte roda uma vez, e o
			// resultado vale para todos os integrantes.
			var alvos []verificacao.Alvo
			porDono := map[string][]string{}
			for _, a := range t.Ativos() {
				if grr != "" && !strings.EqualFold(a.GRR, grr) {
					continue
				}
				dono := donoDaEntrega(t, e.ID, a)
				if _, visto := porDono[dono.GRR]; !visto {
					alvos = append(alvos, verificacao.Alvo{
						GRR:    dono.GRR,
						Nome:   dono.Nome,
						Dir:    repo.Caminho(s.Pasta(), t.Config.PastaEntregas, e.ID, dono),
						Commit: entregas[dono.GRR].Commit,
					})
				}
				porDono[dono.GRR] = append(porDono[dono.GRR], a.GRR)
			}
			if len(alvos) == 0 {
				return fmt.Errorf("nenhum aluno a verificar")
			}

			if semSandbox && !sim {
				if err := confirmarExecucaoDireta(e); err != nil {
					return err
				}
			}

			opts := verificacao.Opcoes{
				Exercicio:   *e,
				Imagem:      imagem,
				TempoLimite: time.Duration(segundos) * time.Second,
				SemSandbox:  semSandbox,
				Escrita:     escrita,
				Runtime:     runtime,
				PastaLogs:   filepath.Join(repo.Base(s.Pasta(), t.Config.PastaEntregas, e.ID), ".logs"),
			}
			if opts.TempoLimite <= 0 {
				opts.TempoLimite = time.Duration(t.Config.TempoLimiteVerificacao) * time.Second
			}
			if opts.Imagem == "" && e.Imagem == "" {
				opts.Imagem = t.Config.ImagemVerificacao
			}

			total := len(alvos)
			res, err := verificacao.Executar(alvos, opts, t.Config.Paralelismo,
				func(feito, _ int, a verificacao.Alvo) {
					fmt.Printf("\r%d/%d  %-40s", feito, total, primeiroNome(a.Nome))
				})
			if err != nil {
				return err
			}
			fmt.Print("\r\033[K")

			contagem := map[turma.SituacaoVerificacao]int{}
			for _, v := range res {
				if v.Situacao == turma.Reprovado || v.Situacao == turma.ErroVerificacao {
					a, _ := t.AlunoPorGRR(v.GRR)
					nome := v.GRR
					if a != nil {
						nome = a.Nome
					}
					fmt.Printf("  %-40s %s\n", nome, v.Resumo())
				}
				// O resultado do fork vale para cada integrante da entrega,
				// e assim o relatório continua tendo uma linha por aluno.
				for _, integrante := range porDono[v.GRR] {
					contagem[v.Situacao]++
					if !dryRun {
						copia := v
						copia.GRR = integrante
						t.RegistrarVerificacao(copia)
					}
				}
			}
			fmt.Printf("%d aprovado(s), %d reprovado(s), %d sem clone, %d com erro.\n",
				contagem[turma.Aprovado], contagem[turma.Reprovado],
				contagem[turma.SemClone], contagem[turma.ErroVerificacao])
			fmt.Printf("Saída completa em %s\n", opts.PastaLogs)

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
