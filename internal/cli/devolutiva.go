package cli

import (
	"fmt"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/alexkutzke/gitlab-classroom/internal/acoes"
	"github.com/alexkutzke/gitlab-classroom/internal/turma"
)

func cmdDevolutiva() *cobra.Command {
	var ids, grrs []string
	var aplicar, refazer bool
	var prazo string

	c := &cobra.Command{
		Use:   "devolutiva",
		Short: "Publica o comentário da correção como issue no fork do aluno",
		Long: "O comentário escrito na correção fica em notas.csv e não chega ao aluno.\n" +
			"Este comando publica cada um como issue no fork de quem entregou, onde\n" +
			"ele fica ao lado do código a que se refere.\n\n" +
			"A issue não leva nota: o registro da nota continua sendo o diario e o\n" +
			"UFPR Virtual. A menção ao usuário no corpo é o que notifica o aluno, e a\n" +
			"issue fica aberta para ele responder.\n\n" +
			"Sem --aplicar, nada é enviado ao GitLab: o ensaio é o padrão. Sem\n" +
			"--exercicio, o comando apenas lista o que já foi publicado.\n\n" +
			"Publicar é escrita, e escrita exige token com escopo api; o read_api da\n" +
			"coleta não abre issue.",
		Example: "  classroom devolutiva\n" +
			"  classroom devolutiva --exercicio html\n" +
			"  classroom devolutiva --exercicio html --aplicar --prazo 2026-10-05\n" +
			"  classroom devolutiva --exercicio html --grr GRR20259001 --refazer --aplicar",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, t, err := abrir()
			if err != nil {
				return err
			}
			if len(ids) == 0 {
				listarDevolutivas(t)
				return nil
			}

			exercicios, err := acoes.EscolherExercicios(t, ids)
			if err != nil {
				return err
			}
			data, err := turma.ParseData(prazo)
			if err != nil {
				return err
			}
			cli, err := cliente(t.Config)
			if err != nil {
				return err
			}

			res, erroRodada := acoes.Devolutivas(cmd.Context(), t, cli, exercicios,
				acoes.OpcoesDevolutiva{
					GRRs: grrs, Aplicar: aplicar, Refazer: refazer, Prazo: data,
				}, progressoTerminal())
			limparProgresso()
			relatarDevolutivas(res, aplicar)

			// O que já foi publicado precisa chegar ao arquivo mesmo que a
			// rodada tenha parado no meio: sem o registro, a rodada seguinte
			// abriria a issue de novo.
			if aplicar && res.Publicadas+res.Comentadas+res.Reconhecidas > 0 {
				if err := s.Gravar(t); err != nil {
					return err
				}
			}
			return erroRodada
		},
	}
	c.Flags().StringArrayVar(&ids, "exercicio", nil, "exercício a publicar (repetível; sem ele, o comando só lista)")
	c.Flags().StringArrayVar(&grrs, "grr", nil, "restringe a um ou mais alunos (repetível)")
	c.Flags().BoolVar(&aplicar, "aplicar", false, "publicar de fato; sem esta opção nada é enviado")
	c.Flags().BoolVar(&refazer, "refazer", false, "republicar quem já tem issue, como comentário novo nela")
	c.Flags().StringVar(&prazo, "prazo", "", "data até quando o aluno pode comentar (AAAA-MM-DD)")
	return c
}

// listarDevolutivas mostra o que já foi publicado, com a marca de quem teve o
// comentário alterado depois.
func listarDevolutivas(t *turma.Turma) {
	if len(t.Devolutivas) == 0 {
		fmt.Println("Nenhuma devolutiva publicada.")
		fmt.Println("Publique com `classroom devolutiva --exercicio <id> --aplicar`.")
		return
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "EXERCÍCIO\tALUNO\tISSUE\tPUBLICADA EM\tURL")
	desatualizadas := 0
	for _, d := range t.Devolutivas {
		aluno, _ := t.AlunoPorGRR(d.GRR)
		marca := ""
		if n, ok := t.Nota(d.Exercicio, d.GRR); ok && d.Desatualizada(n.Comentario) {
			marca, desatualizadas = " !", desatualizadas+1
		}
		fmt.Fprintf(tw, "%s\t%s\t#%d%s\t%s\t%s\n",
			d.Exercicio, nomeOuGRR(aluno, d.GRR), d.Issue, marca,
			d.PublicadoEm.Format("02/01 15:04"), d.URL)
	}
	tw.Flush()
	if desatualizadas > 0 {
		fmt.Println()
		avisar("%d devolutiva(s) marcada(s) com ! tiveram o comentário alterado depois de publicadas. "+
			"Use --refazer para acrescentar o texto novo à issue.", desatualizadas)
	}
}

// relatarDevolutivas imprime o que a rodada fez, ou faria.
func relatarDevolutivas(res acoes.ResumoDevolutiva, aplicar bool) {
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	publicaveis := 0
	for _, it := range res.Itens {
		if it.Acao == acoes.DevolutivaPular {
			continue
		}
		publicaveis++
		destino := it.Projeto
		if it.Issue > 0 {
			destino = fmt.Sprintf("%s #%d", it.Projeto, it.Issue)
		}
		situacao := string(it.Acao)
		if it.Erro != "" {
			situacao = "erro: " + it.Erro
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", it.Exercicio, it.Nome, destino, situacao)
	}
	tw.Flush()
	if publicaveis == 0 {
		fmt.Println("Nada a publicar.")
	}

	if len(res.Fora) > 0 {
		fmt.Println("\nFora da rodada:")
		motivos := make([]string, 0, len(res.Fora))
		for m := range res.Fora {
			motivos = append(motivos, m)
		}
		sort.Strings(motivos)
		for _, m := range motivos {
			fmt.Printf("  %-32s %d\n", m, res.Fora[m])
		}
	}

	fmt.Println()
	if aplicar {
		fmt.Printf("%d issue(s) aberta(s), %d comentada(s), %d reconhecida(s).\n",
			res.Publicadas, res.Comentadas, res.Reconhecidas)
	} else {
		fmt.Printf("Ensaio: %d issue(s) a abrir, %d a comentar, %d já existentes a registrar.\n",
			res.Publicadas, res.Comentadas, res.Reconhecidas)
		fmt.Println("Nada foi enviado ao GitLab. Repita com --aplicar para publicar.")
	}
	for _, e := range res.Erros {
		avisar("  %s", e)
	}
}
