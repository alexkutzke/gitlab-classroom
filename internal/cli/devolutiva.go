package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/alexkutzke/gitlab-classroom/internal/acoes"
	"github.com/alexkutzke/gitlab-classroom/internal/turma"
)

func cmdDevolutiva() *cobra.Command {
	var ids, grrs []string
	var aplicar, refazer, texto, confirmar bool
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
			"--texto mostra, no ensaio, o corpo que cada aluno receberia, e\n" +
			"--aplicar --confirmar percorre a turma um a um, com a chance de editar\n" +
			"o comentário antes de publicar.\n\n" +
			"Publicar é escrita, e escrita exige token com escopo api; o read_api da\n" +
			"coleta não abre issue.",
		Example: "  classroom devolutiva\n" +
			"  classroom devolutiva --exercicio html\n" +
			"  classroom devolutiva --exercicio html --texto\n" +
			"  classroom devolutiva --exercicio html --aplicar --confirmar --prazo 2026-10-05\n" +
			"  classroom devolutiva --exercicio html --aplicar --prazo 2026-10-05\n" +
			"  classroom devolutiva --exercicio html --grr GRR20259001 --refazer --aplicar",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if confirmar {
				if !aplicar {
					return fmt.Errorf("--confirmar revisa antes de publicar e só vale com --aplicar")
				}
				if err := exigeTerminal(os.Stdin); err != nil {
					return err
				}
			}
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

			o := acoes.OpcoesDevolutiva{
				GRRs: grrs, Aplicar: aplicar, Refazer: refazer, Prazo: data,
			}
			if aplicar {
				// Gravar a cada publicação, e não ao final: queda de rede
				// ou Ctrl+C no meio não pode perder o registro do que já
				// foi para o GitLab, senão a rodada seguinte republica.
				o.Gravar = func() error { return s.Gravar(t) }
			}
			if confirmar {
				o.Confirmar = revisorTerminal(os.Stdin)
				o.Editar = editarComentario
			}

			// A revisão já escreve na tela, e o progresso por cima
			// atravessaria o texto que está sendo conferido.
			prog := progressoTerminal()
			if confirmar {
				prog = nil
			}
			res, erroRodada := acoes.Devolutivas(cmd.Context(), t, cli, exercicios, o, prog)
			if !confirmar {
				limparProgresso()
			}
			if texto {
				imprimirTextos(res)
			}
			relatarDevolutivas(res, aplicar, texto)

			// Fora do caminho incremental, o que foi publicado ainda precisa
			// chegar ao arquivo antes de o processo terminar.
			if aplicar && o.Gravar == nil && res.Publicadas+res.Comentadas+res.Reconhecidas > 0 {
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
	c.Flags().BoolVar(&texto, "texto", false, "mostra o corpo que cada aluno receberia, em vez da tabela")
	c.Flags().BoolVar(&confirmar, "confirmar", false, "revisa aluno a aluno antes de cada publicação (exige --aplicar)")
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

// relatarDevolutivas imprime o que a rodada fez, ou faria. Com texto, os
// corpos já foram impressos um a um e a tabela sai de cena.
func relatarDevolutivas(res acoes.ResumoDevolutiva, aplicar, texto bool) {
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	publicaveis := 0
	for _, it := range res.Itens {
		if it.Acao == acoes.DevolutivaPular {
			continue
		}
		publicaveis++
		if texto {
			continue
		}
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

// blocoDevolutiva monta o que aparece na tela para um aluno: o cabeçalho de
// identificação e o corpo que vai para a issue.
//
// O corpo vem pronto de acoes, e não é remontado aqui: se houvesse duas
// montagens, o ensaio deixaria de valer como conferência do que será
// publicado. A quebra em 72 colunas é só de tela; o markdown enviado ao
// GitLab vai sem quebra forçada, e o navegador reflui.
func blocoDevolutiva(it acoes.ItemDevolutiva) string {
	var b strings.Builder
	b.WriteString(strings.Repeat("─", larguraDevolutiva) + "\n")
	fmt.Fprintf(&b, "%s  %s\n", it.Nome, it.GRR)
	if len(it.Equipe) > 0 {
		fmt.Fprintf(&b, "em equipe com %s\n", strings.Join(it.Equipe, ", "))
	}
	fmt.Fprintf(&b, "%s\n\n", it.Projeto)
	if it.Issue > 0 {
		fmt.Fprintf(&b, "issue #%d já aberta\n\n", it.Issue)
	}
	fmt.Fprintf(&b, "%s\n\n", it.Titulo)
	b.WriteString(quebrar(it.Corpo, larguraTexto))
	return b.String()
}

const (
	larguraDevolutiva = 69
	larguraTexto      = 72
)

// quebrar dobra o texto em colunas, preservando as linhas em branco que
// separam os parágrafos do markdown.
func quebrar(texto string, largura int) string {
	var out []string
	for _, linha := range strings.Split(strings.TrimRight(texto, "\n"), "\n") {
		palavras := strings.Fields(linha)
		if len(palavras) == 0 {
			out = append(out, "")
			continue
		}
		atual := palavras[0]
		for _, p := range palavras[1:] {
			if len([]rune(atual))+1+len([]rune(p)) > largura {
				out = append(out, atual)
				atual = p
				continue
			}
			atual += " " + p
		}
		out = append(out, atual)
	}
	return strings.Join(out, "\n") + "\n"
}

// imprimirTextos mostra o corpo de cada aluno da rodada, em vez da tabela.
func imprimirTextos(res acoes.ResumoDevolutiva) {
	for _, it := range res.Itens {
		if it.Acao == acoes.DevolutivaPular {
			continue
		}
		fmt.Println(blocoDevolutiva(it))
	}
}

// revisorTerminal devolve a função que pergunta o que fazer com cada aluno.
func revisorTerminal(in io.Reader) func(acoes.ItemDevolutiva) (acoes.DecisaoDevolutiva, error) {
	leitor := bufio.NewReader(in)
	return func(it acoes.ItemDevolutiva) (acoes.DecisaoDevolutiva, error) {
		fmt.Println(blocoDevolutiva(it))
		for {
			fmt.Println("[s] publicar   [n] pular   [e] editar o comentário   " +
				"[t] publicar todas as restantes   [q] sair")
			fmt.Print("> ")
			linha, err := leitor.ReadString('\n')
			resposta := strings.ToLower(strings.TrimSpace(linha))
			if err != nil && resposta == "" {
				// Entrada fechada no meio da rodada vale por desistência:
				// publicar sem resposta é o contrário do que --confirmar
				// existe para garantir.
				fmt.Println()
				return acoes.DecisaoSair, nil
			}
			switch resposta {
			case "s":
				return acoes.DecisaoPublicar, nil
			case "n":
				return acoes.DecisaoPular, nil
			case "e":
				return acoes.DecisaoEditar, nil
			case "t":
				return acoes.DecisaoTodas, nil
			case "q":
				return acoes.DecisaoSair, nil
			}
			avisar("resposta %q desconhecida", resposta)
		}
	}
}

// editarComentario abre o comentário no $EDITOR e devolve o que foi salvo.
//
// O que vai para o editor é só o comentário: a menção e o rodapé do prazo são
// montados na publicação, e editá-los aqui os gravaria em notas.csv.
func editarComentario(comentario string) (string, error) {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor == "" {
		return "", fmt.Errorf("defina $EDITOR para editar o comentário")
	}
	f, err := os.CreateTemp("", "devolutiva-*.md")
	if err != nil {
		return "", err
	}
	nome := f.Name()
	defer os.Remove(nome)
	if _, err := f.WriteString(strings.TrimSpace(comentario) + "\n"); err != nil {
		f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}

	partes := strings.Fields(editor)
	cmd := exec.Command(partes[0], append(partes[1:], nome)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return "", err
	}
	novo, err := os.ReadFile(nome)
	if err != nil {
		return "", err
	}
	return string(novo), nil
}

// exigeTerminal recusa a revisão quando não há terminal para ler a resposta.
//
// Sem esta checagem, `--confirmar` num script ficaria esperando um stdin que
// nunca responde, com a rodada parada no primeiro aluno.
func exigeTerminal(in *os.File) error {
	info, err := in.Stat()
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeCharDevice != 0 {
		return nil
	}
	return fmt.Errorf("--confirmar pergunta aluno a aluno e precisa de terminal interativo.\n" +
		"Sem terminal, confira o texto com --texto e publique a rodada com --aplicar")
}
