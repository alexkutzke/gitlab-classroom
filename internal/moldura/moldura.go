// Package moldura desenha os painéis da interface: borda arredondada com
// título, tabela alinhada, linha selecionada, barras de título e de teclas e a
// caixa de ajuda.
//
// Copiado de internal/tui/moldura.go e internal/tui/view.go do painel
// (~/Documents/work/dev/painel, commit 10224c0), com os nomes exportados
// porque aqui é pacote próprio: a interface e a correção usam os dois, e a
// correção não importa a interface. As três ferramentas (painel, diario e
// classroom) desenham com o mesmo código para parecerem uma só. Os pacotes
// internal/ não se importam entre módulos, e duzentas linhas copiadas custam
// menos que um quarto repositório; se as cópias divergirem, extrair um módulo
// comum passa a valer a pena.
package moldura

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Tamanho usado antes de o terminal informar o dele, e o mínimo em que a tela
// ainda faz sentido.
const (
	LarguraPadrao = 100
	AlturaPadrao  = 30
	LarguraMinima = 40
	AlturaMinima  = 12
	// Abaixo desta largura os painéis se empilham, e o detalhe ocupa a tela
	// quando está em foco, como o lazygit em meia tela.
	LarguraDuasColunas = 100
)

// Cores comuns às três ferramentas. As adaptativas mudam com o fundo do
// terminal: o cinza que se lê no escuro some no claro.
var (
	CorAcento  = lipgloss.Color("39")
	CorOK      = lipgloss.Color("35")
	CorAtencao = lipgloss.Color("214")
	CorErro    = lipgloss.Color("203")
	CorFraca   = lipgloss.AdaptiveColor{Light: "245", Dark: "243"}
	CorBorda   = lipgloss.AdaptiveColor{Light: "250", Dark: "238"}

	EstNormal  = lipgloss.NewStyle()
	EstFraco   = lipgloss.NewStyle().Foreground(CorFraca)
	EstOK      = lipgloss.NewStyle().Foreground(CorOK)
	EstAtencao = lipgloss.NewStyle().Foreground(CorAtencao)
	EstErro    = lipgloss.NewStyle().Foreground(CorErro)
	EstAcento  = lipgloss.NewStyle().Foreground(CorAcento)
	EstSecao   = lipgloss.NewStyle().Foreground(CorAcento).Bold(true)
	EstTecla   = lipgloss.NewStyle().Foreground(CorAcento).Bold(true)

	// A linha selecionada do painel em foco tem fundo forte; a dos outros
	// painéis, fundo discreto, para se ver o que o detalhe está mostrando.
	EstSelFoco = lipgloss.NewStyle().Bold(true).
			Background(lipgloss.AdaptiveColor{Light: "153", Dark: "24"}).
			Foreground(lipgloss.AdaptiveColor{Light: "16", Dark: "255"})
	EstSel = lipgloss.NewStyle().
		Background(lipgloss.AdaptiveColor{Light: "254", Dark: "236"})

	EstBarra = lipgloss.NewStyle().
			Background(lipgloss.AdaptiveColor{Light: "254", Dark: "236"}).
			Foreground(lipgloss.AdaptiveColor{Light: "235", Dark: "252"})
	EstMarca = lipgloss.NewStyle().Background(CorAcento).Foreground(lipgloss.Color("16")).Bold(true)
)

// Conteudo é o que vai dentro de um painel.
type Conteudo struct {
	Linhas []string
	// Sel é a linha selecionada, -1 sem seleção.
	Sel int
	// Topo é a rolagem pedida; Livre deixa a seleção sair de vista.
	Topo  int
	Livre bool
	// Info vai no canto da borda de baixo, como "2 de 4".
	Info string
}

// Recorte devolve a primeira linha visível, mantendo a selecionada à vista.
func (c Conteudo) Recorte(visiveis int) int {
	topo := c.Topo
	if c.Sel >= 0 && !c.Livre {
		if c.Sel < topo {
			topo = c.Sel
		}
		if c.Sel >= topo+visiveis {
			topo = c.Sel - visiveis + 1
		}
	}
	return max(0, min(topo, len(c.Linhas)-visiveis))
}

// Caixa desenha um painel com borda arredondada e título na borda de cima,
// como "╭─[1] Turmas───╮". O painel em foco ganha borda e título em azul.
func Caixa(num, titulo string, c Conteudo, w, h int, focado bool) string {
	if w < 4 || h < 2 {
		return ""
	}
	est := lipgloss.NewStyle().Foreground(CorBorda)
	estTitulo := lipgloss.NewStyle().Foreground(CorFraca)
	if focado {
		est = lipgloss.NewStyle().Foreground(CorAcento)
		estTitulo = lipgloss.NewStyle().Foreground(CorAcento).Bold(true)
	}
	interna := w - 2

	rotulo := Limpo(titulo)
	if num != "" {
		rotulo = "[" + num + "] " + rotulo
	}
	rotulo = ansi.Truncate(rotulo, max(0, interna-2), "")
	var b strings.Builder
	b.WriteString(est.Render("╭─") + estTitulo.Render(rotulo) +
		est.Render(strings.Repeat("─", max(0, interna-1-ansi.StringWidth(rotulo)))+"╮") + "\n")

	visiveis := h - 2
	topo := c.Recorte(visiveis)
	for i := range visiveis {
		l := ""
		if j := topo + i; j < len(c.Linhas) {
			l = c.Linhas[j]
			if j == c.Sel {
				l = Selecionar(l, interna, focado)
			}
		}
		b.WriteString(est.Render("│") + Preencher(l, interna) + est.Render("│") + "\n")
	}

	info := c.Info
	if info == "" && len(c.Linhas) > visiveis {
		info = "rola"
	}
	if info != "" {
		info = " " + info + " "
	}
	info = ansi.Truncate(info, max(0, interna-1), "")
	b.WriteString(est.Render("╰" + strings.Repeat("─", max(0, interna-ansi.StringWidth(info)-1))))
	b.WriteString(EstFraco.Render(info))
	b.WriteString(est.Render("─╯"))
	return b.String()
}

// Selecionar pinta a linha selecionada de ponta a ponta. As cores de cada
// célula saem: um fundo único é o que liga o nome ao dado da mesma linha.
func Selecionar(l string, largura int, focado bool) string {
	p := ansi.Strip(l)
	p = ansi.Truncate(p, largura, "")
	p += strings.Repeat(" ", max(0, largura-ansi.StringWidth(p)))
	if focado {
		return EstSelFoco.Render(p)
	}
	return EstSel.Render(p)
}

// Preencher corta ou completa a linha até a largura exata, contando colunas
// e ignorando os códigos de cor.
func Preencher(l string, largura int) string {
	l = ansi.Truncate(l, largura, "")
	return l + strings.Repeat(" ", max(0, largura-ansi.StringWidth(l)))
}

// Limpo troca tabulação e quebra de linha por espaço. Texto de aluno e
// mensagem de erro chegam com eles, e o terminal desenha a tabulação com
// mais colunas do que a medida conta, o que desalinha a borda.
func Limpo(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\t' || r == '\n' || r == '\r' {
			return ' '
		}
		return r
	}, s)
}

// Celula é um valor de tabela com cor própria.
type Celula struct {
	Texto  string
	Est    lipgloss.Style
	Direta bool // alinhada à direita, para números
}

// Cel é a célula alinhada à esquerda.
func Cel(t string, est lipgloss.Style) Celula { return Celula{Texto: Limpo(t), Est: est} }

// Num é a célula alinhada à direita.
func Num(t string, est lipgloss.Style) Celula { return Celula{Texto: Limpo(t), Est: est, Direta: true} }

// Tabela alinha as colunas pela largura do texto, antes da cor, para os
// códigos de cor não desalinharem nada.
func Tabela(cab []Celula, linhas [][]Celula) []string {
	larg := make([]int, len(cab))
	for i, c := range cab {
		larg[i] = ansi.StringWidth(c.Texto)
	}
	for _, l := range linhas {
		for i, c := range l {
			if i < len(larg) {
				larg[i] = max(larg[i], ansi.StringWidth(c.Texto))
			}
		}
	}
	monta := func(cs []Celula) string {
		var b strings.Builder
		b.WriteString(" ")
		for i, c := range cs {
			if i >= len(larg) {
				break
			}
			falta := strings.Repeat(" ", max(0, larg[i]-ansi.StringWidth(c.Texto)))
			if c.Direta {
				b.WriteString(falta + c.Est.Render(c.Texto))
			} else {
				b.WriteString(c.Est.Render(c.Texto) + falta)
			}
			if i < len(cs)-1 {
				b.WriteString("  ")
			}
		}
		return b.String()
	}
	r := []string{monta(cab)}
	for _, l := range linhas {
		r = append(r, monta(l))
	}
	return r
}

// LarguraLinhas é a largura da linha mais larga, mais a margem que a
// borda pede.
func LarguraLinhas(linhas []string) int {
	l := 0
	for _, s := range linhas {
		l = max(l, ansi.StringWidth(s)+1)
	}
	return l
}

// LarguraLista decide a largura da coluna da esquerda: a da tabela, para
// nenhuma coluna sumir, deixando ao menos 40 colunas para o detalhe.
func LarguraLista(tabela, w int) int {
	return max(40, min(tabela+2, w-40))
}

// Quebrar divide o texto em linhas de até largura colunas, nas palavras. As
// quebras de linha do próprio texto são mantidas, e linha em branco entre
// parágrafos também.
func Quebrar(s string, largura int) []string {
	largura = max(1, largura)
	var r []string
	for _, paragrafo := range strings.Split(strings.ReplaceAll(s, "\r", ""), "\n") {
		linha := ""
		palavras := strings.Fields(strings.ReplaceAll(paragrafo, "\t", " "))
		if len(palavras) == 0 {
			r = append(r, "")
			continue
		}
		for _, p := range palavras {
			for ansi.StringWidth(p) > largura {
				if linha != "" {
					r = append(r, linha)
					linha = ""
				}
				corte := ansi.Truncate(p, largura, "")
				if corte == "" {
					// Caractere largo numa coluna estreita demais: vai
					// sozinho, senão o laço não termina.
					corte = string([]rune(p)[:1])
				}
				r = append(r, corte)
				p = p[len(corte):]
			}
			if linha != "" && ansi.StringWidth(linha)+1+ansi.StringWidth(p) > largura {
				r = append(r, linha)
				linha = ""
			}
			if p == "" {
				continue
			}
			if linha != "" {
				linha += " "
			}
			linha += p
		}
		if linha != "" {
			r = append(r, linha)
		}
	}
	return r
}

// Encaixar corta ou completa o texto até o retângulo exato, linha a linha.
func Encaixar(s string, w, h int) string {
	linhas := strings.Split(s, "\n")
	if len(linhas) > h {
		linhas = linhas[:h]
	}
	for len(linhas) < h {
		linhas = append(linhas, "")
	}
	for i, l := range linhas {
		linhas[i] = Preencher(l, w)
	}
	return strings.Join(linhas, "\n")
}

// Centralizar põe o texto no meio da tela, que fica com o tamanho exato.
func Centralizar(s string, w, h int) string {
	return Encaixar(lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, s), w, h)
}

// Pequena é a tela que pede um terminal maior.
func Pequena(w, h int) string {
	return Centralizar(EstAtencao.Render("aumente o terminal"), w, h)
}

// Trecho é um pedaço de texto da barra de título, com a cor dele.
type Trecho struct {
	Texto string
	Est   lipgloss.Style
}

// BarraTitulo desenha a primeira linha: o nome da ferramenta em bloco e,
// sobre fundo discreto, os trechos da esquerda e os da direita. Quando não
// cabe, a direita sai primeiro, e depois a esquerda é cortada.
func BarraTitulo(w int, nome string, esquerda, direita []Trecho) string {
	marca := EstMarca.Render(" " + nome + " ")
	resto := w - ansi.StringWidth(marca)
	if resto < 0 {
		return Preencher(marca, w)
	}
	monta := func(ts []Trecho) (string, int) {
		var b strings.Builder
		largura := 0
		for _, t := range ts {
			texto := Limpo(t.Texto)
			b.WriteString(t.Est.Inherit(EstBarra).Render(texto))
			largura += ansi.StringWidth(texto)
		}
		return b.String(), largura
	}
	esq, le := monta(esquerda)
	dir, ld := monta(direita)
	if le+ld+1 > resto {
		dir, ld = "", 0
	}
	if le > resto {
		esq = ansi.Truncate(esq, resto, "")
		le = ansi.StringWidth(esq)
	}
	return marca + esq + EstBarra.Render(strings.Repeat(" ", resto-le-ld)) + dir
}

// Tecla é uma entrada da barra de teclas e da ajuda.
type Tecla struct{ K, Rotulo string }

// BarraTeclas desenha a última linha: tecla em azul e negrito, rótulo em
// cinza, separados por três espaços.
func BarraTeclas(w int, teclas []Tecla) string {
	var partes []string
	for _, t := range teclas {
		partes = append(partes, EstTecla.Render(t.K)+" "+EstFraco.Render(t.Rotulo))
	}
	return Preencher(" "+strings.Join(partes, "   "), w)
}

// BarraMensagem ocupa a linha de teclas com uma mensagem de estado.
func BarraMensagem(w int, texto string, est lipgloss.Style) string {
	return Preencher(" "+est.Render(Limpo(texto)), w)
}

// GrupoAjuda reúne as teclas de uma tela na caixa de ajuda.
type GrupoAjuda struct {
	Titulo string
	Teclas []Tecla
}

// Ajuda desenha a caixa centralizada com todas as teclas, agrupadas. Os
// grupos se distribuem em colunas quando não cabem na altura da tela. Quando
// nem assim cabem na largura, os últimos saem e o rodapé diz quais: quem
// chama põe primeiro os grupos que mais importam na tela corrente.
func Ajuda(w, h int, grupos []GrupoAjuda, rodape string) string {
	// Borda e margem interna tomam quatro linhas, o rodapé mais duas.
	altura := max(3, h-6)
	largura := max(10, w-6)
	var fora []string
	corpo := colunasDeAjuda(grupos, altura)
	for len(grupos) > 1 && lipgloss.Width(corpo) > largura {
		fora = append([]string{grupos[len(grupos)-1].Titulo}, fora...)
		grupos = grupos[:len(grupos)-1]
		corpo = colunasDeAjuda(grupos, altura)
	}
	if len(fora) > 0 {
		corpo += "\n\n" + EstAtencao.Render("sem espaço para "+strings.Join(fora, ", ")+"; aumente o terminal")
	}
	if rodape != "" {
		corpo += "\n\n" + EstFraco.Render(rodape)
	}
	caixa := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(CorAcento).
		Padding(1, 2).Render(corpo)
	return Centralizar(caixa, w, h)
}

// colunasDeAjuda empilha os grupos em colunas de até altura linhas, cada
// coluna com a própria largura de tecla.
func colunasDeAjuda(grupos []GrupoAjuda, altura int) string {
	var colunas [][]GrupoAjuda
	var atual []GrupoAjuda
	linhas := 0
	for _, g := range grupos {
		precisa := len(g.Teclas) + 1
		if len(atual) > 0 {
			precisa++
		}
		if len(atual) > 0 && linhas+precisa > altura {
			colunas = append(colunas, atual)
			atual, linhas = nil, 0
			precisa = len(g.Teclas) + 1
		}
		atual = append(atual, g)
		linhas += precisa
	}
	if len(atual) > 0 {
		colunas = append(colunas, atual)
	}

	partes := make([]string, 0, 2*len(colunas))
	for i, c := range colunas {
		largTecla := 0
		for _, g := range c {
			for _, t := range g.Teclas {
				largTecla = max(largTecla, ansi.StringWidth(t.K))
			}
		}
		var b []string
		for j, g := range c {
			if j > 0 {
				b = append(b, "")
			}
			b = append(b, EstSecao.Render(g.Titulo))
			for _, t := range g.Teclas {
				k := t.K + strings.Repeat(" ", largTecla-ansi.StringWidth(t.K))
				b = append(b, "  "+EstTecla.Render(k)+"  "+t.Rotulo)
			}
		}
		if i > 0 {
			partes = append(partes, "    ")
		}
		partes = append(partes, strings.Join(b, "\n"))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, partes...)
}
