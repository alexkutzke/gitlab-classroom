package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexkutzke/gitlab-classroom/internal/acoes"
)

func TestExigeTerminalRecusaSemTerminal(t *testing.T) {
	nome := filepath.Join(t.TempDir(), "entrada")
	f, err := os.Create(nome)
	if err != nil {
		t.Fatalf("arquivo de entrada: %v", err)
	}
	defer f.Close()

	err = exigeTerminal(f)
	if err == nil {
		t.Fatal("--confirmar foi aceito sem terminal, e a rodada ficaria parada esperando resposta")
	}
	if !strings.Contains(err.Error(), "--confirmar") {
		t.Errorf("o erro não diz qual opção recusou a rodada: %v", err)
	}
}

func TestRevisorTerminalLeAsRespostas(t *testing.T) {
	entrada := strings.NewReader("x\ns\nN\n e \nT\nq\n")
	revisar := revisorTerminal(entrada)
	it := acoes.ItemDevolutiva{Nome: "ANA SOUZA", GRR: "GRR20259001",
		Projeto: "grupo/projeto", Titulo: "Devolutiva: HTML", Corpo: "texto\n"}

	// A primeira resposta é inválida e não conta: a pergunta se repete até
	// vir uma letra conhecida.
	esperadas := []acoes.DecisaoDevolutiva{
		acoes.DecisaoPublicar, acoes.DecisaoPular, acoes.DecisaoEditar,
		acoes.DecisaoTodas, acoes.DecisaoSair,
	}
	for i, esperada := range esperadas {
		dec, err := revisar(it)
		if err != nil {
			t.Fatalf("resposta %d: %v", i+1, err)
		}
		if dec != esperada {
			t.Errorf("resposta %d = %q, esperado %q", i+1, dec, esperada)
		}
	}

	// Entrada esgotada encerra a rodada em vez de publicar sem resposta.
	dec, err := revisar(it)
	if err != nil {
		t.Fatalf("entrada esgotada: %v", err)
	}
	if dec != acoes.DecisaoSair {
		t.Errorf("entrada esgotada = %q, esperado %q", dec, acoes.DecisaoSair)
	}
}

func TestQuebrarPreservaParagrafos(t *testing.T) {
	texto := "@grr20259001, segue a devolutiva da sua entrega.\n\n" +
		strings.Repeat("palavra ", 20) + "\n"
	saida := quebrar(texto, 40)

	linhas := strings.Split(strings.TrimRight(saida, "\n"), "\n")
	branca := false
	for _, l := range linhas {
		if l == "" {
			branca = true
		}
		if len([]rune(l)) > 40 {
			t.Errorf("linha com %d colunas: %q", len([]rune(l)), l)
		}
	}
	if !branca {
		t.Errorf("a linha em branco entre parágrafos sumiu: %q", linhas)
	}
	if strings.Join(strings.Fields(saida), " ") != strings.Join(strings.Fields(texto), " ") {
		t.Errorf("a quebra alterou o texto:\n%s", saida)
	}
}

func TestBlocoDevolutivaMostraOCorpoQueSeraPublicado(t *testing.T) {
	it := acoes.ItemDevolutiva{
		Nome: "ANA SOUZA", GRR: "GRR20259001", Projeto: "grupo/projeto",
		Titulo: "Devolutiva: HTML e CSS",
		Corpo:  "@grr20259001, segue a devolutiva.\n\nfaltou o label.\n",
		Equipe: []string{"GRR20259002"},
	}
	bloco := blocoDevolutiva(it)
	for _, trecho := range []string{
		"ANA SOUZA  GRR20259001", "grupo/projeto", "Devolutiva: HTML e CSS",
		"faltou o label.", "em equipe com GRR20259002",
	} {
		if !strings.Contains(bloco, trecho) {
			t.Errorf("bloco sem %q:\n%s", trecho, bloco)
		}
	}
}
