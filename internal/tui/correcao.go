package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/alexkutzke/gitlab-classroom/internal/acoes"
	"github.com/alexkutzke/gitlab-classroom/internal/correcao"
	"github.com/alexkutzke/gitlab-classroom/internal/turma"
)

// A correção é a mesma tela do subcomando, embutida como subtela: nenhuma
// regra de nota vive aqui.

// abrirCorrecao monta a sessão de correção do exercício informado.
func (a *App) abrirCorrecao(e turma.Exercicio, filtro acoes.FiltroCorrecao) tea.Cmd {
	itens := acoes.ItensDeCorrecao(a.turma, a.store.Pasta(), e, filtro)
	if len(itens) == 0 {
		a.erro = "nenhum aluno a corrigir em " + e.ID + " com esse filtro"
		return nil
	}
	s, err := correcao.Nova(correcao.Opcoes{
		Exercicio:  e,
		NotaMaxima: a.turma.Config.NotaMaxima,
		Itens:      itens,
		Propagar:   true,
	})
	if err != nil {
		a.erro = err.Error()
		return nil
	}
	s.Dimensionar(a.largura, a.altura-4)
	a.correcao = s
	a.ir(idCorrecao)
	return nil
}

// atualizarCorrecao entrega as teclas à subtela e trata a saída dela.
func (a *App) atualizarCorrecao(msg tea.Msg) (tea.Cmd, bool) {
	if a.correcao == nil {
		return nil, false
	}
	cmd := a.correcao.Atualizar(msg)
	if !a.correcao.Encerrada() {
		return cmd, true
	}

	res := a.correcao.Resultado()
	a.correcao = nil
	a.voltar()

	if !res.Salvar {
		a.avisar("correção fechada sem gravar")
		return cmd, true
	}
	lancadas, apagadas := acoes.AplicarCorrecao(a.turma, a.exercicio, res)
	if !a.gravar() {
		return cmd, true
	}
	a.avisar("%d nota(s) lançada(s), %d apagada(s) em %s", lancadas, apagadas, a.exercicio)
	a.tarefas.registrar("correção de %s: %d nota(s) lançada(s), %d apagada(s)",
		a.exercicio, lancadas, apagadas)
	return cmd, true
}
