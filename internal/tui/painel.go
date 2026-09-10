package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/alexkutzke/gitlab-classroom/internal/acoes"
	"github.com/alexkutzke/gitlab-classroom/internal/turma"
)

// painel é a tela inicial: o que falta fazer e como anda cada exercício.
type painel struct {
	cursor int
	topo   int
}

func (p *painel) atualizar(a *App, msg tea.KeyMsg) (tea.Cmd, bool) {
	total := len(a.panorama.Exercicios)
	switch msg.String() {
	case "up", "k":
		p.cursor = max(0, p.cursor-1)
	case "down", "j":
		p.cursor = min(total-1, p.cursor+1)
	case "home", "g":
		p.cursor = 0
	case "end", "G":
		p.cursor = max(0, total-1)
	case "enter":
		if total == 0 {
			a.avisar("nenhum exercício cadastrado; use `classroom exercicios add`")
			return nil, true
		}
		a.exercicio = a.panorama.Exercicios[p.cursor].Exercicio.ID
		a.entregas.reiniciar()
		a.ir(idExercicio)
	case "c":
		if total == 0 {
			a.erro = "nenhum exercício cadastrado"
			return nil, true
		}
		return a.coletar([]turma.Exercicio{a.panorama.Exercicios[p.cursor].Exercicio}), true
	case "n":
		if total == 0 {
			return nil, true
		}
		e := a.panorama.Exercicios[p.cursor].Exercicio
		a.exercicio = e.ID
		a.entregas.reiniciar()
		return a.abrirCorrecao(e, acoes.FiltroCorrecao{}), true
	case "E":
		return a.exportar(), true
	default:
		return nil, false
	}
	return nil, true
}

func (p *painel) desenhar(a *App) string {
	var b strings.Builder

	b.WriteString(estTitulo.Render("Pendências") + "\n")
	pendencias := a.panorama.Pendencias()
	if len(pendencias) == 0 {
		b.WriteString(estOK.Render("  nada pendente") + "\n")
	}
	for _, texto := range pendencias {
		b.WriteString("  " + estAtencao.Render(texto) + "\n")
	}

	b.WriteString("\n" + estTitulo.Render("Exercícios") + "\n")
	if len(a.panorama.Exercicios) == 0 {
		b.WriteString(estFraco.Render("  nenhum exercício cadastrado") + "\n")
		return b.String()
	}

	linhas := a.linhasDisponiveis() - len(pendencias) - 4
	if linhas < 3 {
		linhas = 3
	}
	p.topo = janela(p.topo, p.cursor, linhas, len(a.panorama.Exercicios))
	fim := min(p.topo+linhas, len(a.panorama.Exercicios))

	for i := p.topo; i < fim; i++ {
		b.WriteString(p.linha(a, i) + "\n")
	}
	if len(a.panorama.Exercicios) > linhas {
		b.WriteString(rolagem(p.topo, fim, len(a.panorama.Exercicios)) + "\n")
	}
	return b.String()
}

func (p *painel) linha(a *App, i int) string {
	r := a.panorama.Exercicios[i]
	e := r.Exercicio

	cursor := "  "
	if i == p.cursor {
		cursor = estCursor.Render("> ")
	}

	prazo := e.Prazo.Curta()
	if !r.Vencido {
		prazo = estFraco.Render(prazo + " aberto")
	}

	var estado string
	switch {
	case !r.Coletado:
		estado = estFraco.Render("sem coleta")
	default:
		entregues := r.Situacoes[turma.Entregue]
		estado = estOK.Render(fmt.Sprintf("%d %s", entregues, concordar(entregues, "entregue", "entregues")))
		if r.Pendentes > 0 {
			estado += estAtencao.Render(fmt.Sprintf("  %d %s", r.Pendentes,
				concordar(r.Pendentes, "pendente", "pendentes")))
		}
	}

	notas := estFraco.Render("sem notas")
	if r.Notas > 0 {
		notas = fmt.Sprintf("%d %s", r.Notas, concordar(r.Notas, "nota", "notas"))
		if r.SemNota > 0 {
			notas += estAtencao.Render(fmt.Sprintf(" (%d sem)", r.SemNota))
		}
	}

	extra := ""
	if a.panorama.VariasCategorias() {
		extra += estFraco.Render("  " + e.CategoriaDe())
	}
	if r.Compartilhadas > 0 {
		extra += estFraco.Render(fmt.Sprintf("  %d em dupla", r.Compartilhadas))
	}

	if n := r.Verificacoes[turma.Aprovado]; n > 0 {
		extra += estOK.Render(fmt.Sprintf("  %d aprovados", n))
	}
	if n := r.Verificacoes[turma.Reprovado]; n > 0 {
		extra += estErro.Render(fmt.Sprintf("  %d reprovados", n))
	}

	nome := preencher(e.ID, 12)
	if i == p.cursor {
		nome = estCursor.Render(nome)
	}
	return cursor + nome + preencher(prazo, 14) + preencher(estado, 30) +
		preencher(notas, 20) + extra
}

// concordar escolhe entre singular e plural.
func concordar(n int, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}
