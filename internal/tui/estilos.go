package tui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"

	"github.com/alexkutzke/gitlab-classroom/internal/moldura"
	"github.com/alexkutzke/gitlab-classroom/internal/turma"
)

// Os estilos vêm da moldura, que é a mesma do painel e da correção. Os nomes
// curtos ficam aqui só para o desenho das telas não repetir o pacote a cada
// célula.
var (
	estNormal  = moldura.EstNormal
	estFraco   = moldura.EstFraco
	estOK      = moldura.EstOK
	estAtencao = moldura.EstAtencao
	estErro    = moldura.EstErro
	estAcento  = moldura.EstAcento
	estSecao   = moldura.EstSecao
	estTecla   = moldura.EstTecla
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

// simboloDaSituacao acompanha a cor: em dia, atenção ou falta.
func simboloDaSituacao(s turma.SituacaoEntrega) string {
	switch s {
	case turma.Entregue:
		return "✓"
	case turma.SemCommitNoPrazo, turma.ForkSemCommit:
		return "▲"
	case "":
		return "·"
	}
	return "✖"
}

// corDaVerificacao escolhe a cor pelo veredito da suíte.
func corDaVerificacao(s turma.SituacaoVerificacao) lipgloss.Style {
	switch s {
	case turma.Aprovado:
		return estOK
	case turma.Reprovado, turma.ErroVerificacao:
		return estErro
	}
	return estFraco
}

// textoDaVerificacao resume a suíte numa célula: símbolo do veredito e a
// contagem. Resultado apurado sobre commit anterior ao da entrega leva ▲, a
// mesma condição que a tela de correção marca.
func textoDaVerificacao(v turma.Verificacao, commitDaEntrega string) (string, lipgloss.Style) {
	if v.Situacao == "" || v.Situacao == turma.SemSuite {
		return "-", estFraco
	}
	cont := ""
	if v.Total > 0 {
		cont = fmt.Sprintf(" %d/%d", v.Aprovados, v.Total)
	}
	if v.Desatualizada(commitDaEntrega) {
		return "▲" + cont, estAtencao
	}
	switch v.Situacao {
	case turma.Aprovado:
		return "✓" + cont, estOK
	case turma.Reprovado:
		return "✖" + cont, estErro
	case turma.ErroVerificacao:
		return "✖ erro", estErro
	}
	return string(v.Situacao), estFraco
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
