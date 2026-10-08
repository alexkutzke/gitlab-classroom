package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/alexkutzke/gitlab-classroom/internal/acoes"
	"github.com/alexkutzke/gitlab-classroom/internal/moldura"
	"github.com/alexkutzke/gitlab-classroom/internal/turma"
)

// detalhe monta as linhas de um painel de detalhe: campos com rótulo em
// cinza, seções em azul e texto longo quebrado na largura do painel.
type detalhe struct {
	largura int
	linhas  []string
}

func novoDetalhe(largura int) *detalhe { return &detalhe{largura: largura} }

const larguraRotulo = 16

func (d *detalhe) campo(rotulo, valor string) {
	falta := max(0, larguraRotulo-ansi.StringWidth(rotulo))
	d.linhas = append(d.linhas, " "+estFraco.Render(rotulo+strings.Repeat(" ", falta))+" "+valor)
}

func (d *detalhe) linha(s string) { d.linhas = append(d.linhas, s) }

func (d *detalhe) secao(titulo string) {
	if len(d.linhas) > 0 {
		d.linhas = append(d.linhas, "")
	}
	d.linhas = append(d.linhas, estSecao.Render(" "+titulo))
}

// contagem é a linha "símbolo rótulo número" das seções de totais.
func (d *detalhe) contagem(simbolo, rotulo string, n int, est lipgloss.Style) {
	falta := max(0, 28-ansi.StringWidth(rotulo))
	d.linhas = append(d.linhas, " "+est.Render(simbolo)+" "+rotulo+strings.Repeat(" ", falta)+
		est.Render(fmt.Sprintf("%3d", n)))
}

// texto quebra o parágrafo na largura do painel, com recuo.
func (d *detalhe) texto(s string, est lipgloss.Style) {
	for _, l := range moldura.Quebrar(s, max(10, d.largura-6)) {
		d.linhas = append(d.linhas, "  "+est.Render(l))
	}
}

// aviso é a linha de alerta com símbolo, quebrada na largura do painel.
func (d *detalhe) aviso(simbolo, s string, est lipgloss.Style) {
	for i, l := range moldura.Quebrar(s, max(10, d.largura-7)) {
		prefixo := "   "
		if i == 0 {
			prefixo = " " + simbolo + " "
		}
		d.linhas = append(d.linhas, est.Render(prefixo+l))
	}
}

func (d *detalhe) conteudo() moldura.Conteudo {
	return moldura.Conteudo{Linhas: d.linhas, Sel: -1}
}

// instante escreve data e hora no fuso local, no formato curto das telas.
func instante(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Local().Format("02/01 15:04")
}

// commitCurto é o SHA abreviado, como o git mostra.
func commitCurto(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}

// detalheDaEntrega escreve o que se sabe da entrega de um aluno: fork,
// commits, equipe, suíte, correção e devolutiva. É o painel [2] da tela de
// entregas.
func detalheDaEntrega(a *App, e turma.Exercicio, l linhaEntrega, largura int) *detalhe {
	d := novoDetalhe(largura)
	en := l.Entrega
	d.linha(estFraco.Render(" " + l.Aluno.GRR))

	sit := estFraco.Render("· sem coleta")
	if en.Situacao != "" {
		sit = corDaSituacao(en.Situacao).Render(simboloDaSituacao(en.Situacao) + " " + en.Descricao())
	}
	d.campo("situação", sit)
	if en.Projeto != "" {
		d.campo("fork", en.Projeto)
	}
	if en.Commit != "" {
		d.campo("commit avaliado", commitCurto(en.Commit)+", "+instante(en.DataCommit))
	}
	if en.UltimoCommit != "" && en.UltimoCommit != en.Commit {
		d.campo("último commit", commitCurto(en.UltimoCommit)+", "+instante(en.DataUltimo))
	}
	if en.TemAtraso() {
		d.campo("atraso", estAtencao.Render(fmt.Sprintf("%d %s", en.AtrasoDias,
			concordar(en.AtrasoDias, "dia", "dias"))))
	}
	if en.Commits > 0 {
		d.campo("commits do aluno", fmt.Sprint(en.Commits))
	}
	if len(l.Equipe) > 0 {
		equipe := strings.Join(l.Equipe, ", ")
		if dono := a.turma.Dono(e.ID, l.Aluno.GRR); dono != l.Aluno.GRR {
			nome := dono
			if al, ok := a.turma.AlunoPorGRR(dono); ok {
				nome = al.Nome
			}
			d.campo("entrega", "no fork de "+nome)
		}
		d.campo("equipe", equipe)
	}
	if !en.ColetadoEm.IsZero() {
		d.campo("coletado em", instante(en.ColetadoEm))
	}
	if en.Detalhe != "" && en.Situacao != turma.Erro {
		d.texto(en.Detalhe, estFraco)
	}

	if v := l.Verificacao; v.Situacao != "" && v.Situacao != turma.SemSuite {
		d.secao("VERIFICAÇÃO")
		texto, est := textoDaVerificacao(v, en.Commit)
		d.campo("resultado", est.Render(texto)+" "+estFraco.Render(string(v.Situacao)))
		if v.Commit != "" {
			d.campo("commit", commitCurto(v.Commit)+", "+instante(v.ExecutadoEm))
		}
		if v.Desatualizada(en.Commit) {
			d.aviso("▲", "feita sobre commit anterior ao da entrega; v verifica de novo", estAtencao)
		}
		if v.Detalhe != "" {
			d.texto(v.Detalhe, estFraco)
		}
	}

	d.secao("CORREÇÃO")
	if l.Nota == nil {
		d.campo("nota", estFraco.Render("sem nota"))
	} else {
		d.campo("nota", estAcento.Bold(true).Render(fmt.Sprintf("%g", l.Nota.Valor)))
		if !l.Nota.CorrigidoEm.IsZero() {
			d.campo("corrigida em", instante(l.Nota.CorrigidoEm))
		}
		if strings.TrimSpace(l.Nota.Comentario) == "" {
			d.campo("comentário", estFraco.Render("nenhum"))
		} else {
			d.linha(" " + estFraco.Render("comentário"))
			d.texto(l.Nota.Comentario, estNormal)
		}
	}

	d.secao("DEVOLUTIVA")
	comentario := ""
	if l.Nota != nil {
		comentario = strings.TrimSpace(l.Nota.Comentario)
	}
	dev, publicada := acoes.DevolutivaDaEntrega(a.turma, e.ID, l.Aluno.GRR)
	switch {
	case publicada && dev.Desatualizada(comentario):
		d.aviso("▲", fmt.Sprintf("desatualizada: o comentário mudou depois da issue #%d, de %s",
			dev.Issue, instante(dev.PublicadoEm)), estAtencao)
	case publicada:
		d.aviso("✓", fmt.Sprintf("publicada em %s, issue #%d", instante(dev.PublicadoEm), dev.Issue), estOK)
	case comentario == "":
		d.aviso("·", "nada a publicar sem comentário", estFraco)
	default:
		d.aviso("●", "não publicada", estAtencao)
	}
	if publicada && dev.URL != "" {
		d.linha("   " + estFraco.Render(dev.URL))
	}
	return d
}

// concordar escolhe entre singular e plural.
func concordar(n int, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}
