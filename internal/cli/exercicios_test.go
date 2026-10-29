package cli

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/alexkutzke/gitlab-classroom/internal/store"
)

// editarExercicio roda `exercicios editar` sobre a turma de testdata e
// devolve o ramo gravado do exercício html.
func editarExercicio(t *testing.T, pasta string, args ...string) (string, error) {
	t.Helper()
	dirAntes := dirFlag
	t.Cleanup(func() { dirFlag = dirAntes })
	dirFlag = pasta

	// O comando imprime o resultado na saída padrão, que o teste descarta.
	saida := os.Stdout
	nulo, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer nulo.Close()
	os.Stdout = nulo
	defer func() { os.Stdout = saida }()

	c := cmdExerciciosEditar()
	c.SetOut(io.Discard)
	c.SetErr(io.Discard)
	c.SetArgs(append([]string{"--id", "html"}, args...))
	errCmd := c.Execute()

	s, err := store.Descobrir(pasta)
	if err != nil {
		t.Fatal(err)
	}
	tu, err := s.Carregar()
	if err != nil {
		t.Fatal(err)
	}
	e, ok := tu.Exercicio("html")
	if !ok {
		t.Fatal("exercício html sumiu")
	}
	return e.Ramo, errCmd
}

func TestEditarRamoDoExercicio(t *testing.T) {
	pasta := turmaDeTeste(t)

	if ramo, err := editarExercicio(t, pasta, "--ramo", "reentrega"); err != nil || ramo != "reentrega" {
		t.Fatalf("--ramo: ramo = %q, erro %v", ramo, err)
	}
	if ramo, err := editarExercicio(t, pasta, "--sem-ramo"); err != nil || ramo != "" {
		t.Errorf("--sem-ramo: ramo = %q, erro %v; queria voltar ao ramo padrão", ramo, err)
	}
	if _, err := editarExercicio(t, pasta, "--ramo", "reentrega"); err != nil {
		t.Fatal(err)
	}
	if ramo, err := editarExercicio(t, pasta, "--ramo", ""); err != nil || ramo != "" {
		t.Errorf("--ramo vazio: ramo = %q, erro %v; queria limpar", ramo, err)
	}
}

func TestEditarRamoRecusaValorInvalidoEOpcoesJuntas(t *testing.T) {
	pasta := turmaDeTeste(t)
	if ramo, err := editarExercicio(t, pasta, "--ramo", "re;entrega"); err == nil || ramo != "" {
		t.Errorf("ramo com separador aceito: ramo = %q, erro %v", ramo, err)
	}
	if _, err := editarExercicio(t, pasta, "--ramo", "reentrega", "--sem-ramo"); err == nil {
		t.Error("--ramo e --sem-ramo juntos deveriam ser recusados")
	}
	if _, err := os.Stat(filepath.Join(pasta, store.Dir, "exercicios.csv")); err != nil {
		t.Fatal(err)
	}
}
