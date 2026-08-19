package repo

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexkutzke/gitlab-classroom/internal/turma"
)

func TestSlug(t *testing.T) {
	casos := map[string]string{
		"João da Silva":  "joao-da-silva",
		"MARIA  ANTÔNIA": "maria-antonia",
		"":               "",
	}
	for entrada, quer := range casos {
		if got := Slug(entrada); got != quer {
			t.Errorf("Slug(%q) = %q, queria %q", entrada, got, quer)
		}
	}
}

func TestCaminho(t *testing.T) {
	a := turma.Aluno{GRR: "GRR20259001", Nome: "Ana Souza"}
	got := Caminho("/turma", "entregas", "html", a)
	if quer := "/turma/entregas/html/grr20259001-ana-souza"; got != quer {
		t.Errorf("Caminho = %q, queria %q", got, quer)
	}
}

func TestURLSSH(t *testing.T) {
	quer := "ssh://git@gitlab.com/grupo/projeto.git"
	if got := URLSSH("https://gitlab.com", "grupo/projeto"); got != quer {
		t.Errorf("URLSSH = %q, queria %q", got, quer)
	}
}

// origemFalsa cria um repositório local com dois commits, para servir de
// remoto nos testes. Nenhum teste desta aplicação usa rede.
func origemFalsa(t *testing.T) (dir string, shas []string) {
	t.Helper()
	dir = t.TempDir()
	git := func(args ...string) string {
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
	git("init", "--quiet", "--initial-branch=main")
	for _, arquivo := range []string{"index.html", "estilo.css"} {
		if err := os.WriteFile(filepath.Join(dir, arquivo), []byte("<!-- "+arquivo+" -->"), 0o644); err != nil {
			t.Fatal(err)
		}
		git("add", arquivo)
		git("commit", "--quiet", "-m", "adiciona "+arquivo)
		shas = append(shas, git("rev-parse", "HEAD"))
	}
	return dir, shas
}

func TestPrepararEPosicionar(t *testing.T) {
	origem, shas := origemFalsa(t)
	destino := filepath.Join(t.TempDir(), "grr20259001-ana")

	r, err := Preparar(destino, origem)
	if err != nil {
		t.Fatal(err)
	}
	if !Existe(destino) {
		t.Fatalf("clone não foi criado em %s", destino)
	}

	// Posiciona no primeiro commit, como faria uma entrega cujo prazo é
	// anterior ao último trabalho do aluno.
	head, err := r.Posicionar(NomeRamo("html"), shas[0])
	if err != nil {
		t.Fatal(err)
	}
	if head != shas[0] {
		t.Errorf("HEAD = %s, queria %s", head, shas[0])
	}

	ramo, err := rodar(destino, TempoLimite, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(ramo); got != "entrega/html" {
		t.Errorf("ramo = %q, queria entrega/html: o clone não pode ficar em HEAD destacado", got)
	}
}

func TestPrepararAtualizaOCloneExistente(t *testing.T) {
	origem, shas := origemFalsa(t)
	destino := filepath.Join(t.TempDir(), "clone")

	r, err := Preparar(destino, origem)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Posicionar(NomeRamo("html"), shas[0]); err != nil {
		t.Fatal(err)
	}

	// O aluno commita de novo depois da primeira coleta.
	if err := os.WriteFile(filepath.Join(origem, "script.js"), []byte("// js"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "script.js"}, {"commit", "--quiet", "-m", "adiciona js"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = origem
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Aluna", "GIT_AUTHOR_EMAIL=aluna@ufpr.br",
			"GIT_COMMITTER_NAME=Aluna", "GIT_COMMITTER_EMAIL=aluna@ufpr.br",
		)
		if saida, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", err, saida)
		}
	}
	novo, err := rodar(origem, TempoLimite, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	novo = strings.TrimSpace(novo)

	r2, err := Preparar(destino, origem)
	if err != nil {
		t.Fatal(err)
	}
	if !r2.TemCommit(novo) {
		t.Fatal("o fetch não trouxe o commit novo")
	}
	head, err := r2.Posicionar(NomeRamo("html"), novo)
	if err != nil {
		t.Fatal(err)
	}
	if head != novo {
		t.Errorf("HEAD = %s, queria %s", head, novo)
	}
}

func TestPosicionarSemCommitUsaORamoPadrao(t *testing.T) {
	origem, shas := origemFalsa(t)
	destino := filepath.Join(t.TempDir(), "clone")

	r, err := Preparar(destino, origem)
	if err != nil {
		t.Fatal(err)
	}
	head, err := r.Posicionar(NomeRamo("html"), "")
	if err != nil {
		t.Fatal(err)
	}
	if head != shas[len(shas)-1] {
		t.Errorf("HEAD = %s, queria o topo do ramo padrão %s", head, shas[len(shas)-1])
	}
}

func TestSincronizarEmLote(t *testing.T) {
	origem, shas := origemFalsa(t)
	base := t.TempDir()

	var alvos []Alvo
	for _, grr := range []string{"grr1", "grr2", "grr3"} {
		alvos = append(alvos, Alvo{
			Exercicio: "html", GRR: grr, Nome: grr,
			Projeto: "grupo/" + grr, URL: origem, Commit: shas[0],
			Dir: filepath.Join(base, grr),
		})
	}

	vistos := 0
	res := Sincronizar(alvos, "https://gitlab.com", 3, func(feito, total int, a Alvo) {
		vistos++
	})
	if len(res) != 3 || vistos != 3 {
		t.Fatalf("esperava 3 resultados e 3 avisos de progresso, veio %d e %d", len(res), vistos)
	}
	for _, r := range res {
		if r.Erro != nil {
			t.Errorf("%s falhou: %v", r.GRR, r.Erro)
			continue
		}
		if !r.Novo {
			t.Errorf("%s deveria ser clone novo", r.GRR)
		}
		if r.HEAD != shas[0] {
			t.Errorf("%s ficou em %s, queria %s", r.GRR, r.HEAD, shas[0])
		}
	}

	// Segunda passada: os clones já existem e devem ser atualizados, não
	// recriados.
	res = Sincronizar(alvos, "https://gitlab.com", 3, nil)
	for _, r := range res {
		if r.Erro != nil {
			t.Errorf("%s falhou na segunda passada: %v", r.GRR, r.Erro)
		}
		if r.Novo {
			t.Errorf("%s foi tratado como clone novo na segunda passada", r.GRR)
		}
	}
}

func TestSincronizarRegistraFalhaSemDerrubarOsDemais(t *testing.T) {
	origem, shas := origemFalsa(t)
	base := t.TempDir()

	alvos := []Alvo{
		{Exercicio: "html", GRR: "grr1", Projeto: "grupo/grr1", URL: origem,
			Commit: shas[0], Dir: filepath.Join(base, "grr1")},
		{Exercicio: "html", GRR: "grr2", Projeto: "grupo/grr2",
			URL: filepath.Join(base, "nao-existe"), Dir: filepath.Join(base, "grr2")},
	}
	res := Sincronizar(alvos, "https://gitlab.com", 2, nil)

	if res[0].Erro != nil {
		t.Errorf("o alvo bom não deveria falhar: %v", res[0].Erro)
	}
	if res[1].Erro == nil {
		t.Error("o alvo inexistente deveria registrar erro")
	}
}
