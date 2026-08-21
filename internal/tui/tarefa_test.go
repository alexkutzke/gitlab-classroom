package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/alexkutzke/gitlab-classroom/internal/acoes"
	gl "github.com/alexkutzke/gitlab-classroom/internal/gitlab"
	"github.com/alexkutzke/gitlab-classroom/internal/turma"
)

// clienteFalso responde o suficiente para a coleta rodar sem rede.
type clienteFalso struct {
	grupos   map[string]gl.Grupo
	projetos map[string][]gl.Projeto
	commits  map[string][]gl.Commit
}

func (c *clienteFalso) UsuarioExiste(login string) (bool, error) { return true, nil }

func (c *clienteFalso) Grupo(caminho string) (*gl.Grupo, error) {
	g, ok := c.grupos[caminho]
	if !ok {
		return nil, nil
	}
	return &g, nil
}

func (c *clienteFalso) GruposDoProfessor() ([]gl.Grupo, error) {
	var out []gl.Grupo
	for _, g := range c.grupos {
		out = append(out, g)
	}
	return out, nil
}

func (c *clienteFalso) ProjetosDoGrupo(grupo string) ([]gl.Projeto, error) {
	return c.projetos[grupo], nil
}

func (c *clienteFalso) Commits(projeto, ramo string, todos bool) ([]gl.Commit, error) {
	return c.commits[projeto], nil
}

func (c *clienteFalso) Forks(modelo string) ([]gl.Projeto, error)   { return nil, nil }
func (c *clienteFalso) Membros(projeto string) ([]gl.Membro, error) { return nil, nil }

func clienteComEntrega() *clienteFalso {
	grupo := "ds122-2026-2-n-grr20259001"
	fork := grupo + "/ds122-prepare-assignment"
	return &clienteFalso{
		grupos: map[string]gl.Grupo{grupo: {ID: 1, Caminho: grupo, Membro: true}},
		projetos: map[string][]gl.Projeto{
			grupo: {{ID: 10, Caminho: "ds122-prepare-assignment", Completo: fork, RamoPadrao: "main"}},
		},
		commits: map[string][]gl.Commit{
			fork: {{SHA: "aluno1", Data: time.Date(2026, 8, 10, 9, 0, 0, 0, time.Local)}},
		},
	}
}

// bombear executa os comandos devolvidos pelo modelo, devolvendo as mensagens
// ao Update, até a fila secar. É o laço de eventos do Bubble Tea, à mão.
func bombear(t *testing.T, a *App, cmd tea.Cmd) {
	t.Helper()
	limite := 500
	for cmd != nil && limite > 0 {
		limite--
		msg := cmd()
		if msg == nil {
			return
		}
		_, cmd = a.Update(msg)
	}
}

// teclarCmd digita uma tecla e devolve o comando que o modelo pediu.
func teclarCmd(a *App, tecla string) tea.Cmd {
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tecla)}
	if tecla == "esc" {
		msg = tea.KeyMsg{Type: tea.KeyEsc}
	}
	_, cmd := a.Update(msg)
	return cmd
}

func appComCliente(t *testing.T, c *clienteFalso) *App {
	t.Helper()
	a := appExemplo(t)
	a.abrirGL = func() (gl.Cliente, error) { return c, nil }
	return a
}

func TestColetaPeloPainelGravaEResume(t *testing.T) {
	a := appComCliente(t, clienteComEntrega())

	bombear(t, a, teclarCmd(a, "c"))

	if a.emCurso() {
		t.Fatal("a tarefa deveria ter terminado")
	}
	if a.erro != "" {
		t.Fatalf("erro inesperado: %s", a.erro)
	}
	if !strings.Contains(a.status, "entregue") {
		t.Errorf("status = %q, queria o resumo da coleta", a.status)
	}

	// Ana tem fork com commit no prazo; Bruno não tem grupo no cliente falso.
	if e, ok := a.turma.Entrega("prepare", "GRR20259001"); !ok || e.Situacao != turma.Entregue {
		t.Errorf("entrega de Ana = %+v", e)
	}
	if e, ok := a.turma.Entrega("prepare", "GRR20259002"); !ok || e.Situacao == turma.Entregue {
		t.Errorf("entrega de Bruno = %+v, não deveria estar entregue", e)
	}

	// O que foi apurado precisa ter ido para o disco.
	relida, err := a.store.Carregar()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := relida.Entrega("prepare", "GRR20259001"); !ok {
		t.Error("a coleta não foi gravada")
	}
	if len(a.tarefas.linhas) == 0 {
		t.Error("a operação deveria ficar registrada na tela de tarefas")
	}
}

func TestDuranteATarefaAsTeclasDeNavegacaoNaoValem(t *testing.T) {
	a := appComCliente(t, clienteComEntrega())
	a.tarefa = &tarefa{nome: "coleta", cancelar: func() {}, canal: make(chan tea.Msg, 1)}

	telaAntes := a.tela
	teclarCmd(a, "a")

	if a.tela != telaAntes {
		t.Error("trocar de tela no meio de uma operação deveria ser ignorado")
	}
}

func TestEscCancelaATarefa(t *testing.T) {
	cancelado := false
	a := appComCliente(t, clienteComEntrega())
	a.tarefa = &tarefa{
		nome:     "coleta",
		cancelar: func() { cancelado = true },
		canal:    make(chan tea.Msg, 1),
	}

	teclarCmd(a, "esc")

	if !cancelado {
		t.Error("esc deveria cancelar o contexto da tarefa")
	}
}

func TestTarefaCanceladaNaoGrava(t *testing.T) {
	a := appComCliente(t, clienteComEntrega())
	a.tarefa = &tarefa{nome: "coleta", cancelar: func() {}, canal: make(chan tea.Msg, 1)}

	a.turma.RegistrarNota(turma.Nota{Exercicio: "prepare", GRR: "GRR20259002", Valor: 10})
	a.Update(fimMsg{err: context.Canceled})

	relida, err := a.store.Carregar()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := relida.Nota("prepare", "GRR20259002"); ok {
		t.Error("cancelamento não pode gravar o estado parcial")
	}
	if !strings.Contains(a.status, "cancelada") {
		t.Errorf("status = %q, queria dizer que foi cancelada", a.status)
	}
}

func TestVerificarSemSuiteAvisaSemAbrirTarefa(t *testing.T) {
	a := appComCliente(t, clienteComEntrega())
	teclar(a, "enter") // abre o prepare, que não tem suíte
	teclarCmd(a, "v")

	if a.emCurso() {
		t.Error("não deveria abrir tarefa para exercício sem suíte")
	}
	if !strings.Contains(a.erro, "suíte") {
		t.Errorf("erro = %q, queria explicar a falta de suíte", a.erro)
	}
}

func TestProgressoAlimentaABarra(t *testing.T) {
	a := appComCliente(t, clienteComEntrega())
	a.tarefa = &tarefa{nome: "coleta", cancelar: func() {}, canal: make(chan tea.Msg, 1)}

	a.Update(progressoMsg{p: acoes.Progresso{Feito: 3, Total: 10, Rotulo: "Ana Souza"}})

	barra := a.barraDeProgresso()
	if !strings.Contains(barra, "3/10") || !strings.Contains(barra, "Ana Souza") {
		t.Errorf("barra = %q", barra)
	}
	if !strings.Contains(a.View(), "3/10") {
		t.Error("a barra deveria ocupar o rodapé enquanto a tarefa roda")
	}
}
