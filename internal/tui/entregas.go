package tui

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/alexkutzke/gitlab-classroom/internal/acoes"
	"github.com/alexkutzke/gitlab-classroom/internal/moldura"
	"github.com/alexkutzke/gitlab-classroom/internal/repo"
	"github.com/alexkutzke/gitlab-classroom/internal/turma"
)

// ordem controla como as linhas da tela de entregas são classificadas.
type ordem int

const (
	ordemNome ordem = iota
	ordemSituacao
	ordemNota
)

func (o ordem) String() string {
	switch o {
	case ordemSituacao:
		return "situação"
	case ordemNota:
		return "nota"
	}
	return "nome"
}

// linhaEntrega junta, para um aluno, tudo o que a tela precisa mostrar.
type linhaEntrega struct {
	Aluno       turma.Aluno
	Entrega     turma.Entrega
	Nota        *turma.Nota
	Verificacao turma.Verificacao
	Equipe      []string
}

// telaEntregas lista os alunos de um exercício.
type telaEntregas struct {
	cursor int
	topo   int
	ordem  ordem

	filtro     string
	modoFiltro bool
	buffer     string

	// vinculando guarda o aluno que está esperando o dono do fork ser
	// escolhido, no cadastro de entrega em dupla.
	vinculando *turma.Aluno
}

func (te *telaEntregas) reiniciar() {
	te.cursor, te.topo = 0, 0
	te.filtro, te.buffer, te.modoFiltro = "", "", false
	te.vinculando = nil
}

// linhas monta a lista já filtrada e ordenada.
func (te *telaEntregas) linhas(a *App) []linhaEntrega {
	e, ok := a.exercicioAberto()
	if !ok {
		return nil
	}
	entregas := a.turma.EntregasDoExercicio(e.ID)
	verificacoes := a.turma.VerificacoesDoExercicio(e.ID)

	var out []linhaEntrega
	for _, al := range turma.Busca(a.turma.Ativos(), te.filtro) {
		l := linhaEntrega{
			Aluno:       al,
			Entrega:     entregas[al.GRR],
			Verificacao: verificacoes[al.GRR],
		}
		if n, ok := a.turma.Nota(e.ID, al.GRR); ok {
			l.Nota = n
		}
		if equipe := a.turma.Equipe(e.ID, al.GRR); len(equipe) > 1 {
			l.Equipe = acoes.NomesDaEquipe(a.turma, equipe, al.GRR)
		}
		out = append(out, l)
	}

	switch te.ordem {
	case ordemSituacao:
		// Quem está pior primeiro: é para esses que o professor precisa
		// fazer alguma coisa.
		sort.SliceStable(out, func(i, j int) bool {
			return gravidade(out[i].Entrega.Situacao) > gravidade(out[j].Entrega.Situacao)
		})
	case ordemNota:
		sort.SliceStable(out, func(i, j int) bool {
			return valorDaNota(out[i].Nota) < valorDaNota(out[j].Nota)
		})
	}
	return out
}

// gravidade ordena as situações da pior para a melhor.
func gravidade(s turma.SituacaoEntrega) int {
	switch s {
	case turma.Erro:
		return 6
	case turma.SemConta, turma.GrupoInvisivel, turma.SemAcesso:
		return 5
	case turma.SemFork:
		return 4
	case turma.ForkSemCommit:
		return 3
	case turma.SemCommitNoPrazo:
		return 2
	case "":
		return 1
	}
	return 0
}

// valorDaNota deixa quem ainda não tem nota no começo da ordenação por nota.
func valorDaNota(n *turma.Nota) float64 {
	if n == nil {
		return -1
	}
	return n.Valor
}

func (te *telaEntregas) atualizar(a *App, msg tea.KeyMsg) (tea.Cmd, bool) {
	if te.modoFiltro {
		return te.teclaFiltro(msg), true
	}

	linhas := te.linhas(a)
	switch msg.String() {
	case "up", "k":
		te.cursor = max(0, te.cursor-1)
	case "down", "j":
		te.cursor = max(0, min(len(linhas)-1, te.cursor+1))
	case "pgup":
		te.cursor = max(0, te.cursor-a.linhasDisponiveis())
	case "pgdown":
		te.cursor = max(0, min(len(linhas)-1, te.cursor+a.linhasDisponiveis()))
	case "home", "g":
		te.cursor = 0
	case "end", "G":
		te.cursor = max(0, len(linhas)-1)
	case "/":
		te.modoFiltro, te.buffer = true, te.filtro
	case "s":
		te.ordem = (te.ordem + 1) % 3
		te.cursor = 0
		a.avisar("ordenado por %s", te.ordem)
	case "enter":
		// No modo vínculo, o enter escolhe o dono do fork.
		if te.vinculando != nil {
			l, ok := linhaAtual(linhas, te.cursor)
			if ok {
				a.vincular(*te.vinculando, l.Aluno)
			}
			te.vinculando = nil
			return nil, true
		}
		return nil, false
	case "n":
		e, ok := a.exercicioAberto()
		if !ok {
			return nil, true
		}
		return a.abrirCorrecao(e, acoes.FiltroCorrecao{}), true
	case "N":
		e, ok := a.exercicioAberto()
		if !ok {
			return nil, true
		}
		return a.abrirCorrecao(e, acoes.FiltroCorrecao{SemNota: true}), true
	case "V":
		l, ok := linhaAtual(linhas, te.cursor)
		if !ok {
			return nil, true
		}
		te.vinculando = &l.Aluno
	case "X":
		l, ok := linhaAtual(linhas, te.cursor)
		if !ok {
			return nil, true
		}
		v, ok := a.turma.VinculosDoExercicio(a.exercicio)[l.Aluno.GRR]
		if !ok {
			a.erro = l.Aluno.Nome + " não entrega no fork de ninguém"
			return nil, true
		}
		a.desvincular(a.exercicio, v, l.Aluno.Nome)
	case "esc":
		if te.vinculando != nil {
			te.vinculando = nil
			a.avisar("vínculo cancelado")
			return nil, true
		}
		return nil, false
	case "c":
		e, ok := a.exercicioAberto()
		if !ok {
			return nil, true
		}
		return a.coletar([]turma.Exercicio{e}), true
	case "l":
		e, ok := a.exercicioAberto()
		if !ok {
			return nil, true
		}
		return a.clonar(e), true
	case "v":
		e, ok := a.exercicioAberto()
		if !ok {
			return nil, true
		}
		return a.verificar(e), true
	case "o":
		return te.abrirClone(a, linhas), true
	case "w":
		te.abrirNavegador(a, linhas)
	default:
		return nil, false
	}
	return nil, true
}

func (te *telaEntregas) teclaFiltro(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		te.modoFiltro, te.buffer, te.filtro = false, "", ""
		te.cursor = 0
	case "enter":
		te.filtro, te.modoFiltro, te.buffer = te.buffer, false, ""
		te.cursor = 0
	case "backspace":
		if te.buffer != "" {
			te.buffer = te.buffer[:len(te.buffer)-1]
		}
	case "space":
		te.buffer += " "
	default:
		if t := msg.String(); len(t) == 1 {
			te.buffer += t
		}
	}
	return nil
}

// abrirClone entrega a pasta do aluno ao editor e devolve o terminal à
// interface quando ele fecha.
func (te *telaEntregas) abrirClone(a *App, linhas []linhaEntrega) tea.Cmd {
	l, ok := linhaAtual(linhas, te.cursor)
	if !ok {
		return nil
	}
	dir := acoes.DirDaEntrega(a.turma, a.store.Pasta(), a.exercicio, l.Aluno)
	if !repo.Existe(dir) {
		a.erro = "clone ausente em " + dir + "; use c para clonar"
		return nil
	}
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor == "" {
		a.erro = "defina $EDITOR para abrir o repositório"
		return nil
	}
	partes := strings.Fields(editor)
	cmd := exec.Command(partes[0], append(partes[1:], dir)...)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		if err != nil {
			return erroMsg{err}
		}
		return nil
	})
}

func (te *telaEntregas) abrirNavegador(a *App, linhas []linhaEntrega) {
	l, ok := linhaAtual(linhas, te.cursor)
	if !ok {
		return
	}
	if l.Entrega.Projeto == "" || !strings.Contains(l.Entrega.Projeto, "/") {
		a.erro = "sem fork coletado para " + l.Aluno.Nome
		return
	}
	url := strings.TrimSuffix(a.turma.Config.Host, "/") + "/" + l.Entrega.Projeto
	if _, err := exec.LookPath("xdg-open"); err != nil {
		a.avisar("%s", url)
		return
	}
	if err := exec.Command("xdg-open", url).Start(); err != nil {
		a.erro = err.Error()
		return
	}
	a.avisar("abrindo %s", url)
}

func linhaAtual(linhas []linhaEntrega, cursor int) (linhaEntrega, bool) {
	if cursor < 0 || cursor >= len(linhas) {
		return linhaEntrega{}, false
	}
	return linhas[cursor], true
}

func (te *telaEntregas) digitando() bool { return te.modoFiltro }

// entrada mostra o filtro sendo digitado ou a pergunta do vínculo.
func (te *telaEntregas) entrada(a *App) string {
	switch {
	case te.modoFiltro:
		return estAcento.Render("filtro: ") + te.buffer + "_   " +
			estTecla.Render("enter") + " " + estFraco.Render("aplica") + "   " +
			estTecla.Render("esc") + " " + estFraco.Render("limpa")
	case te.vinculando != nil:
		return estAtencao.Render("escolha o dono do fork onde "+te.vinculando.Nome+" entregou") + "   " +
			estTecla.Render("enter") + " " + estFraco.Render("escolhe") + "   " +
			estTecla.Render("esc") + " " + estFraco.Render("cancela")
	}
	return ""
}

func (te *telaEntregas) teclas(a *App) []moldura.Tecla {
	if a.focoAtual() == focoDetalhe {
		return append(teclasDoDetalhe(), a.teclasComuns()...)
	}
	r := []moldura.Tecla{
		{K: "tab", Rotulo: "painel"}, {K: "n", Rotulo: "corrige"}, {K: "c", Rotulo: "coleta"},
		{K: "l", Rotulo: "clona"}, {K: "v", Rotulo: "verifica"}, {K: "o", Rotulo: "editor"},
		{K: "w", Rotulo: "GitLab"}, {K: "V", Rotulo: "vincula"}, {K: "s", Rotulo: "ordem"},
		{K: "/", Rotulo: "filtra"},
	}
	return append(r, a.teclasComuns()...)
}

// lista é o painel [1]: os alunos ativos, com a marca d da entrega em dupla.
func (te *telaEntregas) painelLista(a *App) (string, moldura.Conteudo) {
	e, ok := a.exercicioAberto()
	if !ok {
		return "Entregas", moldura.Conteudo{Sel: -1,
			Linhas: []string{estErro.Render(" exercício não encontrado")}}
	}
	linhas := te.linhas(a)
	total := len(a.turma.Ativos())
	titulo := fmt.Sprintf("Entregas · %s (%d)", e.ID, total)
	if te.filtro != "" {
		titulo = fmt.Sprintf("Entregas · %s (%d de %d, filtro %q)", e.ID, len(linhas), total, te.filtro)
	}
	if te.ordem != ordemNome {
		titulo += " · ordem por " + te.ordem.String()
	}
	if len(linhas) == 0 {
		return titulo, moldura.Conteudo{Sel: -1,
			Linhas: []string{estFraco.Render(" nenhum aluno com esse filtro")}}
	}

	cab := []moldura.Celula{moldura.Cel(" ", estFraco), moldura.Cel("aluno", estFraco),
		moldura.Cel("situação", estFraco), moldura.Num("commits", estFraco),
		moldura.Num("atraso", estFraco), moldura.Cel("verif", estFraco), moldura.Num("nota", estFraco)}
	var celulas [][]moldura.Celula
	for _, l := range linhas {
		marca := moldura.Cel(" ", estFraco)
		if len(l.Equipe) > 0 {
			marca = moldura.Cel("d", estFraco)
		}
		situacao := moldura.Cel("sem coleta", estFraco)
		if s := l.Entrega.Situacao; s != "" {
			texto := s.Rotulo()
			if s == turma.Entregue && l.Entrega.TemAtraso() {
				texto += fmt.Sprintf(" (+%dd)", l.Entrega.AtrasoDias)
			}
			situacao = moldura.Cel(texto, corDaSituacao(s))
		}
		commits := moldura.Num("-", estFraco)
		if l.Entrega.Commits > 0 {
			commits = moldura.Num(fmt.Sprint(l.Entrega.Commits), estNormal)
		}
		atraso := moldura.Num("-", estFraco)
		if l.Entrega.TemAtraso() {
			atraso = moldura.Num(fmt.Sprintf("%dd", l.Entrega.AtrasoDias), estAtencao)
		}
		verif, estVerif := textoDaVerificacao(l.Verificacao, l.Entrega.Commit)
		nota := moldura.Num("-", estFraco)
		if l.Nota != nil {
			nota = moldura.Num(fmt.Sprintf("%g", l.Nota.Valor), estAcento)
		}
		celulas = append(celulas, []moldura.Celula{marca,
			moldura.Cel(truncar(l.Aluno.Nome, 30), estNormal), situacao, commits, atraso,
			moldura.Cel(verif, estVerif), nota})
	}
	te.cursor = max(0, min(te.cursor, len(linhas)-1))
	return titulo, moldura.Conteudo{
		Linhas: moldura.Tabela(cab, celulas),
		Sel:    1 + te.cursor,
		Info:   fmt.Sprintf("%d de %d", te.cursor+1, len(linhas)),
	}
}

// detalhe é o painel [2]: a entrega do aluno selecionado, que segue o
// cursor sem precisar de enter.
func (te *telaEntregas) detalhe(a *App, largura int) (string, moldura.Conteudo) {
	e, ok := a.exercicioAberto()
	l, temLinha := linhaAtual(te.linhas(a), te.cursor)
	if !ok || !temLinha {
		return "Detalhe", moldura.Conteudo{Sel: -1, Linhas: []string{estFraco.Render(" nada selecionado")}}
	}
	return l.Aluno.Nome, detalheDaEntrega(a, e, l, largura).conteudo()
}
