// Package relatorio escreve no terminal o que foi coletado.
package relatorio

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/alexkutzke/gitlab-classroom/internal/turma"
)

func nova(w io.Writer) *tabwriter.Writer {
	return tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
}

// Cabecalho identifica a turma no topo de cada saída.
func Cabecalho(w io.Writer, t *turma.Turma) {
	c := t.Config
	fmt.Fprintf(w, "%s  %s  %s\n", c.Descricao(), c.Disciplina, c.Semestre)
	fmt.Fprintf(w, "%d aluno(s) ativo(s), %d exercício(s)\n\n", len(t.Ativos()), len(t.ExerciciosAtivos()))
}

// Alunos lista o cadastro com a situação da conta no GitLab.
func Alunos(w io.Writer, alunos []turma.Aluno) {
	tw := nova(w)
	fmt.Fprintln(tw, "GRR\tNOME\tUSUÁRIO\tGRUPO\tCONTA")
	for _, a := range alunos {
		conta := string(a.SituacaoConta)
		if conta == "" {
			conta = "não verificada"
		}
		grupo := a.Grupo
		if grupo == "" {
			grupo = "-"
		}
		nome := a.Nome
		if !a.EstaAtivo() {
			nome += " (cancelado)"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", a.GRR, nome, a.UsuarioEsperado(), grupo, conta)
	}
	tw.Flush()
}

// Exercicios lista os exercícios cadastrados.
func Exercicios(w io.Writer, es []turma.Exercicio) {
	tw := nova(w)
	fmt.Fprintln(tw, "ID\tREPOSITÓRIO\tPRAZO\tPESO\tVERIFICAÇÃO\tSITUAÇÃO\tTÍTULO")
	for _, e := range es {
		verif := e.Verificacao
		if verif == "" {
			verif = "-"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%g\t%s\t%s\t%s\n",
			e.ID, e.Repo, e.Prazo.String(), e.Peso, verif, e.Situacao, e.Titulo)
	}
	tw.Flush()
}

// Entregas detalha um exercício, aluno a aluno.
//
// A coluna de equipe existe porque a entrega em dupla tem um repositório só:
// sem ela, dois alunos apareceriam com o mesmo resultado sem explicação.
func Entregas(w io.Writer, t *turma.Turma, e turma.Exercicio, alunos []turma.Aluno, entregas map[string]turma.Entrega) {
	fmt.Fprintf(w, "%s  %s  prazo %s\n", e.ID, e.Titulo, e.Prazo.String())
	tw := nova(w)
	fmt.Fprintln(tw, "GRR\tALUNO\tSITUAÇÃO\tCOMMITS\tÚLTIMO COMMIT\tEQUIPE")
	for _, a := range alunos {
		en, ok := entregas[a.GRR]
		if !ok {
			fmt.Fprintf(tw, "%s\t%s\t%s\t\t\t\n", a.GRR, a.Nome, "não coletado")
			continue
		}
		ultimo := "-"
		if !en.DataUltimo.IsZero() {
			ultimo = turma.DataDe(en.DataUltimo).String()
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\t%s\n",
			a.GRR, a.Nome, en.Descricao(), en.Commits, ultimo, equipeDe(t, e.ID, a))
	}
	tw.Flush()
}

// equipeDe descreve com quem o aluno dividiu a entrega.
func equipeDe(t *turma.Turma, exercicio string, a turma.Aluno) string {
	equipe := t.Equipe(exercicio, a.GRR)
	if len(equipe) < 2 {
		return ""
	}
	var outros []string
	for _, grr := range equipe {
		if grr == a.GRR {
			continue
		}
		if colega, ok := t.AlunoPorGRR(grr); ok {
			outros = append(outros, colega.Nome)
			continue
		}
		outros = append(outros, grr)
	}
	return "com " + strings.Join(outros, ", ")
}

// Resumo conta as situações de um exercício.
func Resumo(w io.Writer, entregas map[string]turma.Entrega) {
	contagem := map[turma.SituacaoEntrega]int{}
	for _, e := range entregas {
		contagem[e.Situacao]++
	}
	var sits []turma.SituacaoEntrega
	for s := range contagem {
		sits = append(sits, s)
	}
	sort.Slice(sits, func(i, j int) bool { return contagem[sits[i]] > contagem[sits[j]] })

	var partes []string
	for _, s := range sits {
		partes = append(partes, fmt.Sprintf("%d %s", contagem[s], s.Rotulo()))
	}
	if len(partes) > 0 {
		fmt.Fprintln(w, strings.Join(partes, ", "))
	}
}

// ResumoVerificacoes conta os vereditos da suíte automatizada.
func ResumoVerificacoes(w io.Writer, vs map[string]turma.Verificacao) {
	contagem := map[turma.SituacaoVerificacao]int{}
	for _, v := range vs {
		contagem[v.Situacao]++
	}
	var partes []string
	for _, s := range []turma.SituacaoVerificacao{
		turma.Aprovado, turma.Reprovado, turma.SemClone, turma.ErroVerificacao,
	} {
		if contagem[s] > 0 {
			partes = append(partes, fmt.Sprintf("%d %s", contagem[s], s))
		}
	}
	if len(partes) > 0 {
		fmt.Fprintln(w, "verificação: "+strings.Join(partes, ", "))
	}
}

// Status é o panorama da turma: quem está travado antes da entrega e como
// anda cada exercício.
func Status(w io.Writer, t *turma.Turma) {
	Cabecalho(w, t)

	var pendentes []turma.Aluno
	for _, a := range t.Ativos() {
		switch a.SituacaoConta {
		case turma.ContaOK:
		case turma.ContaDesconhecida:
			pendentes = append(pendentes, a)
		default:
			pendentes = append(pendentes, a)
		}
	}
	if len(pendentes) > 0 {
		fmt.Fprintf(w, "Cadastro no GitLab a resolver (%d):\n", len(pendentes))
		tw := nova(w)
		for _, a := range pendentes {
			conta := string(a.SituacaoConta)
			if conta == "" {
				conta = "não verificada"
			}
			detalhe := a.Grupo
			if detalhe == "" {
				detalhe = "esperado " + t.Config.CaminhoGrupo(a.GRR)
			}
			fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\n", a.GRR, a.Nome, conta, detalhe)
		}
		tw.Flush()
		fmt.Fprintln(w)
	}

	hoje := turma.Hoje()
	for _, e := range t.ExerciciosAtivos() {
		entregas := t.EntregasDoExercicio(e.ID)
		prazo := e.Prazo.String()
		if e.Prazo.Depois(hoje) {
			prazo += " (em aberto)"
		}
		fmt.Fprintf(w, "%s  %s  prazo %s\n  ", e.ID, e.Titulo, prazo)
		if len(entregas) == 0 {
			fmt.Fprintln(w, "sem coleta")
		} else {
			Resumo(w, entregas)
		}
		if vs := t.VerificacoesDoExercicio(e.ID); len(vs) > 0 {
			fmt.Fprint(w, "  ")
			ResumoVerificacoes(w, vs)
		}
	}
}
