package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"

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
	a.tarefa = &tarefa{nome: nome, cancelar: cancelar, canal: canal}
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
		if a.tarefa != nil {
			nome = a.tarefa.nome
			a.tarefa.cancelar()
		}
		a.tarefa = nil

		switch {
		case errors.Is(m.err, context.Canceled):
			a.avisar("%s cancelada, nada foi gravado", nome)
			a.tarefas.registrar("%s: cancelada", nome)
		case m.err != nil:
			a.erro = m.err.Error()
			a.tarefas.registrar("%s: erro: %v", nome, m.err)
		default:
			if m.gravar {
				a.gravar()
			}
			a.recarregarPanorama()
			a.avisar("%s", m.resumo)
			a.tarefas.registrar("%s: %s", nome, m.resumo)
		}
		for _, d := range m.detalhes {
			a.tarefas.registrar("  %s", d)
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
	return estDestaque.Render(texto) + estFraco.Render("   esc cancela")
}
