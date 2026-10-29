package repo

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitEm roda o git na origem falsa, com autor fixo.
func gitEm(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Aluna", "GIT_AUTHOR_EMAIL=aluna@ufpr.br",
		"GIT_COMMITTER_NAME=Aluna", "GIT_COMMITTER_EMAIL=aluna@ufpr.br",
	)
	saida, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, saida)
	}
	return strings.TrimSpace(string(saida))
}

// commitNoRamo cria, a partir do primeiro commit, um ramo com um commit que
// o ramo padrão não tem, como a reentrega feita sobre o commit avaliado
// enquanto o ramo padrão segue para a parte seguinte do trabalho.
func commitNoRamo(t *testing.T, origem, base, ramo, arquivo string) string {
	t.Helper()
	gitEm(t, origem, "checkout", "--quiet", "-B", ramo, base)
	if err := os.WriteFile(filepath.Join(origem, arquivo), []byte("<!-- corrigido -->"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitEm(t, origem, "add", arquivo)
	gitEm(t, origem, "commit", "--quiet", "-m", "corrige na reentrega")
	sha := gitEm(t, origem, "rev-parse", "HEAD")
	gitEm(t, origem, "checkout", "--quiet", "main")
	return sha
}

func TestCommitQueSoExisteNoRamoDaReentregaEhPosicionado(t *testing.T) {
	origem, shas := origemFalsa(t)
	reentrega := commitNoRamo(t, origem, shas[0], "reentrega", "correcao.html")
	destino := filepath.Join(t.TempDir(), "grr20259001-ana")

	r, err := Preparar(destino, origem)
	if err != nil {
		t.Fatal(err)
	}
	head, err := r.Posicionar(NomeRamo("trabalho1"), reentrega)
	if err != nil {
		t.Fatal(err)
	}
	if head != reentrega {
		t.Errorf("HEAD = %s, queria o commit do ramo reentrega %s", head, reentrega)
	}
	ramo, err := rodar(destino, TempoLimite, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(ramo); got != "entrega/trabalho1" {
		t.Errorf("ramo = %q, queria entrega/trabalho1", got)
	}
}

// O clone já existe, de antes da reentrega: o fetch tem de trazer o ramo
// criado depois.
func TestCloneExistenteRecebeORamoDaReentrega(t *testing.T) {
	origem, shas := origemFalsa(t)
	destino := filepath.Join(t.TempDir(), "clone")
	r, err := Preparar(destino, origem)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Posicionar(NomeRamo("trabalho1"), shas[1]); err != nil {
		t.Fatal(err)
	}

	reentrega := commitNoRamo(t, origem, shas[0], "reentrega", "correcao.html")
	r2, err := Preparar(destino, origem)
	if err != nil {
		t.Fatal(err)
	}
	if !r2.TemCommit(reentrega) {
		t.Fatal("o fetch não trouxe o commit que só existe em origin/reentrega")
	}
	head, err := r2.Posicionar(NomeRamo("trabalho1"), reentrega)
	if err != nil {
		t.Fatal(err)
	}
	if head != reentrega {
		t.Errorf("HEAD = %s, queria %s", head, reentrega)
	}
}
