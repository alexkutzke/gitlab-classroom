package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/alexkutzke/gitlab-classroom/internal/acoes"
	"github.com/alexkutzke/gitlab-classroom/internal/turma"
)

// tamanhos são os mesmos do teste do painel: largo, o limite das duas
// colunas dos dois lados, o terminal clássico, estreito e o mínimo.
var tamanhos = []struct{ w, h int }{
	{140, 40}, {100, 30}, {99, 30}, {80, 24}, {60, 20}, {40, 12},
}

// conferirTamanho exige o terminal exato: tantas linhas quanto a altura, e
// cada uma com a largura exata. Linha com uma coluna a mais quebra no
// terminal e desalinha todas as bordas abaixo dela.
func conferirTamanho(t *testing.T, caso, tela string, w, h int) {
	t.Helper()
	linhas := strings.Split(tela, "\n")
	if len(linhas) != h {
		t.Errorf("%s: %d linhas, queria %d\n%s", caso, len(linhas), h, tela)
		return
	}
	for i, l := range linhas {
		if lw := ansi.StringWidth(l); lw != w {
			t.Errorf("%s: linha %d com %d colunas, queria %d: %q", caso, i+1, lw, w, ansi.Strip(l))
			return
		}
	}
}

// appCheio é a turma de exemplo com o que mais estica o desenho: nome longo,
// comentário com quebra de linha e tabulação, dupla, verificação velha,
// devolutiva e registro de tarefa.
func appCheio(t *testing.T) *App {
	t.Helper()
	a := appExemplo(t)
	tu := a.turma
	tu.Alunos = append(tu.Alunos, turma.Aluno{GRR: "GRR20259004",
		Nome: "MARIA APARECIDA DOS SANTOS FICTICIA DE ALBUQUERQUE", Situacao: turma.Ativo})
	tu.Exercicios[0].Verificacao = "./verificar.sh"
	tu.Entregas = append(tu.Entregas, turma.Entrega{Exercicio: "prepare", GRR: "GRR20259004",
		Situacao: turma.Entregue, Commits: 2, AtrasoDias: 3,
		Projeto: "ds122-2026-2-n-grr20259001/ds122-prepare-assignment", Commit: "abc",
		Detalhe: "detalhe\tcom tabulação"})
	tu.RegistrarVinculo(turma.Vinculo{Exercicio: "prepare", GRR: "GRR20259004",
		Dono: "GRR20259001", Origem: turma.VinculoDescoberto})
	tu.RegistrarVerificacao(turma.Verificacao{Exercicio: "prepare", GRR: "GRR20259001",
		Situacao: turma.Reprovado, Aprovados: 6, Total: 8, Commit: "velho",
		Detalhe: "falhou o teste 7:\n\tesperado <form>, veio <div>"})
	n, _ := tu.Nota("prepare", "GRR20259001")
	n.Comentario = strings.Repeat("faltou o label nos campos do formulário. ", 12) +
		"\n\nSegundo parágrafo\tcom tabulação."
	tu.RegistrarDevolutiva(turma.Devolutiva{Exercicio: "prepare", GRR: "GRR20259001",
		Projeto: "ds122-2026-2-n-grr20259001/ds122-prepare-assignment", Issue: 3,
		URL:  "https://gitlab.com/ds122-2026-2-n-grr20259001/ds122-prepare-assignment/-/issues/3",
		Hash: "000000"})
	tu.Ordenar()
	a.recarregarPanorama()
	a.tarefas.registrar("exportação: %s", "/tmp/entregas.md")
	r := a.tarefas.abrir("coleta de prepare, html")
	a.tarefas.encerrar(r, regConcluido, "prepare: 3 entregue")
	for i := range 12 {
		a.tarefas.acrescentar(r, fmt.Sprintf("linha de detalhe %d\tcom tabulação", i))
	}
	return a
}

// emCursoFalso põe uma tarefa rodando, com progresso, sem goroutine.
func emCursoFalso(a *App) {
	a.tarefa = &tarefa{nome: "coleta de prepare", cancelar: func() {}, canal: make(chan tea.Msg, 1),
		registro: a.tarefas.abrir("coleta de prepare"),
		prog:     acoes.Progresso{Feito: 12, Total: 31, Rotulo: "MARIA APARECIDA DOS SANTOS"}}
}

func TestTelaOcupaOTerminalExato(t *testing.T) {
	telas := []struct {
		nome  string
		abrir func(a *App)
	}{
		{"painel", func(a *App) {}},
		{"entregas", func(a *App) { teclar(a, "enter") }},
		{"exercícios", func(a *App) { teclar(a, "x") }},
		{"alunos", func(a *App) { teclar(a, "a") }},
		{"equipes", func(a *App) { teclar(a, "e") }},
		{"tarefas", func(a *App) { teclar(a, "t") }},
		{"correção", func(a *App) { teclar(a, "enter", "n") }},
	}
	for _, tl := range telas {
		for _, tarefa := range []bool{false, true} {
			for _, tam := range tamanhos {
				a := appCheio(t)
				a.Update(tea.WindowSizeMsg{Width: tam.w, Height: tam.h})
				tl.abrir(a)
				if tarefa {
					emCursoFalso(a)
				}
				n := a.numPaineis()
				if a.tela == idCorrecao {
					n = 2
				}
				for foco := range n {
					if a.tela == idCorrecao {
						if foco == 1 {
							teclar(a, "tab")
						}
					} else {
						a.foco[a.tela] = foco
					}
					caso := fmt.Sprintf("%s %dx%d foco %d tarefa %t", tl.nome, tam.w, tam.h, foco+1, tarefa)
					conferirTamanho(t, caso, a.View(), tam.w, tam.h)
				}
			}
		}
	}
}

func TestEdicaoEAjudaOcupamOTerminalExato(t *testing.T) {
	estados := []struct {
		nome   string
		teclas []string
	}{
		{"ajuda", []string{"?"}},
		{"filtro de entregas", []string{"enter", "/", "a"}},
		{"vínculo", []string{"enter", "V"}},
		{"edição de prazo", []string{"x", "D"}},
		{"cadastro novo", []string{"x", "n", "d", "s"}},
		{"nota na correção", []string{"enter", "n", "8"}},
		{"comentário na correção", []string{"enter", "n", "c", "o", "k"}},
		{"ajuda da correção", []string{"enter", "n", "?"}},
		{"confirmação", []string{"x", "A"}},
	}
	for _, e := range estados {
		for _, tam := range tamanhos {
			a := appCheio(t)
			a.Update(tea.WindowSizeMsg{Width: tam.w, Height: tam.h})
			teclar(a, e.teclas...)
			conferirTamanho(t, fmt.Sprintf("%s %dx%d", e.nome, tam.w, tam.h), a.View(), tam.w, tam.h)
		}
	}
}

func TestTerminalPequenoPedeParaAumentar(t *testing.T) {
	a := appCheio(t)
	a.Update(tea.WindowSizeMsg{Width: 39, Height: 12})
	tela := a.View()
	conferirTamanho(t, "39x12", tela, 39, 12)
	if !strings.Contains(tela, "aumente o terminal") {
		t.Errorf("a tela pequena não pede para aumentar:\n%s", tela)
	}
}

func TestTabPassaPelosPaineis(t *testing.T) {
	a := appCheio(t)
	a.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	// Sem tarefa recente, a tela inicial tem três painéis.
	relogio = func() time.Time { return time.Now().Add(time.Hour) }
	defer func() { relogio = time.Now }()

	var focos []int
	for i := range 4 {
		focos = append(focos, a.focoAtual())
		if i < 3 {
			teclar(a, "tab")
		}
	}
	if fmt.Sprint(focos) != "[0 1 2 0]" {
		t.Errorf("focos com tab = %v, queria [0 1 2 0]", focos)
	}
	a.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if a.focoAtual() != 2 {
		t.Errorf("shift+tab da lista foi para %d, queria o último painel", a.focoAtual())
	}
	teclar(a, "2")
	if a.focoAtual() != focoDetalhe {
		t.Errorf("2 levou o foco a %d, queria o detalhe", a.focoAtual())
	}
	teclar(a, "esc")
	if a.focoAtual() != focoLista || a.tela != idPainel {
		t.Errorf("esc no detalhe deveria devolver o foco à lista: foco %d, tela %v", a.focoAtual(), a.tela)
	}
}

func TestDetalheDasEntregasSegueOCursor(t *testing.T) {
	a := appCheio(t)
	a.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	teclar(a, "enter")

	titulo, _ := a.conteudoDetalhe(60)
	if titulo != "Ana Souza" {
		t.Fatalf("detalhe = %q, queria a primeira da lista", titulo)
	}
	teclar(a, "j")
	titulo, c := a.conteudoDetalhe(60)
	if titulo != "Bruno Lima" {
		t.Errorf("depois de j o detalhe = %q, queria o Bruno", titulo)
	}
	if !strings.Contains(ansi.Strip(strings.Join(c.Linhas, "\n")), "GRR20259002") {
		t.Errorf("o detalhe não traz o GRR do Bruno:\n%s", strings.Join(c.Linhas, "\n"))
	}
}

func TestDetalheDaEntregaMostraComentarioEDevolutiva(t *testing.T) {
	a := appCheio(t)
	a.Update(tea.WindowSizeMsg{Width: 140, Height: 60})
	teclar(a, "enter")

	_, c := a.conteudoDetalhe(60)
	texto := ansi.Strip(strings.Join(c.Linhas, "\n"))
	for _, quer := range []string{"Segundo parágrafo", "desatualizada", "issue #3", "6/8",
		"commit anterior"} {
		if !strings.Contains(texto, quer) {
			t.Errorf("detalhe sem %q:\n%s", quer, texto)
		}
	}
}

func TestDetalheRolaComFocoNele(t *testing.T) {
	a := appCheio(t)
	a.Update(tea.WindowSizeMsg{Width: 140, Height: 20})
	teclar(a, "enter", "tab", "j", "j")

	if a.rolagem[idExercicio] != 2 {
		t.Errorf("rolagem = %d, queria 2", a.rolagem[idExercicio])
	}
	if a.entregas.cursor != 0 {
		t.Errorf("j com o foco no detalhe moveu a lista: cursor %d", a.entregas.cursor)
	}
	teclar(a, "esc", "j")
	if a.rolagem[idExercicio] != 0 {
		t.Errorf("trocar de aluno deveria voltar o detalhe ao topo: rolagem %d", a.rolagem[idExercicio])
	}
}

func TestEnterNaPendenciaSelecionaOExercicio(t *testing.T) {
	a := appCheio(t)
	a.Update(tea.WindowSizeMsg{Width: 140, Height: 40})

	pend := a.panorama.Pendencias()
	alvo := -1
	for i, p := range pend {
		if strings.HasPrefix(p, "prepare:") {
			alvo = i
			break
		}
	}
	if alvo < 0 {
		t.Fatalf("o exemplo devia ter pendência do prepare: %v", pend)
	}
	a.painel.cursor = 1 // html
	teclar(a, "3")
	for range alvo {
		teclar(a, "j")
	}
	teclar(a, "enter")

	if a.painel.cursor != 0 {
		t.Errorf("cursor de [1] = %d, queria o prepare", a.painel.cursor)
	}
	if a.focoAtual() != focoLista {
		t.Errorf("foco = %d, queria voltar a [1]", a.focoAtual())
	}
	if a.tela != idPainel {
		t.Errorf("tela = %v, a pendência de exercício não sai do painel", a.tela)
	}
}

func TestProgressoAparecaNaBarraDeTituloEmQualquerTela(t *testing.T) {
	for _, tecla := range []string{"", "x", "a", "e", "t"} {
		a := appCheio(t)
		a.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
		if tecla != "" {
			teclar(a, tecla)
		}
		emCursoFalso(a)
		primeira := ansi.Strip(strings.SplitN(a.View(), "\n", 2)[0])
		if !strings.Contains(primeira, "coletando 12/31") {
			t.Errorf("tela %q: barra de título sem o progresso: %q", tecla, primeira)
		}
	}

	// A correção desenha a própria barra, e o progresso vai junto.
	a := appCheio(t)
	a.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	teclar(a, "enter", "n")
	emCursoFalso(a)
	primeira := ansi.Strip(strings.SplitN(a.View(), "\n", 2)[0])
	if !strings.Contains(primeira, "coletando 12/31") {
		t.Errorf("correção: barra de título sem o progresso: %q", primeira)
	}
}

func TestAbasMarcamATelaAtual(t *testing.T) {
	a := appCheio(t)
	a.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	teclar(a, "a")
	primeira := ansi.Strip(strings.SplitN(a.View(), "\n", 2)[0])
	for _, aba := range []string{"p Painel", "x Exercícios", "a Alunos", "e Equipes", "t Tarefas", "DS122 TADSN2A"} {
		if !strings.Contains(primeira, aba) {
			t.Errorf("barra de título sem %q: %q", aba, primeira)
		}
	}
}

func TestAjudaAbreSobreATelaEATeclaQueFechaNaoAge(t *testing.T) {
	a := appCheio(t)
	teclar(a, "a", "?")
	if !a.ajuda || a.tela != idAlunos {
		t.Fatalf("? deveria abrir a ajuda sobre a tela de alunos: ajuda %t, tela %v", a.ajuda, a.tela)
	}
	if !strings.Contains(a.View(), "qualquer tecla fecha") {
		t.Error("a caixa de ajuda não apareceu")
	}
	teclar(a, "q")
	if a.ajuda {
		t.Error("qualquer tecla deveria fechar a ajuda")
	}
	if a.tela != idAlunos {
		t.Errorf("o q que fechou a ajuda agiu por baixo: tela %v", a.tela)
	}
}

func TestPainelDaTarefaApareceSoComTarefaRecente(t *testing.T) {
	a := appExemplo(t)
	if a.tarefaVisivel() {
		t.Fatal("sem tarefa, a tela inicial não tem o painel [4]")
	}
	emCursoFalso(a)
	if !a.tarefaVisivel() || a.numPaineis() != 4 {
		t.Error("tarefa em curso deveria abrir o painel [4]")
	}
	a.Update(fimMsg{resumo: "prepare: 1 entregue"})
	if !a.tarefaVisivel() {
		t.Error("a tarefa recém-terminada deveria continuar à vista")
	}
	relogio = func() time.Time { return time.Now().Add(time.Hour) }
	defer func() { relogio = time.Now }()
	if a.tarefaVisivel() {
		t.Error("tarefa terminada há uma hora não ocupa mais a tela inicial")
	}
}

func TestRegistroDaTarefaGuardaResumoEDetalhes(t *testing.T) {
	a := appExemplo(t)
	emCursoFalso(a)
	a.Update(fimMsg{resumo: "prepare: 1 entregue", detalhes: []string{"piorou, Bruno"}})

	r := a.tarefas.ultimaOperacao()
	if r == nil || r.estado != regConcluido {
		t.Fatalf("registro = %+v", r)
	}
	if strings.Join(r.linhas, "|") != "prepare: 1 entregue|piorou, Bruno" {
		t.Errorf("linhas = %q", r.linhas)
	}
	a.tarefa = &tarefa{nome: "coleta", cancelar: func() {}, registro: a.tarefas.abrir("coleta")}
	a.Update(fimMsg{err: context.Canceled})
	if r := a.tarefas.ultimaOperacao(); r.estado != regCancelado {
		t.Errorf("estado = %v, queria cancelada", r.estado)
	}
}

// Em 140x40 a ajuda inteira cabe; o corte é para terminal menor.
func TestAjudaCompletaEmTerminalGrande(t *testing.T) {
	a := appCheio(t)
	a.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	teclar(a, "?")
	if tela := ansi.Strip(a.View()); strings.Contains(tela, "sem espaço") {
		t.Errorf("em 140x40 todos os grupos deveriam caber:\n%s", tela)
	}
}
