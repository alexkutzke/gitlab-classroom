package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alexkutzke/gitlab-classroom/internal/acoes"
	"github.com/alexkutzke/gitlab-classroom/internal/diario"
	gl "github.com/alexkutzke/gitlab-classroom/internal/gitlab"
	"github.com/alexkutzke/gitlab-classroom/internal/turma"
)

func cmdSync() *cobra.Command {
	var pastaDiario string
	var semGitLab, semDiario, dryRun bool

	c := &cobra.Command{
		Use:   "sync",
		Short: "Atualiza o cadastro e reconcilia as contas com o GitLab",
		Long: "Reimporta o cadastro de alunos do .diario/ e procura, no GitLab, o grupo\n" +
			"de cada um. Nome, e-mail e situação vêm do SIGA; usuário, grupo e situação\n" +
			"da conta são apurados aqui.\n\n" +
			"Ninguém é apagado: quem sai da lista do SIGA vira cancelado e conserva as\n" +
			"entregas já coletadas.",
		Example: "  classroom sync\n  classroom sync --sem-gitlab\n  classroom sync --dry-run",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, t, err := abrir()
			if err != nil {
				return err
			}
			if pastaDiario == "" {
				pastaDiario = t.Config.PastaDiario
			}

			var cli gl.Cliente
			if !semGitLab {
				if cli, err = cliente(t.Config); err != nil {
					return err
				}
			}

			res, err := acoes.Sincronizar(cmd.Context(), t, s.Pasta(), cli, acoes.OpcoesSync{
				PastaDiario: pastaDiario,
				SemDiario:   semDiario,
				SemGitLab:   semGitLab,
			}, progressoTerminal())
			if err != nil {
				return err
			}
			limparProgresso()

			if !semDiario {
				if res.SemCadastro != "" {
					avisar("Aviso: %s. O cadastro atual foi mantido.", res.SemCadastro)
				} else {
					relatarImportacao(res.Importacao)
				}
			}
			if !semGitLab {
				relatarContas(t, res.Alunos)
			}

			if dryRun {
				fmt.Println("Nada foi gravado (--dry-run).")
				return nil
			}
			return s.Gravar(t)
		},
	}
	c.Flags().StringVar(&pastaDiario, "diario", "", "pasta do diario, relativa à da turma")
	c.Flags().BoolVar(&semGitLab, "sem-gitlab", false, "só reimportar o cadastro, sem consultar o GitLab")
	c.Flags().BoolVar(&semDiario, "sem-diario", false, "só reconciliar com o GitLab, sem reimportar o cadastro")
	c.Flags().BoolVar(&dryRun, "dry-run", false, "mostrar o que mudaria sem gravar")
	return c
}

func relatarImportacao(r diario.Resultado) {
	if r.Vazio() {
		fmt.Println("Cadastro do diario: nada mudou.")
		return
	}
	if len(r.Novos) > 0 {
		fmt.Printf("Novos: %s\n", strings.Join(r.Novos, ", "))
	}
	if len(r.Atualizados) > 0 {
		fmt.Printf("Atualizados: %s\n", strings.Join(r.Atualizados, ", "))
	}
	if len(r.Cancelados) > 0 {
		fmt.Printf("Cancelados: %s\n", strings.Join(r.Cancelados, ", "))
	}
}

func relatarContas(t *turma.Turma, alunos []turma.Aluno) {
	contagem := map[turma.SituacaoConta]int{}
	for _, a := range alunos {
		contagem[a.SituacaoConta]++
	}
	fmt.Printf("Contas verificadas: %d em ordem, %d com pendência.\n",
		contagem[turma.ContaOK], len(alunos)-contagem[turma.ContaOK])

	for _, a := range alunos {
		switch a.SituacaoConta {
		case turma.ContaOK:
			continue
		case turma.ContaGrupoDivergente:
			fmt.Printf("  %s %s: grupo %s, fora do padrão %s\n",
				a.GRR, a.Nome, a.Grupo, t.Config.CaminhoGrupo(a.GRR))
		case turma.ContaSemAcesso:
			fmt.Printf("  %s %s: grupo %s existe, mas você não está associado como reporter\n",
				a.GRR, a.Nome, a.Grupo)
		case turma.ContaSemUsuario:
			fmt.Printf("  %s %s: nenhum usuário %s no GitLab\n",
				a.GRR, a.Nome, turma.UsuarioGitLab(a.GRR))
		default:
			fmt.Printf("  %s %s: grupo não criado ou não compartilhado (esperado %s)\n",
				a.GRR, a.Nome, t.Config.CaminhoGrupo(a.GRR))
		}
	}
}

// progressoTerminal devolve o aviso de progresso, que reescreve a mesma linha
// para não encher a tela numa turma de trinta alunos.
func progressoTerminal() acoes.AvisoProgresso {
	return func(p acoes.Progresso) {
		fmt.Printf("\r%d/%d  %-40s", p.Feito, p.Total, primeiroNome(p.Rotulo))
	}
}

// limparProgresso apaga a linha de progresso antes do relatório final.
func limparProgresso() { fmt.Print("\r\033[K") }

func primeiroNome(nome string) string {
	if i := strings.IndexByte(nome, ' '); i > 0 {
		return nome[:i]
	}
	return nome
}
