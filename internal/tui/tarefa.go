package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/alexkutzke/gitlab-classroom/internal/acoes"
)

// As operações de rede levam de segundos a minutos numa turma de trinta. Elas
// rodam em goroutine e conversam com o laço de eventos por um canal: a
// interface continua respondendo, mostra progresso e aceita cancelamento.

type progressoMsg struct{ p acoes.Progresso }

type fimMsg struct {
	// resumo é a linha que vai para a barra de status e para as tarefas.
	resumo string
	// detalhes são as linhas extras do registro, como os erros por aluno.
	detalhes []string
	err      error
	// gravar diz se o estado mudou e precisa ir para o disco.
	gravar bool
}

// tarefa é a operação longa em curso.
type tarefa struct {
	nome     string
	prog     acoes.Progresso
	cancelar context.CancelFunc
	canal    chan tea.Msg
	// registro é a entrada da tela de tarefas que acompanha esta operação.
	registro *registro
}

// emCurso informa se há operação rodando; enquanto houver, as ações ficam
// bloqueadas para não haver duas coletas mexendo na mesma turma.
func (a *App) emCurso() bool { return a.tarefa != nil }

// iniciar dispara a operação em goroutine e devolve o comando que escuta o
// canal.
//
// O trabalho recebe o aviso de progresso e devolve a mensagem de fim, já com
// o resumo pronto: assim a goroutine não toca no estado da aplicação, que é
// de uso exclusivo do laço de eventos.
func (a *App) iniciar(nome string, trabalho func(ctx context.Context, prog acoes.AvisoProgresso) fimMsg) tea.Cmd {
	if a.emCurso() {
		a.erro = "espere a operação em curso terminar"
		return nil
	}

	ctx, cancelar := context.WithCancel(context.Background())
	canal := make(chan tea.Msg, 64)
	a.tarefa = &tarefa{nome: nome, cancelar: cancelar, canal: canal,
		registro: a.tarefas.abrir(nome)}
	a.status = nome + ": começando"

	go func() {
		defer close(canal)
		fim := trabalho(ctx, func(p acoes.Progresso) {
			// Progresso é descartável: se o laço de eventos estiver ocupado,
			// perder um quadro é melhor que travar o trabalho.
			select {
			case canal <- progressoMsg{p}:
			default:
			}
		})
		canal <- fim
	}()

	return escutar(canal)
}

// escutar lê uma mensagem do canal e se reagenda a cada chegada.
func escutar(canal chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		msg, aberto := <-canal
		if !aberto {
			return nil
		}
		return msg
	}
}

// tratarTarefa cuida das mensagens vindas da goroutine.
func (a *App) tratarTarefa(msg tea.Msg) (tea.Cmd, bool) {
	switch m := msg.(type) {
	case progressoMsg:
		// Mensagem atrasada de uma tarefa já encerrada: descarta.
		if a.tarefa == nil {
			return nil, true
		}
		a.tarefa.prog = m.p
		return escutar(a.tarefa.canal), true

	case fimMsg:
		nome := "operação"
		var reg *registro
		if a.tarefa != nil {
			nome, reg = a.tarefa.nome, a.tarefa.registro
			a.tarefa.cancelar()
		}
		a.tarefa = nil
		if reg == nil {
			reg = a.tarefas.abrir(nome)
		}

		switch {
		case errors.Is(m.err, context.Canceled):
			a.avisar("%s cancelada, nada foi gravado", nome)
			a.tarefas.encerrar(reg, regCancelado, "cancelada")
		case m.err != nil:
			a.erro = m.err.Error()
			a.tarefas.encerrar(reg, regErro, "erro: "+m.err.Error())
		default:
			if m.gravar {
				a.gravar()
			}
			a.recarregarPanorama()
			a.avisar("%s", m.resumo)
			a.tarefas.encerrar(reg, regConcluido, m.resumo)
		}
		for _, d := range m.detalhes {
			a.tarefas.acrescentar(reg, d)
		}
		return nil, true

	case erroMsg:
		if m.err != nil {
			a.erro = m.err.Error()
		}
		return nil, true
	}
	return nil, false
}

// cancelarTarefa interrompe o que está rodando.
func (a *App) cancelarTarefa() {
	if !a.emCurso() {
		return
	}
	a.tarefa.cancelar()
	a.avisar("cancelando %s", a.tarefa.nome)
}

// barraDeProgresso desenha o andamento da operação em curso.
func (a *App) barraDeProgresso() string {
	if !a.emCurso() {
		return ""
	}
	p := a.tarefa.prog
	largura := 24
	preenchido := 0
	if p.Total > 0 {
		preenchido = p.Feito * largura / p.Total
	}
	barra := strings.Repeat("#", preenchido) + strings.Repeat(".", largura-preenchido)

	texto := fmt.Sprintf("%s  [%s]  %d/%d", a.tarefa.nome, barra, p.Feito, p.Total)
	if p.Rotulo != "" {
		texto += "  " + truncar(p.Rotulo, 30)
	}
	return estAcento.Render(texto) + estFraco.Render("   esc cancela")
}

// gerundios dão o nome curto da operação na barra de título, como
// "coletando 12/31".
var gerundios = []struct{ prefixo, gerundio string }{
	{"coleta", "coletando"},
	{"clone", "clonando"},
	{"verificação", "verificando"},
	{"sincronização", "sincronizando"},
	{"busca", "buscando membros"},
}

// textoProgresso é o andamento da tarefa em curso, para a barra de título.
func (a *App) textoProgresso() string {
	if !a.emCurso() {
		return ""
	}
	nome := a.tarefa.nome
	for _, g := range gerundios {
		if strings.HasPrefix(nome, g.prefixo) {
			nome = g.gerundio
			break
		}
	}
	p := a.tarefa.prog
	if p.Total == 0 {
		return nome + "..."
	}
	return fmt.Sprintf("%s %d/%d", nome, p.Feito, p.Total)
}

// recenteTarefa é quanto tempo o painel da tarefa continua na tela inicial
// depois de ela terminar, para o resumo não sumir antes de ser lido.
const recenteTarefa = 2 * time.Minute

// tarefaVisivel informa se a tela inicial mostra o painel da tarefa: há uma
// em curso, ou uma terminou há pouco.
func (a *App) tarefaVisivel() bool {
	if a.emCurso() {
		return true
	}
	r := a.tarefas.ultimaOperacao()
	return r != nil && relogio().Sub(r.fim) < recenteTarefa
}
