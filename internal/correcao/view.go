package correcao

import (
	"fmt"
	"strings"

	"github.com/alexkutzke/gitlab-classroom/internal/turma"
)

func (m *modelo) View() string {
	var b strings.Builder

	b.WriteString(m.cabecalho())
	b.WriteString("\n\n")
	b.WriteString(m.lista())
	b.WriteString("\n")
	b.WriteString(m.rodape())
	return b.String()
}

func (m *modelo) cabecalho() string {
	e := m.exercicio
	titulo := e.ID
	if e.Titulo != "" {
		titulo = e.Titulo
	}
	linha := estTitulo.Render(fmt.Sprintf("Correção de %s", titulo))
	linha += estFraco.Render(fmt.Sprintf("   prazo %s   nota de 0 a %s",
		e.Prazo.String(), formatarNota(m.notaMaxima)))

	corrigidos := 0
	for _, i := range m.itens {
		if i.temNota {
			corrigidos++
		}
	}
	linha += "\n" + estFraco.Render(fmt.Sprintf("%d de %d com nota lançada", corrigidos, len(m.itens)))
	return linha
}

func (m *modelo) lista() string {
	if len(m.visivel) == 0 {
		return estFraco.Render("  nenhum aluno com esse filtro")
	}

	var b strings.Builder
	fim := min(m.topo+m.altura, len(m.visivel))
	for pos := m.topo; pos < fim; pos++ {
		it := m.itens[m.visivel[pos]]
		cursor := "  "
		nome := it.Aluno.Nome
		if pos == m.cursor {
			cursor = estCursor.Render("> ")
			nome = estCursor.Render(nome)
		}
		marca := " "
		if len(it.Equipe) > 0 {
			marca = estFraco.Render("d")
		}
		b.WriteString(fmt.Sprintf("%s%s %-32s %-30s %-22s %s\n",
			cursor, marca, truncar(nome, 32), m.situacao(it), m.verificacao(it), m.nota(it)))
	}
	if fim < len(m.visivel) {
		b.WriteString(estFraco.Render(fmt.Sprintf("  ... mais %d\n", len(m.visivel)-fim)))
	}
	return b.String()
}

// situacao resume o que a coleta apurou, que é o contexto para dar a nota.
func (m *modelo) situacao(it Item) string {
	e := it.Entrega
	switch e.Situacao {
	case "":
		return estFraco.Render("sem coleta")
	case turma.Entregue:
		texto := fmt.Sprintf("entregue, %d commit(s)", e.Commits)
		if e.TemAtraso() {
			return estEntregue.Render(texto) + estAtraso.Render(fmt.Sprintf(" +%dd depois", e.AtrasoDias))
		}
		return estEntregue.Render(texto)
	case turma.SemCommitNoPrazo:
		return estAtraso.Render(fmt.Sprintf("fora do prazo, +%dd", e.AtrasoDias))
	default:
		return estFalta.Render(e.Situacao.Rotulo())
	}
}

// verificacao mostra o resultado da suíte, marcando com ! o que foi apurado
// sobre um commit anterior ao da entrega atual.
func (m *modelo) verificacao(it Item) string {
	v := it.Verificacao
	if v.Situacao == "" || v.Situacao == turma.SemSuite {
		return ""
	}
	texto := v.Resumo()
	if v.Desatualizada(it.Entrega.Commit) {
		texto += " !"
	}
	switch v.Situacao {
	case turma.Aprovado:
		return estEntregue.Render(texto)
	case turma.Reprovado:
		return estFalta.Render(texto)
	default:
		return estFraco.Render(texto)
	}
}

func (m *modelo) nota(it Item) string {
	if !it.temNota {
		return estFraco.Render("  -")
	}
	marca := ""
	if it.alterado {
		marca = "*"
	}
	return estNota.Render(fmt.Sprintf("%5s%s", formatarNota(it.nota), marca))
}

func (m *modelo) rodape() string {
	var b strings.Builder

	if it := m.atual(); it != nil {
		if len(it.Equipe) > 0 {
			b.WriteString(estFraco.Render("entrega em dupla com ") +
				strings.Join(it.Equipe, ", ") + "\n")
		}
		if it.comentario != "" {
			b.WriteString(estFraco.Render("comentário: ") + truncar(it.comentario, m.largura-14) + "\n")
		}
	}

	switch m.modo {
	case digitandoNota:
		b.WriteString(fmt.Sprintf("nota (0 a %s): %s\n", formatarNota(m.notaMaxima), estNota.Render(m.buffer+"_")))
	case digitandoComentario:
		b.WriteString("comentário: " + m.buffer + "_\n")
	case digitandoFiltro:
		b.WriteString(estFiltro.Render("filtro: ") + m.buffer + "_\n")
	default:
		if m.filtro != "" {
			b.WriteString(estFiltro.Render("filtro: "+m.filtro) + estFraco.Render("  (esc limpa)") + "\n")
		}
	}

	if m.aviso != "" {
		b.WriteString(estAviso.Render(m.aviso) + "\n")
	}

	atalhos := "j k move   0-9/n nota   c comentário   r repete a última   x apaga   o abre no editor   / filtra   enter grava   q sai"
	if temEquipe(m.itens) {
		estado := "ligada"
		if !m.propagar {
			estado = "desligada"
		}
		atalhos += "\nD nota da dupla nos dois integrantes: " + estado
	}
	b.WriteString(estFraco.Render(atalhos))
	return b.String()
}

func temEquipe(itens []Item) bool {
	for _, i := range itens {
		if len(i.Equipe) > 0 {
			return true
		}
	}
	return false
}

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
