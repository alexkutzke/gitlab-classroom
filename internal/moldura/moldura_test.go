package moldura

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func conferir(t *testing.T, caso, s string, w, h int) {
	t.Helper()
	linhas := strings.Split(s, "\n")
	if len(linhas) != h {
		t.Fatalf("%s: %d linhas, queria %d", caso, len(linhas), h)
	}
	for i, l := range linhas {
		if lw := ansi.StringWidth(l); lw != w {
			t.Fatalf("%s: linha %d com %d colunas, queria %d: %q", caso, i+1, lw, w, ansi.Strip(l))
		}
	}
}

func TestCaixaTemOTamanhoPedido(t *testing.T) {
	c := Conteudo{Linhas: []string{"curta", strings.Repeat("longa ", 30), "com\ttab"}, Sel: 1}
	for _, tam := range []struct{ w, h int }{{40, 5}, {10, 3}, {80, 2}} {
		conferir(t, "caixa", Caixa("1", "Título bem comprido para a borda", c, tam.w, tam.h, true), tam.w, tam.h)
	}
}

func TestRecorteMantemASelecaoAVista(t *testing.T) {
	c := Conteudo{Linhas: make([]string, 20), Sel: 15}
	if topo := c.Recorte(5); topo != 11 {
		t.Errorf("topo = %d, queria 11", topo)
	}
	c.Livre, c.Topo = true, 30
	if topo := c.Recorte(5); topo != 15 {
		t.Errorf("rolagem livre além do fim = %d, queria parar em 15", topo)
	}
}

func TestBarrasTemALarguraExata(t *testing.T) {
	esq := []Trecho{{Texto: strings.Repeat("aba ", 30), Est: EstAcento}}
	dir := []Trecho{{Texto: "coletando 12/31", Est: EstAtencao}}
	for _, w := range []int{140, 60, 12, 5} {
		conferir(t, "título", BarraTitulo(w, "classroom", esq, dir), w, 1)
		conferir(t, "teclas", BarraTeclas(w, []Tecla{{"tab", "painel"}, {"q", strings.Repeat("sai ", 40)}}), w, 1)
		conferir(t, "mensagem", BarraMensagem(w, "erro:\tcom\nquebra", EstErro), w, 1)
	}
}

func TestBarraTituloLargaMostraADireita(t *testing.T) {
	b := ansi.Strip(BarraTitulo(80, "classroom", []Trecho{{Texto: " p Painel", Est: EstFraco}},
		[]Trecho{{Texto: "coletando 3/10 ", Est: EstAtencao}}))
	if !strings.HasPrefix(b, " classroom ") || !strings.HasSuffix(b, "coletando 3/10 ") {
		t.Errorf("barra = %q", b)
	}
}

func TestQuebrarRespeitaALarguraEOsParagrafos(t *testing.T) {
	linhas := Quebrar("uma frase comprida o bastante\n\nsegundo\tparágrafo "+strings.Repeat("x", 25), 10)
	branca := false
	for _, l := range linhas {
		if ansi.StringWidth(l) > 10 {
			t.Errorf("linha com %d colunas: %q", ansi.StringWidth(l), l)
		}
		branca = branca || l == ""
	}
	if !branca {
		t.Errorf("a linha em branco entre parágrafos sumiu: %q", linhas)
	}
}

func TestAjudaCabeNaTelaEDizOQueFicouDeFora(t *testing.T) {
	var grupos []GrupoAjuda
	for _, nome := range []string{"Um", "Dois", "Três", "Quatro", "Cinco", "Seis"} {
		g := GrupoAjuda{Titulo: nome}
		for range 8 {
			g.Teclas = append(g.Teclas, Tecla{"k", "uma descrição de tecla razoavelmente longa"})
		}
		grupos = append(grupos, g)
	}
	tela := Ajuda(80, 24, grupos, "qualquer tecla fecha")
	conferir(t, "ajuda", tela, 80, 24)
	if !strings.Contains(ansi.Strip(tela), "sem espaço para") {
		t.Errorf("a ajuda cortada precisa dizer o que ficou de fora:\n%s", ansi.Strip(tela))
	}
	conferir(t, "ajuda larga", Ajuda(200, 60, grupos, ""), 200, 60)
}
