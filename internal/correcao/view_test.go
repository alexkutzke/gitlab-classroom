package correcao

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/alexkutzke/gitlab-classroom/internal/turma"
)

var tamanhos = []struct{ w, h int }{
	{140, 40}, {100, 30}, {99, 30}, {80, 24}, {60, 20}, {40, 12},
}

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

// modeloLongo traz o que estica o desenho: nome longo, comentário com
// parágrafos e tabulação, verificação velha e devolutiva publicada.
func modeloLongo() *modelo {
	m := modeloComDupla(true)
	m.itens[0].Aluno.Nome = "MARIA APARECIDA DOS SANTOS FICTICIA DE ALBUQUERQUE"
	m.itens[0].Preencher(turma.Nota{Valor: 95, Comentario: strings.Repeat("faltou o label. ", 40) +
		"\n\nSegundo\tparágrafo."})
	m.itens[0].Verificacao = turma.Verificacao{Situacao: turma.Reprovado, Aprovados: 6, Total: 8,
		Commit: "velho", Detalhe: "falhou o teste 7:\n\tesperado <form>"}
	m.itens[0].Entrega.Commit = "novo"
	m.itens[0].Devolutiva = &turma.Devolutiva{Issue: 3, Hash: "000000"}
	return m
}

func TestCorrecaoOcupaOTerminalExato(t *testing.T) {
	estados := []struct {
		nome   string
		teclas []string
	}{
		{"lista", nil},
		{"detalhe", []string{"tab"}},
		{"detalhe rolado", []string{"tab", "G"}},
		{"nota", []string{"8"}},
		{"comentário", []string{"c", "x"}},
		{"filtro", []string{"/", "a"}},
		{"ajuda", []string{"?"}},
		{"aviso", []string{"r"}},
	}
	for _, e := range estados {
		for _, tam := range tamanhos {
			m := modeloLongo()
			m.Update(tea.WindowSizeMsg{Width: tam.w, Height: tam.h})
			m.anuncio = "coletando 12/31"
			teclar(m, e.teclas...)
			conferirTamanho(t, fmt.Sprintf("%s %dx%d", e.nome, tam.w, tam.h), m.View(), tam.w, tam.h)
		}
	}
}

func TestDetalheDaCorrecaoSegueOCursor(t *testing.T) {
	m := modeloLongo()
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	if !strings.Contains(ansi.Strip(m.View()), "[2] MARIA APARECIDA") {
		t.Fatalf("o detalhe deveria mostrar a primeira da lista:\n%s", ansi.Strip(m.View()))
	}
	teclar(m, "j")
	tela := ansi.Strip(m.View())
	if !strings.Contains(tela, "[2] Bruno Lima") || !strings.Contains(tela, "GRR2") {
		t.Errorf("depois de j o detalhe deveria ser do Bruno:\n%s", tela)
	}
}

func TestDetalheDaCorrecaoMostraComentarioEDevolutiva(t *testing.T) {
	m := modeloLongo()
	texto := ansi.Strip(strings.Join(m.linhasDetalhe(60), "\n"))
	for _, quer := range []string{"Segundo parágrafo", "desatualizada", "issue #3", "commit anterior"} {
		if !strings.Contains(texto, quer) {
			t.Errorf("detalhe sem %q:\n%s", quer, texto)
		}
	}
}

func TestComentarioEmEdicaoFicaAVista(t *testing.T) {
	m := modeloLongo()
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 20})
	teclar(m, "c", "Z", "Z")
	if !strings.Contains(ansi.Strip(m.View()), "ZZ_") {
		t.Errorf("o texto digitado deveria aparecer no painel [2]:\n%s", ansi.Strip(m.View()))
	}
}

func TestComFocoNoDetalheJRolaEDigitoAindaLancaNota(t *testing.T) {
	m := modeloLongo()
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 20})
	teclar(m, "tab", "j", "j")
	if m.cursor != 0 || m.topoDet != 2 {
		t.Errorf("cursor %d, rolagem %d: j com foco no detalhe rola e não move a lista", m.cursor, m.topoDet)
	}
	teclar(m, "7", "0", "enter")
	if m.itens[0].nota != 70 {
		t.Errorf("nota = %v: o dígito continua lançando a nota com o foco no detalhe", m.itens[0].nota)
	}
	teclar(m, "esc")
	if m.encerrada || m.foco != 0 {
		t.Errorf("esc no detalhe devolve o foco à lista: encerrada %t, foco %d", m.encerrada, m.foco)
	}
	teclar(m, "esc")
	if !m.encerrada || m.salvar {
		t.Error("esc na lista continua saindo sem gravar")
	}
}

func TestAjudaDaCorrecaoFechaSemAgir(t *testing.T) {
	m := modeloLongo()
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	teclar(m, "?")
	if !m.ajuda {
		t.Fatal("? deveria abrir a ajuda")
	}
	teclar(m, "q")
	if m.ajuda || m.encerrada {
		t.Errorf("a tecla que fecha a ajuda não pode agir: ajuda %t, encerrada %t", m.ajuda, m.encerrada)
	}
}
