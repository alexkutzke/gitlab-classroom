package tui

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/alexkutzke/gitlab-classroom/internal/acoes"
	"github.com/alexkutzke/gitlab-classroom/internal/moldura"
	"github.com/alexkutzke/gitlab-classroom/internal/turma"
)

// painel é a tela inicial: [1] os exercícios com as contagens do resumo,
// [2] o detalhe do selecionado, [3] o que falta fazer e, com tarefa em curso
// ou recente, [4] o andamento dela.
type painel struct {
	cursor     int
	cursorPend int
}

func (p *painel) digitando() bool { return false }

func (p *painel) atualizar(a *App, msg tea.KeyMsg) (tea.Cmd, bool) {
	switch a.focoAtual() {
	case focoPendencias:
		total := len(a.panorama.Pendencias())
		switch msg.String() {
		case "up", "k":
			p.cursorPend = max(0, p.cursorPend-1)
			return nil, true
		case "down", "j":
			p.cursorPend = max(0, min(total-1, p.cursorPend+1))
			return nil, true
		case "home", "g":
			p.cursorPend = 0
			return nil, true
		case "end", "G":
			p.cursorPend = max(0, total-1)
			return nil, true
		case "enter":
			p.irParaPendencia(a)
			return nil, true
		}
	case focoTarefa:
		if msg.String() == "enter" {
			a.ir(idTarefas)
			return nil, true
		}
	}

	total := len(a.panorama.Exercicios)
	switch msg.String() {
	case "up", "k":
		p.cursor = max(0, p.cursor-1)
	case "down", "j":
		p.cursor = max(0, min(total-1, p.cursor+1))
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

// irParaPendencia leva à origem da pendência: o exercício dela em [1], ou a
// lista de alunos com cadastro a resolver.
func (p *painel) irParaPendencia(a *App) {
	pend := a.panorama.Pendencias()
	if p.cursorPend < 0 || p.cursorPend >= len(pend) {
		return
	}
	texto := pend[p.cursorPend]
	for i, r := range a.panorama.Exercicios {
		id := r.Exercicio.ID
		if strings.HasPrefix(texto, id+":") || strings.HasPrefix(texto, id+" venceu") {
			p.cursor = i
			a.foco[idPainel] = focoLista
			return
		}
	}
	if len(a.panorama.ContasPendentes) > 0 {
		a.alunos.soPendentes = true
		a.alunos.cursor = 0
		a.ir(idAlunos)
	}
}

func (p *painel) teclas(a *App) []moldura.Tecla {
	r := []moldura.Tecla{{K: "tab", Rotulo: "painel"}}
	switch a.focoAtual() {
	case focoDetalhe:
		return append(teclasDoDetalhe(), a.teclasComuns()...)
	case focoPendencias:
		r = append(r, moldura.Tecla{K: "j/k", Rotulo: "move"}, moldura.Tecla{K: "enter", Rotulo: "vai à origem"})
	case focoTarefa:
		r = append(r, moldura.Tecla{K: "enter", Rotulo: "tarefas"})
	default:
		r = append(r, moldura.Tecla{K: "j/k", Rotulo: "move"}, moldura.Tecla{K: "enter", Rotulo: "entregas"})
	}
	r = append(r,
		moldura.Tecla{K: "n", Rotulo: "corrige"}, moldura.Tecla{K: "c", Rotulo: "coleta"},
		moldura.Tecla{K: "E", Rotulo: "exporta"}, moldura.Tecla{K: "S", Rotulo: "sincroniza"},
		moldura.Tecla{K: "C", Rotulo: "coleta tudo"})
	return append(r, a.teclasComuns()...)
}

// retangulo é o tamanho de um painel, com borda.
type retangulo struct{ w, h int }

// dispPainel é a divisão da tela inicial.
type dispPainel struct {
	estreita                                bool
	temTarefa                               bool
	exercicios, detalhe, pendencias, tarefa retangulo
}

// dispor reparte o corpo: [1] e [3] empilhados à esquerda, na largura da
// tabela, [2] à direita e [4] embaixo, na largura toda, quando existe.
func (p *painel) dispor(a *App, w, h int) dispPainel {
	d := dispPainel{estreita: w < moldura.LarguraDuasColunas, temTarefa: a.tarefaVisivel()}
	altTarefa := 0
	if d.temTarefa {
		altTarefa = max(3, min(8, h/3))
	}
	cima := h - altTarefa
	altPend := max(3, min(len(a.panorama.Pendencias())+2, cima/3))
	d.tarefa = retangulo{w, altTarefa}
	esq := w
	if d.estreita {
		d.detalhe = retangulo{w, h}
	} else {
		esq = moldura.LarguraLista(moldura.LarguraLinhas(p.linhasExercicios(a)), w)
		d.detalhe = retangulo{w - esq, cima}
	}
	d.exercicios = retangulo{esq, cima - altPend}
	d.pendencias = retangulo{esq, altPend}
	return d
}

func (p *painel) corpo(a *App, w, h int) string {
	d := p.dispor(a, w, h)
	foco := a.focoAtual()
	ex := p.caixaExercicios(a, d.exercicios, foco == focoLista)
	pend := p.caixaPendencias(a, d.pendencias, foco == focoPendencias)

	var partes []string
	switch {
	case d.estreita && foco == focoDetalhe:
		return a.caixaDetalhe(d.detalhe.w, d.detalhe.h)
	case d.estreita:
		partes = []string{ex, pend}
	default:
		esq := lipgloss.JoinVertical(lipgloss.Left, ex, pend)
		partes = []string{lipgloss.JoinHorizontal(lipgloss.Top, esq,
			a.caixaDetalhe(d.detalhe.w, d.detalhe.h))}
	}
	if d.temTarefa {
		partes = append(partes, p.caixaTarefa(a, d.tarefa, foco == focoTarefa))
	}
	return lipgloss.JoinVertical(lipgloss.Left, partes...)
}

// resumoDe acha a linha do exercício no resumo, que traz as contagens que
// `classroom resumo` publica.
func resumoDe(a *App, id string) acoes.ResumoExercicioContagem {
	for _, r := range a.resumo.Exercicios {
		if r.ID == id {
			return r
		}
	}
	return acoes.ResumoExercicioContagem{ID: id}
}

func (p *painel) linhasExercicios(a *App) []string {
	categorias := a.panorama.VariasCategorias()
	comErro := false
	for _, r := range a.resumo.Exercicios {
		comErro = comErro || r.Erros > 0
	}

	cab := []moldura.Celula{moldura.Cel("id", estFraco)}
	if categorias {
		cab = append(cab, moldura.Cel("categoria", estFraco))
	}
	cab = append(cab, moldura.Cel("prazo", estFraco), moldura.Num("entregues", estFraco),
		moldura.Num("atras", estFraco), moldura.Num("sem", estFraco))
	if comErro {
		cab = append(cab, moldura.Num("erro", estFraco))
	}
	cab = append(cab, moldura.Num("corrig", estFraco), moldura.Num("a corrig", estFraco),
		moldura.Num("verif", estFraco))

	var linhas [][]moldura.Celula
	for _, r := range a.panorama.Exercicios {
		e := r.Exercicio
		c := resumoDe(a, e.ID)

		estID := estNormal
		switch {
		case !r.Vencido:
			estID = estFraco
		case !r.Coletado:
			estID = estErro
		}
		l := []moldura.Celula{moldura.Cel(e.ID, estID)}
		if categorias {
			l = append(l, moldura.Cel(e.CategoriaDe(), estFraco))
		}
		l = append(l, moldura.Cel(e.Prazo.Curta(), estID))

		if !r.Coletado {
			l = append(l, moldura.Num("-", estFraco), moldura.Num("-", estFraco), moldura.Num("-", estFraco))
			if comErro {
				l = append(l, moldura.Num("-", estFraco))
			}
		} else {
			l = append(l, numero(c.Entregues, estOK), numero(c.Atrasadas, estAtencao),
				numero(c.SemEntrega, estErro))
			if comErro {
				l = append(l, numero(c.Erros, estErro))
			}
		}
		l = append(l, numero(c.Corrigidas, estNormal), numero(c.PorCorrigir, estAtencao))

		verif := moldura.Num("·", estFraco)
		switch {
		case r.VerificacoesVelhas > 0:
			verif = moldura.Num(fmt.Sprintf("▲ %d", r.VerificacoesVelhas), estAtencao)
		case e.TemSuite() && len(r.Verificacoes) > 0:
			verif = moldura.Num("✓", estOK)
		}
		linhas = append(linhas, append(l, verif))
	}
	return moldura.Tabela(cab, linhas)
}

// numero pinta a contagem com a cor do sentido dela, e zero em fraco: zero é
// nada a registrar.
func numero(n int, est lipgloss.Style) moldura.Celula {
	if n == 0 {
		return moldura.Num("0", estFraco)
	}
	return moldura.Num(strconv.Itoa(n), est)
}

func (p *painel) caixaExercicios(a *App, r retangulo, focado bool) string {
	c := moldura.Conteudo{Linhas: p.linhasExercicios(a), Sel: -1}
	if n := len(a.panorama.Exercicios); n == 0 {
		c.Linhas = []string{estFraco.Render(" nenhum exercício; x cadastra o primeiro")}
	} else {
		p.cursor = min(p.cursor, n-1)
		c.Sel = 1 + p.cursor
		c.Info = fmt.Sprintf("%d de %d", p.cursor+1, n)
	}
	return moldura.Caixa("1", "Exercícios", c, r.w, r.h, focado)
}

func (p *painel) caixaPendencias(a *App, r retangulo, focado bool) string {
	pend := a.panorama.Pendencias()
	c := moldura.Conteudo{Sel: -1}
	titulo := "Pendências"
	if len(pend) == 0 {
		c.Linhas = []string{estOK.Render(" ✓ nada pendente")}
	} else {
		titulo = fmt.Sprintf("Pendências (%d)", len(pend))
		for _, t := range pend {
			c.Linhas = append(c.Linhas, " "+estAtencao.Render("▲")+" "+t)
		}
		p.cursorPend = min(p.cursorPend, len(pend)-1)
		if focado {
			c.Sel = p.cursorPend
			c.Info = fmt.Sprintf("%d de %d", p.cursorPend+1, len(pend))
		}
	}
	return moldura.Caixa("3", titulo, c, r.w, r.h, focado)
}

func (p *painel) caixaTarefa(a *App, r retangulo, focado bool) string {
	titulo := "Tarefa"
	var c moldura.Conteudo
	switch {
	case a.emCurso():
		titulo += " · " + a.tarefa.nome
		c.Linhas = []string{" " + a.barraDeProgresso()}
		if reg := a.tarefa.registro; reg != nil {
			for _, l := range reg.linhas {
				c.Linhas = append(c.Linhas, "   "+l)
			}
		}
		c.Info = "em curso"
	default:
		if reg := a.tarefas.ultimaOperacao(); reg != nil {
			titulo += " · " + reg.nome
			c.Linhas = linhasDoRegistro(reg)
			c.Info = reg.estado.String()
		}
	}
	// As últimas linhas são as que importam: o resumo e os erros que
	// chegaram por último.
	c.Sel, c.Topo, c.Livre = -1, len(c.Linhas), true
	return moldura.Caixa("4", titulo, c, r.w, r.h, focado)
}

// detalhe do exercício selecionado em [1].
func (p *painel) detalhe(a *App, largura int) (string, moldura.Conteudo) {
	if len(a.panorama.Exercicios) == 0 {
		return "Detalhe", moldura.Conteudo{Sel: -1,
			Linhas: []string{estFraco.Render(" nenhum exercício cadastrado")}}
	}
	r := a.panorama.Exercicios[min(p.cursor, len(a.panorama.Exercicios)-1)]
	e := r.Exercicio
	c := resumoDe(a, e.ID)
	titulo := e.ID
	if e.Titulo != "" {
		titulo += " · " + e.Titulo
	}

	d := novoDetalhe(largura)
	d.campo("prazo", e.Prazo.Curta()+", "+distanciaDoPrazo(e.Prazo))
	d.campo("peso", fmt.Sprintf("%g, categoria %s", e.Peso, e.CategoriaDe()))
	d.campo("repositório", e.Repo)
	if e.TemSuite() {
		d.campo("suíte", e.Verificacao)
		imagem := e.Imagem
		if imagem == "" {
			imagem = estFraco.Render("padrão, " + a.turma.Config.ImagemVerificacao)
		}
		d.campo("imagem", imagem)
	} else {
		d.campo("suíte", estFraco.Render("sem suíte, correção à mão"))
	}

	d.secao("ENTREGAS")
	if !r.Coletado {
		d.linha(estFraco.Render(" · nunca coletado; c coleta"))
	} else {
		semColeta := a.panorama.Ativos
		for _, s := range ordemDasSituacoes {
			n := r.Situacoes[s]
			if n == 0 {
				continue
			}
			semColeta -= n
			d.contagem(simboloDaSituacao(s), s.Rotulo(), n, corDaSituacao(s))
		}
		if semColeta > 0 {
			d.contagem("·", "sem linha na coleta", semColeta, estFraco)
		}
		if r.Compartilhadas > 0 {
			d.contagem("·", "em dupla, no fork do colega", r.Compartilhadas, estFraco)
		}
	}

	d.secao("CORREÇÃO")
	d.contagem("✓", "notas lançadas", r.Notas, estOK)
	estSem := estFraco
	simbolo := "·"
	if r.SemNota > 0 {
		estSem, simbolo = estAtencao, "●"
	}
	d.contagem(simbolo, "entregues sem nota", r.SemNota, estSem)
	estDev := estFraco
	simbolo = "·"
	if c.DevolutivasPendentes > 0 {
		estDev, simbolo = estAtencao, "●"
	}
	d.contagem(simbolo, "devolutivas a publicar", c.DevolutivasPendentes, estDev)

	if e.TemSuite() || len(r.Verificacoes) > 0 {
		d.secao("VERIFICAÇÃO")
		if len(r.Verificacoes) == 0 {
			d.linha(estFraco.Render(" · nenhuma verificação; v roda a suíte"))
		}
		for _, s := range ordemDasVerificacoes {
			if n := r.Verificacoes[s]; n > 0 {
				d.contagem(simboloDaVerificacao(s), string(s), n, corDaVerificacao(s))
			}
		}
		if r.VerificacoesVelhas > 0 {
			d.contagem("▲", "sobre commit antigo", r.VerificacoesVelhas, estAtencao)
		}
	}
	return titulo, d.conteudo()
}

// ordemDasSituacoes lista as situações da melhor para a pior, que é a ordem
// em que o detalhe as mostra.
var ordemDasSituacoes = []turma.SituacaoEntrega{
	turma.Entregue, turma.SemCommitNoPrazo, turma.ForkSemCommit, turma.SemFork,
	turma.SemAcesso, turma.GrupoInvisivel, turma.SemConta, turma.Erro,
}

var ordemDasVerificacoes = []turma.SituacaoVerificacao{
	turma.Aprovado, turma.Reprovado, turma.ErroVerificacao, turma.SemClone,
}

func simboloDaVerificacao(s turma.SituacaoVerificacao) string {
	switch s {
	case turma.Aprovado:
		return "✓"
	case turma.Reprovado, turma.ErroVerificacao:
		return "✖"
	}
	return "·"
}

// distanciaDoPrazo diz quanto falta para o prazo ou quanto passou dele.
func distanciaDoPrazo(prazo turma.Data) string {
	dias := int(prazo.Sub(turma.Hoje().Time).Hours() / 24)
	switch {
	case dias == 0:
		return estAtencao.Render("vence hoje")
	case dias == 1:
		return estFraco.Render("vence amanhã")
	case dias > 1:
		return estFraco.Render(fmt.Sprintf("faltam %d dias", dias))
	case dias == -1:
		return "venceu ontem"
	}
	return fmt.Sprintf("venceu há %d dias", -dias)
}
