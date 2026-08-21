package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	gl "github.com/alexkutzke/gitlab-classroom/internal/gitlab"
	"github.com/alexkutzke/gitlab-classroom/internal/store"
	"github.com/alexkutzke/gitlab-classroom/internal/turma"
)

func turmaExemplo() *turma.Turma {
	t := &turma.Turma{
		Config: turma.Config{
			Codigo: "DS122", Turma: "TADSN2A", Semestre: "2026-02", Turno: "n",
			PadraoGrupo: "{codigo}-{ano}-{periodo}-{turno}-{grr}",
		},
		Alunos: []turma.Aluno{
			{GRR: "GRR20259001", Nome: "Ana Souza", Situacao: turma.Ativo,
				SituacaoConta: turma.ContaOK, Grupo: "ds122-2026-2-n-grr20259001"},
			{GRR: "GRR20259002", Nome: "Bruno Lima", Situacao: turma.Ativo,
				SituacaoConta: turma.ContaGrupoInvisivel},
			{GRR: "GRR20259003", Nome: "Carla Dias", Situacao: turma.Cancelado},
		},
		Exercicios: []turma.Exercicio{
			{ID: "prepare", Repo: "ds122-prepare-assignment", Titulo: "Preparação",
				Prazo: turma.NovaData(2026, time.August, 15), Peso: 1, Situacao: turma.ExercicioAtivo},
			{ID: "html", Repo: "ds122-html-assignment", Titulo: "HTML e CSS",
				Prazo: turma.NovaData(2026, time.September, 5), Peso: 2, Situacao: turma.ExercicioAtivo},
		},
		Entregas: []turma.Entrega{
			{Exercicio: "prepare", GRR: "GRR20259001", Situacao: turma.Entregue, Commits: 4,
				Projeto: "ds122-2026-2-n-grr20259001/ds122-prepare-assignment", Commit: "abc"},
			{Exercicio: "prepare", GRR: "GRR20259002", Situacao: turma.SemFork},
		},
		Notas: []turma.Nota{
			{Exercicio: "prepare", GRR: "GRR20259001", Valor: 95},
		},
	}
	t.Config.Padroes()
	return t
}

func appExemplo(t *testing.T) *App {
	t.Helper()
	s, err := store.Criar(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	tur := turmaExemplo()
	if err := s.Gravar(tur); err != nil {
		t.Fatal(err)
	}
	a := &App{
		store:   s,
		turma:   tur,
		abrirGL: func() (gl.Cliente, error) { return nil, nil },
		largura: 120,
		altura:  30,
	}
	a.recarregarPanorama()
	return a
}

func teclar(a *App, teclas ...string) {
	for _, t := range teclas {
		var msg tea.KeyMsg
		switch t {
		case "enter":
			msg = tea.KeyMsg{Type: tea.KeyEnter}
		case "esc":
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		case "down":
			msg = tea.KeyMsg{Type: tea.KeyDown}
		case "up":
			msg = tea.KeyMsg{Type: tea.KeyUp}
		case "backspace":
			msg = tea.KeyMsg{Type: tea.KeyBackspace}
		default:
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(t)}
		}
		a.Update(msg)
	}
}

func TestPainelMostraPendenciasEExercicios(t *testing.T) {
	a := appExemplo(t)
	tela := a.View()

	if !strings.Contains(tela, "DS122") {
		t.Errorf("cabeçalho sem a turma:\n%s", tela)
	}
	for _, quer := range []string{"prepare", "html", "Pendências"} {
		if !strings.Contains(tela, quer) {
			t.Errorf("painel não mostra %q:\n%s", quer, tela)
		}
	}
	// Bruno está sem grupo visível e o prepare já venceu com um sem entrega.
	if !strings.Contains(tela, "sem grupo em ordem") {
		t.Errorf("pendência de cadastro ausente:\n%s", tela)
	}
	if !strings.Contains(tela, "sem entrega no prazo") {
		t.Errorf("pendência de entrega ausente:\n%s", tela)
	}
}

func TestAlunoCanceladoNaoContaNoPainel(t *testing.T) {
	a := appExemplo(t)
	if a.panorama.Ativos != 2 {
		t.Errorf("ativos = %d, queria 2", a.panorama.Ativos)
	}
}

func TestEnterAbreOExercicioSobOCursor(t *testing.T) {
	a := appExemplo(t)
	teclar(a, "enter")

	if a.tela != idExercicio {
		t.Fatalf("tela = %v, queria a de entregas", a.tela)
	}
	if a.exercicio != "prepare" {
		t.Errorf("exercício aberto = %q, queria o primeiro por prazo", a.exercicio)
	}
	tela := a.View()
	if !strings.Contains(tela, "Ana Souza") || !strings.Contains(tela, "Bruno Lima") {
		t.Errorf("a tela do exercício deveria listar os alunos ativos:\n%s", tela)
	}
	if strings.Contains(tela, "Carla") {
		t.Errorf("aluno cancelado não entra na lista:\n%s", tela)
	}
	if !strings.Contains(tela, "95") {
		t.Errorf("a nota lançada deveria aparecer:\n%s", tela)
	}
}

func TestCursorDoPainelEscolheOSegundoExercicio(t *testing.T) {
	a := appExemplo(t)
	teclar(a, "j", "enter")

	if a.exercicio != "html" {
		t.Errorf("exercício aberto = %q, queria html", a.exercicio)
	}
}

func TestEscVoltaAoPainel(t *testing.T) {
	a := appExemplo(t)
	teclar(a, "enter", "esc")

	if a.tela != idPainel {
		t.Errorf("tela = %v, queria voltar ao painel", a.tela)
	}
}

func TestTrocaDeTelasPorAtalho(t *testing.T) {
	a := appExemplo(t)
	casos := []struct {
		tecla string
		quer  telaID
	}{
		{"a", idAlunos},
		{"e", idEquipes},
		{"t", idTarefas},
		{"?", idAjuda},
		{"p", idPainel},
	}
	for _, c := range casos {
		teclar(a, c.tecla)
		if a.tela != c.quer {
			t.Errorf("tecla %q levou à tela %v, queria %v", c.tecla, a.tela, c.quer)
		}
		if a.View() == "" {
			t.Errorf("tela %v desenhou vazio", a.tela)
		}
	}
}

func TestFiltroCapturaAsLetrasEmVezDeSair(t *testing.T) {
	a := appExemplo(t)
	teclar(a, "enter") // abre o prepare
	teclar(a, "/", "b", "r", "u", "q")

	if a.sair {
		t.Fatal("o q digitado dentro do filtro não pode encerrar a aplicação")
	}
	if a.entregas.buffer != "bruq" {
		t.Errorf("buffer = %q, queria as letras digitadas", a.entregas.buffer)
	}

	teclar(a, "backspace", "enter")
	if a.entregas.filtro != "bru" {
		t.Errorf("filtro = %q", a.entregas.filtro)
	}
	tela := a.View()
	if strings.Contains(tela, "Ana Souza") {
		t.Errorf("o filtro deveria esconder quem não casa:\n%s", tela)
	}
	if !strings.Contains(tela, "Bruno Lima") {
		t.Errorf("o filtro escondeu quem casa:\n%s", tela)
	}
}

func TestOrdemAlternaComS(t *testing.T) {
	a := appExemplo(t)
	teclar(a, "enter", "s")

	if a.entregas.ordem != ordemSituacao {
		t.Errorf("ordem = %v, queria por situação", a.entregas.ordem)
	}
	// Por situação, quem está pior vem primeiro.
	linhas := a.entregas.linhas(a)
	if linhas[0].Aluno.Nome != "Bruno Lima" {
		t.Errorf("primeiro da lista = %s, queria quem está sem fork", linhas[0].Aluno.Nome)
	}

	teclar(a, "s", "s")
	if a.entregas.ordem != ordemNome {
		t.Errorf("a ordem deveria dar a volta e voltar ao nome, veio %v", a.entregas.ordem)
	}
}

func TestQNoPainelEncerra(t *testing.T) {
	a := appExemplo(t)
	teclar(a, "q")

	if !a.sair {
		t.Error("q no painel deveria encerrar")
	}
}

func TestQForaDoPainelSoVolta(t *testing.T) {
	a := appExemplo(t)
	teclar(a, "a", "q")

	if a.sair {
		t.Error("q fora do painel não pode encerrar")
	}
	if a.tela != idPainel {
		t.Errorf("tela = %v, queria o painel", a.tela)
	}
}

func TestTelaDeAlunosFiltraPendentes(t *testing.T) {
	a := appExemplo(t)
	teclar(a, "a", "P")

	lista := a.alunos.lista(a)
	if len(lista) != 1 || lista[0].Nome != "Bruno Lima" {
		t.Errorf("lista de pendentes = %+v, queria só o Bruno", lista)
	}
}

func TestRecarregarDoDiscoPegaEdicaoManual(t *testing.T) {
	a := appExemplo(t)
	// Outro processo, ou o próprio professor no editor, mexeu no arquivo.
	outra := turmaExemplo()
	outra.RegistrarNota(turma.Nota{Exercicio: "prepare", GRR: "GRR20259002", Valor: 60})
	if err := a.store.Gravar(outra); err != nil {
		t.Fatal(err)
	}

	teclar(a, "r")

	if _, ok := a.turma.Nota("prepare", "GRR20259002"); !ok {
		t.Error("recarregar deveria trazer a nota gravada por fora")
	}
	if a.status == "" {
		t.Error("o recarregamento precisa aparecer na barra de status")
	}
}

func TestTelaPequenaNaoQuebraODesenho(t *testing.T) {
	a := appExemplo(t)
	a.Update(tea.WindowSizeMsg{Width: 40, Height: 10})
	teclar(a, "enter")

	if a.View() == "" {
		t.Error("a tela do exercício ficou vazia num terminal pequeno")
	}
}
