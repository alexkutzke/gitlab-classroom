// Package export gera os artefatos que saem da ferramenta, hoje a tabela de
// entregas em markdown.
package export

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/alexkutzke/gitlab-classroom/internal/turma"
)

// Identificacao escolhe como o aluno aparece na tabela.
type Identificacao string

const (
	PorNome Identificacao = "nome"
	PorGRR  Identificacao = "grr"
)

// Opcoes controla a geração da tabela.
type Opcoes struct {
	Identificacao Identificacao
	// Exercicios restringe as colunas; vazio usa todos os ativos.
	Exercicios []turma.Exercicio
	// Momento é o carimbo de atualização, injetável para o teste não
	// depender do relógio.
	Momento time.Time
}

// celulas traduz a situação da entrega para o texto da tabela, usando
// Detalhe para distinguir casos que a situação sozinha esconde: fork vazio
// de fork com commit no ramo errado, e entrega achada no fork do colega.
func celula(e turma.Entrega, existe bool) string {
	if !existe || e.Situacao == "" {
		return "-"
	}
	dupla := strings.HasPrefix(e.Detalhe, "entrega compartilhada")
	sufixo := ""
	if dupla {
		sufixo = " (dupla)"
	}
	switch e.Situacao {
	case turma.Entregue:
		if e.TemAtraso() {
			return fmt.Sprintf("ok (mexeu +%dd)%s", e.AtrasoDias, sufixo)
		}
		return "ok" + sufixo
	case turma.SemCommitNoPrazo:
		return fmt.Sprintf("fora do prazo (+%dd)%s", e.AtrasoDias, sufixo)
	case turma.ForkSemCommit:
		switch {
		case dupla:
			return "sem commit (dupla)"
		case e.Detalhe == "repositório vazio":
			return "sem commit (fork vazio)"
		case strings.Contains(e.Detalhe, "fora do ramo"):
			return "sem commit (ramo errado)"
		default:
			return "sem commit"
		}
	case turma.SemFork:
		return "sem fork"
	case turma.SemConta:
		return "sem conta"
	case turma.SemAcesso:
		return "sem acesso"
	case turma.GrupoInvisivel:
		return "sem grupo"
	case turma.Erro:
		return "erro"
	}
	return string(e.Situacao)
}

// Markdown escreve a tabela de entregas, uma linha por aluno e uma coluna por
// exercício. É o formato que os scripts antigos publicavam no material da
// disciplina.
func Markdown(w io.Writer, t *turma.Turma, o Opcoes) error {
	exs := o.Exercicios
	if len(exs) == 0 {
		exs = t.ExerciciosAtivos()
	}
	if o.Momento.IsZero() {
		o.Momento = time.Now()
	}
	if o.Identificacao == "" {
		o.Identificacao = PorNome
	}

	// O título segue a categoria das colunas: publicada em separado, a tabela
	// do trabalho não pode se anunciar como a dos exercícios.
	assunto := "dos exercícios"
	if cats := turma.CategoriasAtivas(exs); len(cats) == 1 && cats[0] != turma.CategoriaExercicio {
		assunto = "do " + cats[0]
	}
	fmt.Fprintf(w, "# Entrega %s, %s\n\n", assunto, t.Config.Descricao())
	fmt.Fprintf(w, "- **Turma**: %s\n", t.Config.Turma)
	fmt.Fprintf(w, "- **Semestre**: %s\n", t.Config.Semestre)
	fmt.Fprintf(w, "- **Última atualização**: %s\n\n", o.Momento.Format("02/01/2006 15:04"))

	if len(exs) == 0 {
		fmt.Fprintln(w, "Nenhum exercício cadastrado.")
		return nil
	}

	primeira := "Aluno"
	if o.Identificacao == PorGRR {
		primeira = "GRR"
	}
	cabecalho := []string{primeira}
	separador := []string{"---"}
	for _, e := range exs {
		titulo := e.ID
		if e.Titulo != "" {
			titulo = e.Titulo
		}
		cabecalho = append(cabecalho, fmt.Sprintf("%s<br>%s", titulo, e.Prazo.Curta()))
		separador = append(separador, ":---:")
	}
	fmt.Fprintf(w, "| %s |\n", strings.Join(cabecalho, " | "))
	fmt.Fprintf(w, "| %s |\n", strings.Join(separador, " | "))

	entregas := map[string]map[string]turma.Entrega{}
	for _, e := range exs {
		entregas[e.ID] = t.EntregasDoExercicio(e.ID)
	}

	for _, a := range alunosDaTabela(t, o.Identificacao) {
		linha := []string{identificar(a, o.Identificacao)}
		for _, e := range exs {
			en, ok := entregas[e.ID][a.GRR]
			linha = append(linha, celula(en, ok))
		}
		fmt.Fprintf(w, "| %s |\n", strings.Join(linha, " | "))
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "Legenda: `ok` entregue no prazo; `fora do prazo` só há commits depois da data;")
	fmt.Fprintln(w, "`sem commit` fork criado sem trabalho do aluno (`fork vazio` nunca teve um")
	fmt.Fprintln(w, "commit, `ramo errado` tem commits fora do ramo padrão); `sem fork` a tarefa não")
	fmt.Fprintln(w, "foi bifurcada; `sem acesso` o grupo existe, mas o professor não foi adicionado")
	fmt.Fprintln(w, "como reporter; `sem grupo` o grupo não foi criado, ou foi criado privado sem")
	fmt.Fprintln(w, "compartilhar. O sufixo `(dupla)` marca entrega achada no fork do colega.")

	if o.Identificacao == PorGRR {
		fmt.Fprintln(w)
		fmt.Fprintf(w, "Se a sua linha mostra `sem grupo` ou `sem acesso`, confira em **Settings >\n"+
			"General** se o grupo se chama `%s`, se está compartilhado (não privado sem\n"+
			"convite) e se `alexkutzke` consta em **Members** como `reporter`.\n",
			t.Config.CaminhoGrupo("grrXXXXXXXX"))
	}
	return nil
}

// alunosDaTabela ordena as linhas conforme a identificação escolhida.
//
// A tabela publicada é por GRR e sai ordenada por GRR: manter a ordem
// alfabética de nome deixaria a posição na lista revelando quem é quem.
func alunosDaTabela(t *turma.Turma, i Identificacao) []turma.Aluno {
	alunos := t.Ativos()
	if i != PorGRR {
		return alunos
	}
	ordenados := append([]turma.Aluno(nil), alunos...)
	sort.SliceStable(ordenados, func(a, b int) bool { return ordenados[a].GRR < ordenados[b].GRR })
	return ordenados
}

func identificar(a turma.Aluno, i Identificacao) string {
	if i == PorGRR {
		return a.GRR
	}
	return a.Nome
}
