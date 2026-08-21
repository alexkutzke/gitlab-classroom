package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/alexkutzke/gitlab-classroom/internal/turma"
)

var (
	estTitulo   = lipgloss.NewStyle().Bold(true)
	estFraco    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	estCursor   = lipgloss.NewStyle().Bold(true)
	estOK       = lipgloss.NewStyle().Foreground(lipgloss.Color("35"))
	estAtencao  = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	estErro     = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	estAviso    = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	estDestaque = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
)

// corDaSituacao escolhe a cor pela gravidade da situação da entrega.
func corDaSituacao(s turma.SituacaoEntrega) lipgloss.Style {
	switch s {
	case turma.Entregue:
		return estOK
	case turma.SemCommitNoPrazo, turma.ForkSemCommit:
		return estAtencao
	case "":
		return estFraco
	}
	return estErro
}

// corDaVerificacao escolhe a cor pelo veredito da suíte.
func corDaVerificacao(s turma.SituacaoVerificacao) lipgloss.Style {
	switch s {
	case turma.Aprovado:
		return estOK
	case turma.Reprovado:
		return estErro
	}
	return estFraco
}

// janela devolve o novo topo da lista para manter o cursor visível.
func janela(topo, cursor, linhas, total int) int {
	if linhas <= 0 || total == 0 {
		return 0
	}
	if cursor < topo {
		topo = cursor
	}
	if cursor >= topo+linhas {
		topo = cursor - linhas + 1
	}
	if topo > total-linhas {
		topo = total - linhas
	}
	if topo < 0 {
		topo = 0
	}
	return topo
}

// truncar corta o texto pela largura de tela, e não por bytes.
func truncar(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 3 {
		return string(r[:n])
	}
	return string(r[:n-3]) + "..."
}

// preencher completa o texto até a largura pedida, medindo em colunas de
// tela: um %-Ns contaria os bytes das sequências de cor.
func preencher(s string, n int) string {
	falta := n - lipgloss.Width(s)
	if falta <= 0 {
		return s
	}
	return s + strings.Repeat(" ", falta)
}

// alinhar coloca o segundo texto à direita da largura informada.
func alinhar(esquerda, direita string, largura int) string {
	espaco := largura - lipgloss.Width(esquerda) - lipgloss.Width(direita)
	if espaco < 1 {
		return esquerda
	}
	return esquerda + strings.Repeat(" ", espaco) + direita
}

// rolagem descreve a posição da janela numa lista maior que a tela.
func rolagem(topo, fim, total int) string {
	if total == 0 {
		return ""
	}
	return estFraco.Render(fmt.Sprintf("  %d-%d de %d", topo+1, fim, total))
}
