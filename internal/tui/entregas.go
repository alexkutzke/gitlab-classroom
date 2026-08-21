package tui

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/alexkutzke/gitlab-classroom/internal/acoes"
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
		te.cursor = min(len(linhas)-1, te.cursor+1)
	case "pgup":
		te.cursor = max(0, te.cursor-a.linhasDisponiveis())
	case "pgdown":
		te.cursor = min(len(linhas)-1, te.cursor+a.linhasDisponiveis())
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
		a.avisar("escolha com enter o dono do fork onde %s entregou; esc cancela",
			l.Aluno.Nome)
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

func (te *telaEntregas) atalhos() string {
	if te.vinculando != nil {
		return estAtencao.Render("escolha o dono do fork com enter · esc cancela")
	}
	return "n corrige · c coleta · l clona · v verifica · o editor · w GitLab · V vincula · s ordem · / filtra"
}

func (te *telaEntregas) desenhar(a *App) string {
	e, ok := a.exercicioAberto()
	if !ok {
		return estErro.Render("exercício não encontrado")
	}

	var b strings.Builder
	cabecalho := fmt.Sprintf("%s  prazo %s  peso %g", e.Titulo, e.Prazo.String(), e.Peso)
	if e.TemSuite() {
		cabecalho += estFraco.Render("  suíte: " + e.Verificacao)
	}
	b.WriteString(estTitulo.Render(cabecalho) + "\n")
	if te.modoFiltro {
		b.WriteString(estDestaque.Render("filtro: ") + te.buffer + "_\n")
	} else if te.filtro != "" {
		b.WriteString(estDestaque.Render("filtro: "+te.filtro) + estFraco.Render("  (/ muda, esc limpa)") + "\n")
	} else {
		b.WriteString(estFraco.Render(fmt.Sprintf("ordem por %s", te.ordem)) + "\n")
	}
	b.WriteString("\n")

	linhas := te.linhas(a)
	if len(linhas) == 0 {
		b.WriteString(estFraco.Render("  nenhum aluno com esse filtro") + "\n")
		return b.String()
	}

	altura := a.linhasDisponiveis() - 2
	if te.cursor >= len(linhas) {
		te.cursor = len(linhas) - 1
	}
	te.topo = janela(te.topo, te.cursor, altura, len(linhas))
	fim := min(te.topo+altura, len(linhas))

	for i := te.topo; i < fim; i++ {
		b.WriteString(te.linha(linhas[i], i == te.cursor) + "\n")
	}
	if len(linhas) > altura {
		b.WriteString(rolagem(te.topo, fim, len(linhas)) + "\n")
	}
	return b.String()
}

func (te *telaEntregas) linha(l linhaEntrega, sob bool) string {
	cursor := "  "
	nome := truncar(l.Aluno.Nome, 30)
	if sob {
		cursor = estCursor.Render("> ")
		nome = estCursor.Render(nome)
	}

	marca := " "
	if len(l.Equipe) > 0 {
		marca = estFraco.Render("d")
	}

	situacao := estFraco.Render("sem coleta")
	if l.Entrega.Situacao != "" {
		situacao = corDaSituacao(l.Entrega.Situacao).Render(l.Entrega.Descricao())
	}

	commits := ""
	if l.Entrega.Commits > 0 {
		commits = estFraco.Render(fmt.Sprintf("%d commits", l.Entrega.Commits))
	}

	verificacao := ""
	if v := l.Verificacao; v.Situacao != "" && v.Situacao != turma.SemSuite {
		texto := v.Resumo()
		if v.Desatualizada(l.Entrega.Commit) {
			texto += " !"
		}
		verificacao = corDaVerificacao(v.Situacao).Render(texto)
	}

	nota := estFraco.Render("   -")
	if l.Nota != nil {
		nota = estDestaque.Render(fmt.Sprintf("%4g", l.Nota.Valor))
	}

	return cursor + marca + " " + preencher(nome, 31) + preencher(situacao, 30) +
		preencher(commits, 12) + preencher(verificacao, 20) + nota
}
