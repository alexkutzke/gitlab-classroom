// Package tui é a interface interativa do classroom.
//
// Ela não reimplementa nada: as operações vêm de internal/acoes, as mesmas
// que os subcomandos usam. Aqui ficam só navegação, desenho e o laço de
// eventos.
package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/alexkutzke/gitlab-classroom/internal/acoes"
	"github.com/alexkutzke/gitlab-classroom/internal/correcao"
	gl "github.com/alexkutzke/gitlab-classroom/internal/gitlab"
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
	idAjuda
	idCorrecao
	idExercicios
)

// AbrirCliente devolve um cliente do GitLab. Fica como função para o token só
// ser exigido quando alguma ação precisar da rede: abrir a interface para
// olhar notas não deve falhar por falta de token.
type AbrirCliente func() (gl.Cliente, error)

// App é o modelo da aplicação inteira.
type App struct {
	store   *store.Store
	turma   *turma.Turma
	abrirGL AbrirCliente
	cliente gl.Cliente

	tela      telaID
	anterior  telaID
	panorama  acoes.Panorama
	exercicio string // exercício aberto na tela de entregas

	correcao  *correcao.Sessao
	confirmar *confirmacao

	painel    painel
	entregas  telaEntregas
	catalogo  telaExercicios
	alunos    telaAlunos
	equipes   telaEquipes
	tarefas   telaTarefas
	ajudaTela ajuda

	tarefa *tarefa

	largura, altura int
	status          string
	erro            string
	sair            bool
}

// Executar abre a interface e só volta quando o professor sai.
func Executar(s *store.Store, t *turma.Turma, abrir AbrirCliente) error {
	a := &App{
		store:   s,
		turma:   t,
		abrirGL: abrir,
		largura: 100,
		altura:  30,
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

// linhasDisponiveis é quanto sobra para o corpo da tela depois de cabeçalho e
// rodapé.
func (a *App) linhasDisponiveis() int {
	n := a.altura - 7
	if n < 3 {
		return 3
	}
	return n
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if cmd, tratada := a.tratarTarefa(msg); tratada {
		return a, cmd
	}
	if a.tela == idCorrecao {
		if cmd, tratada := a.atualizarCorrecao(msg); tratada {
			return a, cmd
		}
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.largura, a.altura = msg.Width, msg.Height
		return a, nil

	case tea.KeyMsg:
		// Uma confirmação pendente captura tudo até ser respondida.
		if a.confirmar != nil {
			return a, a.responderConfirmacao(msg)
		}
		// Enquanto uma operação de rede roda, só cancelar e sair valem: duas
		// coletas ao mesmo tempo mexeriam na mesma turma.
		if a.emCurso() {
			switch msg.String() {
			case "esc":
				a.cancelarTarefa()
			case "ctrl+c":
				a.cancelarTarefa()
				a.sair = true
				return a, tea.Quit
			}
			return a, nil
		}
		// A tela corrente vê a tecla primeiro: um filtro sendo digitado
		// precisa receber o "q" como letra, e não como pedido de saída.
		if cmd, tratada := a.telaAtual(msg); tratada {
			return a, cmd
		}
		return a, a.teclaGlobal(msg)
	}
	return a, nil
}

// telaAtual entrega a tecla à tela corrente.
func (a *App) telaAtual(msg tea.KeyMsg) (tea.Cmd, bool) {
	switch a.tela {
	case idPainel:
		return a.painel.atualizar(a, msg)
	case idExercicio:
		return a.entregas.atualizar(a, msg)
	case idAlunos:
		return a.alunos.atualizar(a, msg)
	case idEquipes:
		return a.equipes.atualizar(a, msg)
	case idTarefas:
		return a.tarefas.atualizar(a, msg)
	case idAjuda:
		return a.ajudaTela.atualizar(a, msg)
	case idExercicios:
		return a.catalogo.atualizar(a, msg)
	}
	return nil, false
}

// teclaGlobal trata o que vale em qualquer tela.
func (a *App) teclaGlobal(msg tea.KeyMsg) tea.Cmd {
	a.status, a.erro = "", ""
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
		a.ir(idAjuda)
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

func (a *App) View() string {
	if a.sair {
		return ""
	}
	var b strings.Builder
	b.WriteString(a.cabecalho())
	b.WriteString("\n\n")
	b.WriteString(a.corpo())
	b.WriteString("\n")
	b.WriteString(a.rodape())
	return b.String()
}

func (a *App) cabecalho() string {
	c := a.turma.Config
	esquerda := estTitulo.Render(c.Descricao())
	if c.Disciplina != "" {
		esquerda += estFraco.Render("  "+c.Disciplina) + estFraco.Render("  "+c.Semestre)
	}
	direita := estFraco.Render(a.nomeDaTela())
	return alinhar(esquerda, direita, a.largura)
}

func (a *App) nomeDaTela() string {
	switch a.tela {
	case idExercicio:
		return "exercício " + a.exercicio
	case idAlunos:
		return "alunos"
	case idEquipes:
		return "equipes"
	case idTarefas:
		return "tarefas"
	case idAjuda:
		return "ajuda"
	case idCorrecao:
		return "correção de " + a.exercicio
	case idExercicios:
		return "exercícios"
	}
	return "painel"
}

func (a *App) corpo() string {
	switch a.tela {
	case idExercicio:
		return a.entregas.desenhar(a)
	case idAlunos:
		return a.alunos.desenhar(a)
	case idEquipes:
		return a.equipes.desenhar(a)
	case idTarefas:
		return a.tarefas.desenhar(a)
	case idAjuda:
		return a.ajudaTela.desenhar(a)
	case idExercicios:
		return a.catalogo.desenhar(a)
	case idCorrecao:
		if a.correcao != nil {
			return a.correcao.View()
		}
	}
	return a.painel.desenhar(a)
}

func (a *App) rodape() string {
	if a.confirmar != nil {
		return estAtencao.Render(a.confirmar.pergunta + estFraco.Render("   s confirma · n cancela"))
	}
	if a.tela == idCorrecao {
		return "" // a subtela desenha o próprio rodapé
	}
	if a.emCurso() {
		return a.barraDeProgresso()
	}
	if a.erro != "" {
		return estErro.Render("erro: " + truncar(a.erro, a.largura-8))
	}
	if a.status != "" {
		return estAviso.Render(truncar(a.status, a.largura-2))
	}
	return estFraco.Render(a.atalhosDaTela())
}

func (a *App) atalhosDaTela() string {
	comuns := "p painel · x exercícios · a alunos · e equipes · t tarefas · ? ajuda · q volta"
	switch a.tela {
	case idPainel:
		return "enter abre o exercício · " + comuns
	case idExercicio:
		return a.entregas.atalhos() + " · " + comuns
	case idAlunos:
		return a.alunos.atalhos() + " · " + comuns
	case idEquipes:
		return a.equipes.atalhos() + " · " + comuns
	case idExercicios:
		return a.catalogo.atalhos() + " · " + comuns
	}
	return comuns
}

// avisar coloca uma mensagem na barra de status.
func (a *App) avisar(formato string, args ...any) {
	a.status = fmt.Sprintf(formato, args...)
}
