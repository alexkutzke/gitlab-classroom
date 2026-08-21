package correcao

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/alexkutzke/gitlab-classroom/internal/turma"
)

func modeloExemplo() *modelo {
	m := &modelo{
		exercicio: turma.Exercicio{ID: "html", Titulo: "HTML",
			Prazo: turma.NovaData(2026, time.September, 5)},
		notaMaxima: 100,
		altura:     10,
		largura:    100,
		itens: []Item{
			{Aluno: turma.Aluno{GRR: "GRR1", Nome: "Ana Souza"},
				Entrega: turma.Entrega{Situacao: turma.Entregue, Commits: 3}},
			{Aluno: turma.Aluno{GRR: "GRR2", Nome: "Bruno Lima"},
				Entrega: turma.Entrega{Situacao: turma.SemFork}},
		},
	}
	m.filtrar()
	return m
}

func teclar(m *modelo, teclas ...string) {
	for _, t := range teclas {
		var msg tea.KeyMsg
		switch t {
		case "enter":
			msg = tea.KeyMsg{Type: tea.KeyEnter}
		case "esc":
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		case "backspace":
			msg = tea.KeyMsg{Type: tea.KeyBackspace}
		case "down":
			msg = tea.KeyMsg{Type: tea.KeyDown}
		default:
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(t)}
		}
		m.Update(msg)
	}
}

func TestDigitoComecaALancarNota(t *testing.T) {
	m := modeloExemplo()
	teclar(m, "9", "0", "enter")

	if !m.itens[0].temNota || m.itens[0].nota != 90 {
		t.Errorf("nota de Ana = %v (lançada=%t), queria 90", m.itens[0].nota, m.itens[0].temNota)
	}
	if !m.itens[0].alterado {
		t.Error("a nota lançada deveria ficar marcada como alterada")
	}
	if m.cursor != 1 {
		t.Errorf("cursor = %d, queria avançar para o próximo aluno", m.cursor)
	}
}

func TestNotaForaDaEscalaEhRecusada(t *testing.T) {
	m := modeloExemplo()
	teclar(m, "n", "1", "5", "0", "enter")

	if m.itens[0].temNota {
		t.Error("nota acima do máximo não podia ser aceita")
	}
	if m.aviso == "" {
		t.Error("o aviso sobre a escala deveria aparecer")
	}
}

func TestRepetirAplicaAUltimaNota(t *testing.T) {
	m := modeloExemplo()
	teclar(m, "n", "7", "0", "enter") // Ana; o cursor passa para Bruno
	teclar(m, "r")

	if !m.itens[1].temNota || m.itens[1].nota != 70 {
		t.Errorf("nota de Bruno = %v (lançada=%t), queria 70 pela repetição",
			m.itens[1].nota, m.itens[1].temNota)
	}
}

func TestRepetirSemNotaAnteriorAvisa(t *testing.T) {
	m := modeloExemplo()
	teclar(m, "r")

	if m.itens[0].temNota {
		t.Error("não havia nota para repetir")
	}
	if m.aviso == "" {
		t.Error("deveria avisar que nada foi lançado ainda")
	}
}

func TestApagarNotaCarregada(t *testing.T) {
	m := modeloExemplo()
	m.itens[0].Preencher(turma.Nota{Valor: 50, Comentario: "refazer"})
	teclar(m, "x")

	if m.itens[0].temNota {
		t.Error("a nota deveria ter sido apagada")
	}
	if !m.itens[0].removido {
		t.Error("a remoção precisa ficar marcada para o comando apagar do arquivo")
	}
}

func TestComentarioSemNotaAvisa(t *testing.T) {
	m := modeloExemplo()
	teclar(m, "c", "b", "o", "m", "enter")

	if m.itens[0].comentario != "bom" {
		t.Errorf("comentário = %q, queria bom", m.itens[0].comentario)
	}
	if m.aviso == "" {
		t.Error("comentário sem nota deveria avisar que nada será gravado")
	}
}

func TestFiltroReduzALista(t *testing.T) {
	m := modeloExemplo()
	teclar(m, "/", "b", "r", "u", "enter")

	if len(m.visivel) != 1 {
		t.Fatalf("visíveis = %d, queria 1", len(m.visivel))
	}
	if m.itens[m.visivel[0]].Aluno.Nome != "Bruno Lima" {
		t.Errorf("filtro trouxe %s", m.itens[m.visivel[0]].Aluno.Nome)
	}
}

func TestSairSemGravar(t *testing.T) {
	m := modeloExemplo()
	teclar(m, "n", "8", "0", "enter", "q")

	if m.salvar {
		t.Error("q deveria sair sem gravar")
	}
}

func TestViewNaoQuebraComListaVazia(t *testing.T) {
	m := modeloExemplo()
	m.filtro = "ninguem"
	m.filtrar()
	if m.View() == "" {
		t.Error("a tela vazia ainda precisa desenhar o cabeçalho")
	}
}

// --- entrega em dupla ---

func modeloComDupla(propagar bool) *modelo {
	m := modeloExemplo()
	m.propagar = propagar
	fork := "ds122-2026-2-n-grr1/ds122-html-assignment"
	m.itens[0].Entrega = turma.Entrega{Situacao: turma.Entregue, Commits: 3, Projeto: fork}
	m.itens[1].Entrega = turma.Entrega{Situacao: turma.Entregue, Commits: 3, Projeto: fork}
	m.itens[0].Equipe = []string{"Bruno"}
	m.itens[1].Equipe = []string{"Ana"}
	return m
}

func TestNotaDaDuplaVaiParaOsDoisIntegrantes(t *testing.T) {
	m := modeloComDupla(true)
	teclar(m, "n", "8", "5", "enter")

	for i, nome := range []string{"Ana", "Bruno"} {
		if !m.itens[i].temNota || m.itens[i].nota != 85 {
			t.Errorf("%s ficou com %v (lançada=%t), queria 85",
				nome, m.itens[i].nota, m.itens[i].temNota)
		}
		if !m.itens[i].alterado {
			t.Errorf("%s deveria entrar na gravação", nome)
		}
	}
	if m.aviso == "" {
		t.Error("a propagação precisa ficar visível na tela")
	}
}

func TestPropagacaoDesligadaDeixaAOutroSemNota(t *testing.T) {
	m := modeloComDupla(false)
	teclar(m, "n", "8", "5", "enter")

	if m.itens[1].temNota {
		t.Error("com a propagação desligada, só o aluno sob o cursor recebe a nota")
	}
}

func TestTeclaDAlternaAPropagacao(t *testing.T) {
	m := modeloComDupla(true)
	teclar(m, "D")
	if m.propagar {
		t.Error("D deveria desligar a propagação")
	}
	teclar(m, "D")
	if !m.propagar {
		t.Error("D deveria ligar de volta")
	}
}

func TestApagarNotaDaDuplaApagaDosDois(t *testing.T) {
	m := modeloComDupla(true)
	m.itens[0].Preencher(turma.Nota{Valor: 70})
	m.itens[1].Preencher(turma.Nota{Valor: 70})

	teclar(m, "x")

	if m.itens[1].temNota || !m.itens[1].removido {
		t.Errorf("a remoção deveria alcançar o colega: %+v", m.itens[1])
	}
}

func TestAlunoSemEquipeNaoContaminaOsDemais(t *testing.T) {
	m := modeloExemplo() // sem equipe e sem projeto em comum
	teclar(m, "n", "9", "0", "enter")

	if m.itens[1].temNota {
		t.Error("nota de um aluno individual não pode ir para outro")
	}
}
