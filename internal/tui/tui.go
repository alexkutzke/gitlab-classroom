// Package tui é a interface interativa do classroom.
//
// Ela não reimplementa nada: as operações vêm de internal/acoes, as mesmas
// que os subcomandos usam. Aqui ficam só navegação, desenho e o laço de
// eventos. O desenho é o de internal/moldura, que é o mesmo do painel e do
// diario: painéis com borda, foco, detalhe que segue o cursor e cores pelo
// sentido do valor.
package tui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/alexkutzke/gitlab-classroom/internal/acoes"
	"github.com/alexkutzke/gitlab-classroom/internal/correcao"
	gl "github.com/alexkutzke/gitlab-classroom/internal/gitlab"
	"github.com/alexkutzke/gitlab-classroom/internal/moldura"
	"github.com/alexkutzke/gitlab-classroom/internal/store"
	"github.com/alexkutzke/gitlab-classroom/internal/turma"
)

// telaID identifica a tela corrente.
type telaID int

const (
	idPainel telaID = iota
	idExercicio
	idAlunos
	idEquipes
	idTarefas
	idCorrecao
	idExercicios
	numTelas
)

// Os painéis de cada tela, pelo número que leva o foco a eles. A lista é
// sempre o [1] e o detalhe sempre o [2], para a mão aprender uma vez só.
const (
	focoLista   = 0
	focoDetalhe = 1
	// focoPendencias e focoTarefa só existem na tela inicial.
	focoPendencias = 2
	focoTarefa     = 3
)

// AbrirCliente devolve um cliente do GitLab. Fica como função para o token só
// ser exigido quando alguma ação precisar da rede: abrir a interface para
// olhar notas não deve falhar por falta de token.
type AbrirCliente func() (gl.Cliente, error)

// relogio existe para os testes congelarem o tempo.
var relogio = time.Now

// App é o modelo da aplicação inteira.
type App struct {
	store   *store.Store
	turma   *turma.Turma
	abrirGL AbrirCliente
	cliente gl.Cliente

	tela      telaID
	anterior  telaID
	panorama  acoes.Panorama
	resumo    acoes.ResumoTurma
	exercicio string // exercício aberto na tela de entregas

	correcao  *correcao.Sessao
	confirmar *confirmacao

	painel   painel
	entregas telaEntregas
	catalogo telaExercicios
	alunos   telaAlunos
	equipes  telaEquipes
	tarefas  telaTarefas

	// foco e rolagem são por tela: voltar a uma tela devolve o painel que
	// estava em foco e o ponto do detalhe onde se parou.
	foco    [numTelas]int
	rolagem [numTelas]int
	ajuda   bool

	tarefa *tarefa

	largura, altura int
	status          string
	erro            string
	sair            bool
}

// tela é o que cada tela oferece ao laço de eventos e ao desenho.
type tela interface {
	atualizar(a *App, msg tea.KeyMsg) (tea.Cmd, bool)
	// digitando informa se a tela está recebendo texto. Aí toda tecla é
	// dela, inclusive as globais: um filtro precisa do "q" como letra.
	digitando() bool
	teclas(a *App) []moldura.Tecla
}

// telaDupla é a tela de lista à esquerda e detalhe à direita.
type telaDupla interface {
	tela
	painelLista(a *App) (string, moldura.Conteudo)
	detalhe(a *App, largura int) (string, moldura.Conteudo)
}

// comEntrada é a tela que, em algum modo, põe uma pergunta ou o texto
// digitado na barra de teclas.
type comEntrada interface {
	entrada(a *App) string
}

// Executar abre a interface e só volta quando o professor sai.
func Executar(s *store.Store, t *turma.Turma, abrir AbrirCliente) error {
	a := &App{
		store:   s,
		turma:   t,
		abrirGL: abrir,
	}
	a.recarregarPanorama()
	a.entregas.ordem = ordemNome

	_, err := tea.NewProgram(a, tea.WithAltScreen()).Run()
	return err
}

func (a *App) Init() tea.Cmd { return nil }

// recarregarPanorama recalcula o resumo depois de qualquer mudança de estado.
func (a *App) recarregarPanorama() {
	a.panorama = acoes.PanoramaDe(a.turma, turma.Hoje())
	a.resumo = acoes.ResumoDe(a.turma, relogio())
}

// recarregarDoDisco relê os arquivos, para pegar edição manual feita por fora.
func (a *App) recarregarDoDisco() {
	t, err := a.store.Carregar()
	if err != nil {
		a.erro = err.Error()
		return
	}
	a.turma = t
	a.recarregarPanorama()
	a.status = "recarregado do disco"
}

// gravar persiste a turma e reporta a falha na barra de status, em vez de
// derrubar a interface.
func (a *App) gravar() bool {
	if err := a.store.Gravar(a.turma); err != nil {
		a.erro = err.Error()
		return false
	}
	a.recarregarPanorama()
	return true
}

// conectar resolve o cliente do GitLab uma vez por sessão.
func (a *App) conectar() (gl.Cliente, error) {
	if a.cliente != nil {
		return a.cliente, nil
	}
	c, err := a.abrirGL()
	if err != nil {
		return nil, err
	}
	a.cliente = c
	return c, nil
}

// exercicioAberto devolve o exercício da tela de entregas.
func (a *App) exercicioAberto() (turma.Exercicio, bool) {
	e, ok := a.turma.Exercicio(a.exercicio)
	if !ok {
		return turma.Exercicio{}, false
	}
	return *e, true
}

// ir troca de tela guardando de onde veio, para o esc voltar.
func (a *App) ir(t telaID) {
	if t == a.tela {
		return
	}
	a.anterior = a.tela
	a.tela = t
	a.erro = ""
}

func (a *App) voltar() {
	if a.tela == idPainel {
		return
	}
	destino := a.anterior
	if destino == a.tela {
		destino = idPainel
	}
	a.tela, a.anterior = destino, idPainel
	a.erro = ""
}

// tamanho devolve o terminal, ou o padrão antes de ele se informar.
func (a *App) tamanho() (int, int) {
	if a.largura <= 0 || a.altura <= 0 {
		return moldura.LarguraPadrao, moldura.AlturaPadrao
	}
	return a.largura, a.altura
}

// linhasDisponiveis é quanto cabe numa lista: a tela menos as duas barras,
// a borda e o cabeçalho da tabela. Serve de página para pgup e pgdown.
func (a *App) linhasDisponiveis() int {
	_, h := a.tamanho()
	return max(1, h-5)
}

// telaCorrente devolve a tela que recebe as teclas.
func (a *App) telaCorrente() tela {
	switch a.tela {
	case idExercicio:
		return &a.entregas
	case idAlunos:
		return &a.alunos
	case idEquipes:
		return &a.equipes
	case idTarefas:
		return &a.tarefas
	case idExercicios:
		return &a.catalogo
	}
	return &a.painel
}

// numPaineis conta os painéis que recebem foco na tela corrente.
func (a *App) numPaineis() int {
	if a.tela == idPainel {
		if a.tarefaVisivel() {
			return 4
		}
		return 3
	}
	return 2
}

// focoAtual devolve o painel em foco, corrigido quando o painel some, como
// o da tarefa que terminou há tempo.
func (a *App) focoAtual() int {
	return min(a.foco[a.tela], a.numPaineis()-1)
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m, ok := msg.(tea.WindowSizeMsg); ok {
		a.largura, a.altura = m.Width, m.Height
		if a.correcao != nil {
			a.correcao.Dimensionar(m.Width, m.Height)
		}
		return a, nil
	}
	if cmd, tratada := a.tratarTarefa(msg); tratada {
		return a, cmd
	}
	if a.tela == idCorrecao {
		if cmd, tratada := a.atualizarCorrecao(msg); tratada {
			return a, cmd
		}
	}

	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return a, nil
	}
	// Uma confirmação pendente captura tudo até ser respondida.
	if a.confirmar != nil {
		return a, a.responderConfirmacao(k)
	}
	// Enquanto uma operação de rede roda, só cancelar e sair valem: duas
	// coletas ao mesmo tempo mexeriam na mesma turma.
	if a.emCurso() {
		switch k.String() {
		case "esc":
			a.cancelarTarefa()
		case "ctrl+c":
			a.cancelarTarefa()
			a.sair = true
			return a, tea.Quit
		}
		return a, nil
	}
	// A tecla que fecha a ajuda não age por baixo dela.
	if a.ajuda {
		a.ajuda = false
		return a, nil
	}

	t := a.telaCorrente()
	if t.digitando() {
		cmd, _ := t.atualizar(a, k)
		return a, cmd
	}
	// A mensagem de estado dura até a próxima tecla.
	a.status, a.erro = "", ""
	a.foco[a.tela] = a.focoAtual()
	if a.teclaFoco(k) {
		return a, nil
	}
	if a.focoAtual() == focoDetalhe && a.rolarDetalhe(k) {
		return a, nil
	}
	cmd, tratada := t.atualizar(a, k)
	// Mover na lista troca o detalhe, que volta ao topo.
	if a.focoAtual() == focoLista {
		a.rolagem[a.tela] = 0
	}
	if tratada {
		return a, cmd
	}
	return a, a.teclaGlobal(k)
}

// teclaFoco trata tab, shift+tab, os números dos painéis e o esc que devolve
// o foco à lista.
func (a *App) teclaFoco(k tea.KeyMsg) bool {
	n := a.numPaineis()
	foco := a.focoAtual()
	switch s := k.String(); s {
	case "tab":
		a.foco[a.tela] = (foco + 1) % n
	case "shift+tab":
		a.foco[a.tela] = (foco - 1 + n) % n
	case "esc":
		if foco == focoLista {
			return false
		}
		a.foco[a.tela] = focoLista
	default:
		if len(s) != 1 || s[0] < '1' || int(s[0]-'1') >= n {
			return false
		}
		a.foco[a.tela] = int(s[0] - '1')
	}
	return true
}

// rolarDetalhe move o detalhe em foco; devolve false para as teclas que não
// são de rolagem, que seguem para a tela.
func (a *App) rolarDetalhe(k tea.KeyMsg) bool {
	w, h := a.retanguloDetalhe()
	_, c := a.conteudoDetalhe(w)
	visiveis := max(1, h-2)
	limite := max(0, len(c.Linhas)-visiveis)
	r := &a.rolagem[a.tela]
	switch k.String() {
	case "down", "j":
		*r++
	case "up", "k":
		*r--
	case "pgdown":
		*r += visiveis
	case "pgup":
		*r -= visiveis
	case "home", "g":
		*r = 0
	case "end", "G":
		*r = limite
	default:
		return false
	}
	*r = max(0, min(*r, limite))
	return true
}

// teclaGlobal trata o que vale em qualquer tela.
func (a *App) teclaGlobal(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "ctrl+c":
		a.sair = true
		return tea.Quit
	case "q":
		if a.tela == idPainel {
			a.sair = true
			return tea.Quit
		}
		a.voltar()
	case "esc":
		a.voltar()
	case "?":
		a.ajuda = true
	case "p":
		a.ir(idPainel)
	case "a":
		a.ir(idAlunos)
	case "e":
		a.ir(idEquipes)
	case "t":
		a.ir(idTarefas)
	case "x":
		a.ir(idExercicios)
	case "r":
		a.recarregarDoDisco()
	case "S":
		return a.sincronizar()
	case "C":
		return a.coletar(nil)
	}
	return nil
}

// --- desenho ---

func (a *App) View() string {
	if a.sair {
		return ""
	}
	w, h := a.tamanho()
	if w < moldura.LarguraMinima || h < moldura.AlturaMinima {
		return moldura.Pequena(w, h)
	}
	if a.ajuda {
		return moldura.Ajuda(w, h, a.gruposAjuda(), "qualquer tecla fecha")
	}
	if a.tela == idCorrecao && a.correcao != nil {
		a.correcao.Dimensionar(w, h)
		a.correcao.Anunciar(a.textoProgresso())
		return a.correcao.View()
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		a.barraTitulo(w), a.corpo(w, h-2), a.barraTeclas(w))
}

// corpo desenha os painéis da tela corrente no espaço entre as barras.
func (a *App) corpo(w, h int) string {
	if a.tela == idPainel {
		return a.painel.corpo(a, w, h)
	}
	d, ok := a.telaCorrente().(telaDupla)
	if !ok {
		return moldura.Encaixar("", w, h)
	}
	titulo, lista := d.painelLista(a)
	foco := a.focoAtual()
	if w < moldura.LarguraDuasColunas {
		if foco == focoDetalhe {
			return a.caixaDetalhe(w, h)
		}
		return moldura.Caixa("1", titulo, lista, w, h, true)
	}
	esq := moldura.LarguraLista(moldura.LarguraLinhas(lista.Linhas), w)
	return lipgloss.JoinHorizontal(lipgloss.Top,
		moldura.Caixa("1", titulo, lista, esq, h, foco == focoLista),
		a.caixaDetalhe(w-esq, h))
}

// retanguloDetalhe é o tamanho do painel de detalhe na disposição atual,
// que a rolagem por página precisa saber.
func (a *App) retanguloDetalhe() (int, int) {
	w, h := a.tamanho()
	h -= 2
	if a.tela == idPainel {
		d := a.painel.dispor(a, w, h)
		return d.detalhe.w, d.detalhe.h
	}
	if w < moldura.LarguraDuasColunas {
		return w, h
	}
	if d, ok := a.telaCorrente().(telaDupla); ok {
		_, lista := d.painelLista(a)
		return w - moldura.LarguraLista(moldura.LarguraLinhas(lista.Linhas), w), h
	}
	return w, h
}

// conteudoDetalhe devolve título e linhas do detalhe da tela corrente.
func (a *App) conteudoDetalhe(w int) (string, moldura.Conteudo) {
	if a.tela == idPainel {
		return a.painel.detalhe(a, w)
	}
	if d, ok := a.telaCorrente().(telaDupla); ok {
		return d.detalhe(a, w)
	}
	return "Detalhe", moldura.Conteudo{Sel: -1}
}

func (a *App) caixaDetalhe(w, h int) string {
	titulo, c := a.conteudoDetalhe(w)
	c.Topo, c.Livre = a.rolagem[a.tela], true
	return moldura.Caixa("2", titulo, c, w, h, a.focoAtual() == focoDetalhe)
}

// abas são as telas que a barra de título mostra, com a tecla na frente.
var abas = []struct {
	id    telaID
	tecla string
	nome  string
}{
	{idPainel, "p", "Painel"},
	{idExercicios, "x", "Exercícios"},
	{idAlunos, "a", "Alunos"},
	{idEquipes, "e", "Equipes"},
	{idTarefas, "t", "Tarefas"},
}

// barraTitulo leva as abas, o contexto da turma e, à direita, o progresso da
// tarefa em curso, que fica à vista em qualquer tela.
func (a *App) barraTitulo(w int) string {
	var esq []moldura.Trecho
	for _, ab := range abas {
		est := estFraco
		if ab.id == a.tela {
			est = estAcento.Bold(true)
		}
		esq = append(esq, moldura.Trecho{Texto: " " + ab.tecla + " " + ab.nome + " ", Est: est})
	}
	esq = append(esq, moldura.Trecho{Texto: "  " + a.contexto(), Est: estNormal})
	var dir []moldura.Trecho
	if p := a.textoProgresso(); p != "" {
		dir = append(dir, moldura.Trecho{Texto: p + " ", Est: estAtencao})
	}
	return moldura.BarraTitulo(w, "classroom", esq, dir)
}

// contexto identifica a turma e, nas telas de um exercício, o exercício.
func (a *App) contexto() string {
	c := a.turma.Config
	texto := c.Codigo
	if c.Turma != "" {
		texto += " " + c.Turma
	}
	if c.Semestre != "" {
		texto += " · " + c.Semestre
	}
	if a.tela == idExercicio {
		texto += " · exercício " + a.exercicio
	}
	return texto
}

// barraTeclas é a última linha: a pergunta pendente, o progresso, a
// mensagem de estado ou, sem nada disso, as teclas da tela.
func (a *App) barraTeclas(w int) string {
	switch {
	case a.confirmar != nil:
		return moldura.Preencher(" "+estAtencao.Render(moldura.Limpo(a.confirmar.pergunta))+"   "+
			estTecla.Render("s")+" "+estFraco.Render("confirma")+"   "+
			estTecla.Render("n")+" "+estFraco.Render("cancela"), w)
	case a.emCurso():
		return moldura.Preencher(" "+a.barraDeProgresso(), w)
	case a.erro != "":
		return moldura.BarraMensagem(w, "erro: "+a.erro, estErro)
	case a.status != "":
		return moldura.BarraMensagem(w, a.status, estAtencao)
	}
	t := a.telaCorrente()
	if e, ok := t.(comEntrada); ok {
		if texto := e.entrada(a); texto != "" {
			return moldura.Preencher(" "+texto, w)
		}
	}
	return moldura.BarraTeclas(w, t.teclas(a))
}

// teclasComuns fecham a barra de teclas de toda tela.
func (a *App) teclasComuns() []moldura.Tecla {
	sair := "volta"
	if a.tela == idPainel {
		sair = "sai"
	}
	return []moldura.Tecla{{K: "?", Rotulo: "ajuda"}, {K: "q", Rotulo: sair}}
}

// teclasDoDetalhe é o que vale com o foco no painel de detalhe.
func teclasDoDetalhe() []moldura.Tecla {
	return []moldura.Tecla{{K: "tab", Rotulo: "painel"}, {K: "j/k", Rotulo: "rola"},
		{K: "pgup/pgdn", Rotulo: "página"}, {K: "esc", Rotulo: "lista"}}
}

// avisar coloca uma mensagem na barra de status.
func (a *App) avisar(formato string, args ...any) {
	a.status = fmt.Sprintf(formato, args...)
}

// gruposAjuda são as teclas de todas as telas, para a caixa de ajuda. O
// grupo da tela corrente vem primeiro: em terminal pequeno, os últimos são
// os que ficam de fora.
func (a *App) gruposAjuda() []moldura.GrupoAjuda {
	t := func(k, r string) moldura.Tecla { return moldura.Tecla{K: k, Rotulo: r} }
	geral := []moldura.GrupoAjuda{
		{Titulo: "Navegação", Teclas: []moldura.Tecla{
			t("tab S-tab", "painel seguinte, anterior"),
			t("1 2 3 4", "foco direto no painel"),
			t("j k", "move na lista, rola o detalhe"),
			t("g G", "primeiro e último"),
			t("pgup pgdn", "página"),
			t("enter", "abre o item"),
			t("esc", "volta à lista, ou à tela anterior"),
			t("r", "relê os arquivos do disco"),
			t("q", "volta, e sai no painel"),
		}},
		{Titulo: "Telas", Teclas: []moldura.Tecla{
			t("p x a", "painel, exercícios, alunos"),
			t("e t", "equipes, tarefas"),
			t("?", "esta ajuda"),
		}},
		{Titulo: "Ações", Teclas: []moldura.Tecla{
			t("C", "coleta todos os exercícios"),
			t("S", "sincroniza cadastro e contas"),
			t("c", "coleta o exercício selecionado"),
			t("l v", "clona e verifica o exercício"),
			t("esc", "cancela a operação em curso"),
		}},
	}
	porTela := map[telaID]moldura.GrupoAjuda{
		idPainel: {Titulo: "Painel", Teclas: []moldura.Tecla{
			t("enter", "entregas; na pendência, a origem"),
			t("n", "corrige o exercício"),
			t("E", "exporta relatório e planilha"),
		}},
		idExercicio: {Titulo: "Entregas", Teclas: []moldura.Tecla{
			t("n N", "corrige, e só quem não tem nota"),
			t("V X", "vincula e desvincula a dupla"),
			t("o w", "abre no $EDITOR, no GitLab"),
			t("s", "ordem: nome, situação, nota"),
			t("/", "filtra por nome ou GRR"),
		}},
		idExercicios: {Titulo: "Exercícios", Teclas: []moldura.Tecla{
			t("n", "cadastra um exercício"),
			t("T D P", "edita título, prazo, peso"),
			t("V I K", "edita suíte, imagem, categoria"),
			t("A z", "arquiva, mostra arquivados"),
		}},
		idAlunos: {Titulo: "Alunos", Teclas: []moldura.Tecla{
			t("P", "só pendência de cadastro"),
			t("/", "filtra por nome ou GRR"),
		}},
		idEquipes: {Titulo: "Equipes", Teclas: []moldura.Tecla{
			t("d", "desfaz o vínculo"),
			t("u", "procura membro sem cadastro"),
		}},
		idTarefas: {Titulo: "Tarefas", Teclas: []moldura.Tecla{
			t("c", "limpa o histórico"),
		}},
	}
	ordem := []telaID{idPainel, idExercicio, idExercicios, idAlunos, idEquipes, idTarefas}

	r := []moldura.GrupoAjuda{}
	if g, ok := porTela[a.tela]; ok {
		r = append(r, g)
	}
	r = append(r, geral...)
	for _, id := range ordem {
		if id != a.tela {
			r = append(r, porTela[id])
		}
	}
	return r
}
