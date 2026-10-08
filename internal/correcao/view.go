package correcao

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/alexkutzke/gitlab-classroom/internal/moldura"
	"github.com/alexkutzke/gitlab-classroom/internal/turma"
)

// A correção desenha com a mesma moldura da interface: [1] a lista de alunos,
// [2] a entrega do aluno selecionado, com o comentário inteiro. A nota e o
// comentário são editados dentro de [2], no lugar do valor.

var (
	estNormal  = moldura.EstNormal
	estFraco   = moldura.EstFraco
	estOK      = moldura.EstOK
	estAtencao = moldura.EstAtencao
	estErro    = moldura.EstErro
	estAcento  = moldura.EstAcento
	estTecla   = moldura.EstTecla
)

func (m *modelo) View() string {
	w, h := m.largura, m.altura
	if w < moldura.LarguraMinima || h < moldura.AlturaMinima {
		return moldura.Pequena(w, h)
	}
	if m.ajuda {
		return moldura.Ajuda(w, h, m.gruposAjuda(), "qualquer tecla fecha")
	}
	return lipgloss.JoinVertical(lipgloss.Left, m.barraTitulo(w), m.corpo(w, h-2), m.barraTeclas(w))
}

// barraTitulo leva o que era o cabeçalho da tela: exercício, prazo, escala e
// quantos já têm nota.
func (m *modelo) barraTitulo(w int) string {
	e := m.exercicio
	titulo := e.ID
	if e.Titulo != "" {
		titulo = e.Titulo
	}
	corrigidos := 0
	for _, i := range m.itens {
		if i.temNota {
			corrigidos++
		}
	}
	esq := []moldura.Trecho{
		{Texto: " Correção de " + titulo, Est: estAcento.Bold(true)},
		{Texto: fmt.Sprintf(" · prazo %s · nota de 0 a %s · %d de %d com nota lançada",
			e.Prazo.Curta(), formatarNota(m.notaMaxima), corrigidos, len(m.itens)), Est: estNormal},
	}
	var dir []moldura.Trecho
	if m.anuncio != "" {
		dir = append(dir, moldura.Trecho{Texto: m.anuncio + " ", Est: estAtencao})
	}
	return moldura.BarraTitulo(w, "classroom", esq, dir)
}

// retanguloDetalhe é o tamanho do painel [2], que a rolagem precisa saber.
func (m *modelo) retanguloDetalhe() (int, int) {
	w, h := m.largura, m.altura-2
	if w < moldura.LarguraDuasColunas {
		return w, h
	}
	return w - moldura.LarguraLista(moldura.LarguraLinhas(m.linhasLista()), w), h
}

// linhasVisiveis é a página da lista, para pgup e pgdown.
func (m *modelo) linhasVisiveis() int { return max(1, m.altura-5) }

func (m *modelo) corpo(w, h int) string {
	lista := m.caixaLista
	if w < moldura.LarguraDuasColunas {
		if m.foco == 1 {
			return m.caixaDetalhe(w, h)
		}
		return lista(w, h)
	}
	esq := moldura.LarguraLista(moldura.LarguraLinhas(m.linhasLista()), w)
	return lipgloss.JoinHorizontal(lipgloss.Top, lista(esq, h), m.caixaDetalhe(w-esq, h))
}

func (m *modelo) linhasLista() []string {
	cab := []moldura.Celula{moldura.Cel(" ", estFraco), moldura.Cel("aluno", estFraco),
		moldura.Cel("situação", estFraco), moldura.Cel("verif", estFraco), moldura.Num("nota", estFraco)}
	var linhas [][]moldura.Celula
	for _, i := range m.visivel {
		it := m.itens[i]
		marca := moldura.Cel(" ", estFraco)
		if len(it.Equipe) > 0 {
			marca = moldura.Cel("d", estFraco)
		}
		verif, estVerif := textoDaVerificacao(it.Verificacao, it.Entrega.Commit)
		linhas = append(linhas, []moldura.Celula{marca, moldura.Cel(truncar(it.Aluno.Nome, 32), estNormal),
			m.celulaSituacao(it), moldura.Cel(verif, estVerif), m.celulaNota(it)})
	}
	return moldura.Tabela(cab, linhas)
}

func (m *modelo) caixaLista(w, h int) string {
	titulo := "Alunos"
	if m.filtro != "" {
		titulo += fmt.Sprintf(" · filtro %q", m.filtro)
	}
	c := moldura.Conteudo{Linhas: m.linhasLista(), Sel: -1}
	if len(m.visivel) == 0 {
		c.Linhas = []string{estFraco.Render(" nenhum aluno com esse filtro")}
	} else {
		c.Sel = 1 + m.cursor
		c.Info = fmt.Sprintf("%d de %d", m.cursor+1, len(m.visivel))
	}
	return moldura.Caixa("1", titulo, c, w, h, m.foco == 0)
}

// celulaSituacao resume o que a coleta apurou, que é o contexto para dar a
// nota.
func (m *modelo) celulaSituacao(it Item) moldura.Celula {
	e := it.Entrega
	switch e.Situacao {
	case "":
		return moldura.Cel("sem coleta", estFraco)
	case turma.Entregue:
		if e.TemAtraso() {
			return moldura.Cel(fmt.Sprintf("entregue (+%dd)", e.AtrasoDias), estOK)
		}
		return moldura.Cel("entregue", estOK)
	case turma.SemCommitNoPrazo:
		return moldura.Cel(fmt.Sprintf("fora do prazo (+%dd)", e.AtrasoDias), estAtencao)
	case turma.ForkSemCommit:
		return moldura.Cel(e.Situacao.Rotulo(), estAtencao)
	}
	return moldura.Cel(e.Situacao.Rotulo(), estErro)
}

// celulaNota marca com * o que mudou nesta sessão e ainda não foi gravado.
func (m *modelo) celulaNota(it Item) moldura.Celula {
	marca := ""
	if it.alterado {
		marca = "*"
	}
	if !it.temNota {
		return moldura.Num("-"+marca, estFraco)
	}
	return moldura.Num(formatarNota(it.nota)+marca, estAcento)
}

// textoDaVerificacao mostra o veredito da suíte, com ▲ no que foi apurado
// sobre um commit anterior ao da entrega atual.
func textoDaVerificacao(v turma.Verificacao, commit string) (string, lipgloss.Style) {
	if v.Situacao == "" || v.Situacao == turma.SemSuite {
		return "-", estFraco
	}
	cont := ""
	if v.Total > 0 {
		cont = fmt.Sprintf(" %d/%d", v.Aprovados, v.Total)
	}
	if v.Desatualizada(commit) {
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

func (m *modelo) caixaDetalhe(w, h int) string {
	titulo := "Detalhe"
	if it := m.atual(); it != nil {
		titulo = it.Aluno.Nome
	}
	linhas, edicao := m.detalheComEdicao(w)
	topo := m.topoDet
	// Durante a edição, o texto digitado fica à vista, mesmo que o
	// comentário seja longo e esteja no fim do painel.
	if visiveis := h - 2; edicao >= 0 && (edicao < topo || edicao >= topo+visiveis) {
		topo = max(0, edicao-visiveis+1)
	}
	c := moldura.Conteudo{Linhas: linhas, Sel: -1, Topo: topo, Livre: true}
	return moldura.Caixa("2", titulo, c, w, h, m.foco == 1)
}

func (m *modelo) linhasDetalhe(largura int) []string {
	linhas, _ := m.detalheComEdicao(largura)
	return linhas
}

// detalheComEdicao escreve a entrega do aluno selecionado. Segue o cursor,
// sem enter, e é onde a nota e o comentário são digitados. Devolve também a
// linha onde o texto em edição termina, ou -1 fora da edição.
func (m *modelo) detalheComEdicao(largura int) ([]string, int) {
	it := m.atual()
	if it == nil {
		return []string{estFraco.Render(" nada selecionado")}, -1
	}
	edicao := -1
	var r []string
	campo := func(rotulo, valor string) {
		falta := max(0, 16-ansi.StringWidth(rotulo))
		r = append(r, " "+estFraco.Render(rotulo+strings.Repeat(" ", falta))+" "+valor)
	}
	secao := func(t string) { r = append(r, "", moldura.EstSecao.Render(" "+t)) }
	texto := func(s string, est lipgloss.Style) {
		for _, l := range moldura.Quebrar(s, max(10, largura-6)) {
			r = append(r, "  "+est.Render(l))
		}
	}
	aviso := func(simbolo, s string, est lipgloss.Style) {
		for i, l := range moldura.Quebrar(s, max(10, largura-7)) {
			prefixo := "   "
			if i == 0 {
				prefixo = " " + simbolo + " "
			}
			r = append(r, est.Render(prefixo+l))
		}
	}

	en := it.Entrega
	r = append(r, estFraco.Render(" "+it.Aluno.GRR))
	sit := m.celulaSituacao(*it)
	campo("situação", sit.Est.Render(simboloDaSituacao(en.Situacao)+" "+sit.Texto))
	if en.Projeto != "" {
		campo("fork", en.Projeto)
	}
	if en.Commit != "" {
		campo("commit avaliado", commitCurto(en.Commit)+", "+instante(en.DataCommit))
	}
	if en.UltimoCommit != "" && en.UltimoCommit != en.Commit {
		campo("último commit", commitCurto(en.UltimoCommit)+", "+instante(en.DataUltimo))
	}
	if en.Commits > 0 {
		campo("commits do aluno", fmt.Sprint(en.Commits))
	}
	if len(it.Equipe) > 0 {
		campo("equipe", strings.Join(it.Equipe, ", "))
	}
	if en.Detalhe != "" {
		texto(en.Detalhe, estFraco)
	}

	if v := it.Verificacao; v.Situacao != "" && v.Situacao != turma.SemSuite {
		secao("VERIFICAÇÃO")
		t, est := textoDaVerificacao(v, en.Commit)
		campo("resultado", est.Render(t)+" "+estFraco.Render(string(v.Situacao)))
		if v.Desatualizada(en.Commit) {
			aviso("▲", "feita sobre commit anterior ao da entrega", estAtencao)
		}
		if v.Detalhe != "" {
			texto(v.Detalhe, estFraco)
		}
	}

	secao("CORREÇÃO")
	switch {
	case m.modo == digitandoNota:
		campo("nota", estAcento.Bold(true).Render(m.buffer+"_")+
			estFraco.Render(" de 0 a "+formatarNota(m.notaMaxima)))
		edicao = len(r) - 1
	case it.temNota:
		nota := estAcento.Bold(true).Render(formatarNota(it.nota))
		if it.alterado {
			nota += estFraco.Render("  lançada nesta sessão, grava com enter")
		}
		campo("nota", nota)
	case it.removido:
		campo("nota", estAtencao.Render("apagada nesta sessão"))
	default:
		campo("nota", estFraco.Render("sem nota"))
	}
	switch {
	case m.modo == digitandoComentario:
		r = append(r, " "+estFraco.Render("comentário"))
		texto(m.buffer+"_", estAcento)
		edicao = len(r) - 1
	case strings.TrimSpace(it.comentario) == "":
		campo("comentário", estFraco.Render("nenhum"))
	default:
		r = append(r, " "+estFraco.Render("comentário"))
		texto(it.comentario, estNormal)
	}

	secao("DEVOLUTIVA")
	d := it.Devolutiva
	comentario := strings.TrimSpace(it.comentario)
	switch {
	case d != nil && d.Desatualizada(comentario):
		aviso("▲", fmt.Sprintf("desatualizada: o comentário mudou depois da issue #%d, de %s",
			d.Issue, instante(d.PublicadoEm)), estAtencao)
	case d != nil:
		aviso("✓", fmt.Sprintf("publicada em %s, issue #%d", instante(d.PublicadoEm), d.Issue), estOK)
	case comentario == "":
		aviso("·", "nada a publicar sem comentário", estFraco)
	default:
		aviso("●", "não publicada", estAtencao)
	}
	return r, edicao
}

func (m *modelo) barraTeclas(w int) string {
	if m.aviso != "" {
		return moldura.BarraMensagem(w, m.aviso, estAtencao)
	}
	confirma := estTecla.Render("enter") + " " + estFraco.Render("confirma") + "   " +
		estTecla.Render("esc") + " " + estFraco.Render("cancela")
	switch m.modo {
	case digitandoNota:
		return moldura.Preencher(" "+estAcento.Render("nota, no painel [2]")+"   "+confirma, w)
	case digitandoComentario:
		return moldura.Preencher(" "+estAcento.Render("comentário, no painel [2]")+"   "+confirma, w)
	case digitandoFiltro:
		return moldura.Preencher(" "+estAcento.Render("filtro: ")+m.buffer+"_   "+
			estTecla.Render("enter")+" "+estFraco.Render("aplica")+"   "+
			estTecla.Render("esc")+" "+estFraco.Render("limpa"), w)
	}

	var t []moldura.Tecla
	if m.foco == 1 {
		t = []moldura.Tecla{{K: "tab", Rotulo: "painel"}, {K: "j/k", Rotulo: "rola"},
			{K: "esc", Rotulo: "lista"}}
	} else {
		t = []moldura.Tecla{{K: "tab", Rotulo: "painel"}, {K: "j/k", Rotulo: "move"}}
	}
	t = append(t, moldura.Tecla{K: "0-9/n", Rotulo: "nota"}, moldura.Tecla{K: "c", Rotulo: "comentário"},
		moldura.Tecla{K: "r", Rotulo: "repete"}, moldura.Tecla{K: "x", Rotulo: "apaga"},
		moldura.Tecla{K: "o", Rotulo: "editor"}, moldura.Tecla{K: "/", Rotulo: "filtra"})
	if temEquipe(m.itens) {
		estado := "ligada"
		if !m.propagar {
			estado = "desligada"
		}
		t = append(t, moldura.Tecla{K: "D", Rotulo: "dupla " + estado})
	}
	t = append(t, moldura.Tecla{K: "enter", Rotulo: "grava"}, moldura.Tecla{K: "q", Rotulo: "sai"},
		moldura.Tecla{K: "?", Rotulo: "ajuda"})
	return moldura.BarraTeclas(w, t)
}

func (m *modelo) gruposAjuda() []moldura.GrupoAjuda {
	t := func(k, r string) moldura.Tecla { return moldura.Tecla{K: k, Rotulo: r} }
	return []moldura.GrupoAjuda{
		{Titulo: "Correção", Teclas: []moldura.Tecla{
			t("j k, setas", "move na lista; com o foco em [2], rola"),
			t("tab", "alterna entre a lista e o detalhe"),
			t("pgup pgdown", "página"),
			t("0-9 n", "lança a nota, em [2]"),
			t("c", "escreve o comentário, em [2]"),
			t("r", "repete a última nota e o comentário dela"),
			t("x", "apaga a nota"),
			t("D", "liga e desliga a nota da dupla nos dois"),
			t("o", "abre o clone no $EDITOR"),
			t("/", "filtra por nome ou GRR"),
			t("enter", "grava e sai"),
			t("q esc", "sai sem gravar"),
			t("?", "esta ajuda"),
		}},
	}
}

// simboloDaSituacao acompanha a cor da situação: em dia, atenção ou falta.
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

func temEquipe(itens []Item) bool {
	for _, i := range itens {
		if len(i.Equipe) > 0 {
			return true
		}
	}
	return false
}

func instante(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Local().Format("02/01 15:04")
}

func commitCurto(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
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
