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
	membros  map[string][]gl.Membro
	// renovacoes conta os pedidos de descarte do cache.
	renovacoes int
}

func (c *clienteFalso) Renovar() { c.renovacoes++ }

func (c *clienteFalso) UsuarioExiste(login string) (bool, error) { return true, nil }

func (c *clienteFalso) Grupo(caminho string) (*gl.Grupo, error) {
	g, ok := c.grupos[caminho]
	if !ok {
		return nil, nil
	}
	return &g, nil
}

func (c *clienteFalso) Eu() (string, error) { return "alexkutzke", nil }

func (c *clienteFalso) MembroDoGrupo(caminho string) (bool, error) {
	g, ok := c.grupos[caminho]
	return ok && g.Membro, nil
}

func (c *clienteFalso) GruposComAcesso(busca string) ([]gl.Grupo, error) {
	var out []gl.Grupo
	for _, g := range c.grupos {
		if busca == "" || strings.Contains(strings.ToLower(g.Caminho), strings.ToLower(busca)) {
			out = append(out, g)
		}
	}
	return out, nil
}

func (c *clienteFalso) ProjetosDoGrupo(grupo string) ([]gl.Projeto, error) {
	return c.projetos[grupo], nil
}

func (c *clienteFalso) Commits(projeto, ramo string, todos bool) ([]gl.Commit, error) {
	return c.commits[projeto], nil
}

func (c *clienteFalso) Membros(projeto string) ([]gl.Membro, error) {
	return c.membros[projeto], nil
}

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

// O cliente do GitLab dura a sessão inteira na TUI, e as listagens dele são
// memorizadas. Sem descartar o cache a cada coleta, o grupo que o aluno
// acabou de criar só apareceria depois de fechar e reabrir a aplicação.
func TestCadaColetaDescartaOCacheDoCliente(t *testing.T) {
	c := clienteComEntrega()
	a := appComCliente(t, c)

	bombear(t, a, teclarCmd(a, "c"))
	if c.renovacoes != 1 {
		t.Fatalf("renovações = %d, queria 1", c.renovacoes)
	}
	bombear(t, a, teclarCmd(a, "c"))
	if c.renovacoes != 2 {
		t.Errorf("renovações = %d: a segunda coleta também precisa partir do zero", c.renovacoes)
	}
}

func TestBuscaDeMembroSemCadastroListaNaTelaDeTarefas(t *testing.T) {
	c := clienteComEntrega()
	fork := "ds122-2026-2-n-grr20259001/ds122-prepare-assignment"
	c.membros = map[string][]gl.Membro{fork: {
		{Usuario: "grr20259001", Nome: "Ana Souza", NivelAcesso: 50},
		{Usuario: "alexkutzke", Nome: "Alexander Kutzke", NivelAcesso: 20},
		{Usuario: "bruno.lima", Nome: "Bruno Lima", NivelAcesso: 30},
	}}
	a := appComCliente(t, c)

	teclar(a, "e") // tela de equipes
	bombear(t, a, teclarCmd(a, "u"))

	if a.erro != "" {
		t.Fatalf("erro: %s", a.erro)
	}
	juntas := strings.Join(a.tarefas.linhas, "\n")
	if !strings.Contains(juntas, "bruno.lima") {
		t.Errorf("tarefas = %q, queria o login sem cadastro", juntas)
	}
	if !strings.Contains(juntas, "GRR20259002") {
		t.Errorf("tarefas = %q, queria o palpite pelo nome", juntas)
	}
	if strings.Contains(juntas, "alexkutzke") {
		t.Error("o dono do token é membro de todo fork e não pode entrar na lista")
	}
	if strings.Contains(juntas, "grr20259001") {
		t.Error("login que casa com o cadastro não é desconhecido")
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
