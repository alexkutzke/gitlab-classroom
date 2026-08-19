package verificacao

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alexkutzke/gitlab-classroom/internal/turma"
)

// Os testes rodam sem contêiner, com SemSandbox, para não depender de imagem
// baixada nem de runtime instalado na máquina de quem roda a suíte. A montagem
// do contêiner é conferida à parte, em TestArgsContainer.

func exercicioCom(comando string) turma.Exercicio {
	return turma.Exercicio{
		ID: "html", Repo: "ds122-html-assignment",
		Prazo: turma.NovaData(2026, time.September, 5), Verificacao: comando,
	}
}

func cloneFalso(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<h1>oi</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func rodar(t *testing.T, comando string, o Opcoes, alvos ...Alvo) []turma.Verificacao {
	t.Helper()
	o.Exercicio = exercicioCom(comando)
	o.SemSandbox = true
	if o.TempoLimite == 0 {
		o.TempoLimite = 10 * time.Second
	}
	res, err := Executar(alvos, o, 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestSuiteQuePassa(t *testing.T) {
	dir := cloneFalso(t)
	res := rodar(t, "test -f index.html", Opcoes{}, Alvo{GRR: "GRR1", Dir: dir, Commit: "abc"})

	if res[0].Situacao != turma.Aprovado {
		t.Errorf("situação = %v, queria aprovado (%s)", res[0].Situacao, res[0].Detalhe)
	}
	if res[0].Commit != "abc" {
		t.Errorf("o commit verificado deveria ficar registrado: %+v", res[0])
	}
}

func TestSuiteQueFalha(t *testing.T) {
	dir := cloneFalso(t)
	res := rodar(t, "test -f estilo.css", Opcoes{}, Alvo{GRR: "GRR1", Dir: dir})

	if res[0].Situacao != turma.Reprovado {
		t.Errorf("situação = %v, queria reprovado", res[0].Situacao)
	}
	if !strings.Contains(res[0].Detalhe, "código de saída") {
		t.Errorf("detalhe = %q, queria o código de saída", res[0].Detalhe)
	}
}

func TestLinhaDeResultadoTemPrioridadeSobreOCodigoDeSaida(t *testing.T) {
	dir := cloneFalso(t)
	res := rodar(t, "echo 'RESULTADO: 7/10'", Opcoes{}, Alvo{GRR: "GRR1", Dir: dir})

	if res[0].Aprovados != 7 || res[0].Total != 10 {
		t.Errorf("contagem = %d/%d, queria 7/10", res[0].Aprovados, res[0].Total)
	}
	if res[0].Situacao != turma.Reprovado {
		t.Errorf("situação = %v: a suíte disse que faltaram casos, mesmo saindo com zero",
			res[0].Situacao)
	}
	if res[0].Resumo() != "reprovado (7/10)" {
		t.Errorf("resumo = %q", res[0].Resumo())
	}
}

func TestTodosOsCasosPassando(t *testing.T) {
	dir := cloneFalso(t)
	res := rodar(t, "echo 'RESULTADO: 10/10'", Opcoes{}, Alvo{GRR: "GRR1", Dir: dir})

	if res[0].Situacao != turma.Aprovado || res[0].Aprovados != 10 {
		t.Errorf("resultado = %+v, queria aprovado 10/10", res[0])
	}
}

func TestUltimaLinhaDeResultadoEhAQueVale(t *testing.T) {
	dir := cloneFalso(t)
	res := rodar(t, "echo 'RESULTADO: 1/10'; echo 'RESULTADO: 10/10'",
		Opcoes{}, Alvo{GRR: "GRR1", Dir: dir})

	if res[0].Aprovados != 10 {
		t.Errorf("aprovados = %d, queria a última contagem impressa", res[0].Aprovados)
	}
}

func TestCloneAusente(t *testing.T) {
	res := rodar(t, "true", Opcoes{}, Alvo{GRR: "GRR1", Dir: filepath.Join(t.TempDir(), "nao-existe")})

	if res[0].Situacao != turma.SemClone {
		t.Errorf("situação = %v, queria sem_clone", res[0].Situacao)
	}
}

func TestExercicioSemSuite(t *testing.T) {
	res, err := Executar(
		[]Alvo{{GRR: "GRR1", Dir: cloneFalso(t)}},
		Opcoes{Exercicio: exercicioCom("")}, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res[0].Situacao != turma.SemSuite {
		t.Errorf("situação = %v, queria sem_suite", res[0].Situacao)
	}
}

func TestTempoEsgotado(t *testing.T) {
	dir := cloneFalso(t)
	res := rodar(t, "sleep 5", Opcoes{TempoLimite: 200 * time.Millisecond},
		Alvo{GRR: "GRR1", Dir: dir})

	if res[0].Situacao != turma.ErroVerificacao {
		t.Errorf("situação = %v, queria erro", res[0].Situacao)
	}
	if !strings.Contains(res[0].Detalhe, "tempo esgotado") {
		t.Errorf("detalhe = %q", res[0].Detalhe)
	}
}

func TestLogGuardaASaidaCompleta(t *testing.T) {
	dir := cloneFalso(t)
	logs := filepath.Join(t.TempDir(), "logs")
	rodar(t, "echo linha-de-saida", Opcoes{PastaLogs: logs},
		Alvo{GRR: "GRR20259001", Nome: "Ana Souza", Dir: dir})

	conteudo, err := os.ReadFile(filepath.Join(logs, "grr20259001.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(conteudo), "linha-de-saida") {
		t.Errorf("o log não tem a saída da suíte:\n%s", conteudo)
	}
	if !strings.Contains(string(conteudo), "Ana Souza") {
		t.Errorf("o log deveria identificar o aluno:\n%s", conteudo)
	}
}

func TestVariosAlvos(t *testing.T) {
	a, b := cloneFalso(t), cloneFalso(t)
	res := rodar(t, "test -f index.html", Opcoes{},
		Alvo{GRR: "GRR1", Dir: a}, Alvo{GRR: "GRR2", Dir: b})

	if len(res) != 2 {
		t.Fatalf("esperava 2 resultados, veio %d", len(res))
	}
	for _, v := range res {
		if v.Situacao != turma.Aprovado {
			t.Errorf("%s: %v", v.GRR, v.Situacao)
		}
	}
}

func TestArgsContainer(t *testing.T) {
	o := Opcoes{Exercicio: exercicioCom("./verifica.sh"), Imagem: "alpine:3.20"}
	o.padroes()
	args := strings.Join(argsContainer(Alvo{Dir: "/tmp/clone"}, o), " ")

	for _, quer := range []string{
		"--network=none",
		"--security-opt=no-new-privileges",
		"--memory=512m",
		"--pids-limit=256",
		"--volume=/tmp/clone:/repo:ro,z",
		"--workdir=/repo",
		"alpine:3.20",
		"./verifica.sh",
	} {
		if !strings.Contains(args, quer) {
			t.Errorf("faltou %q em: %s", quer, args)
		}
	}
}

func TestArgsContainerComEscrita(t *testing.T) {
	o := Opcoes{Exercicio: exercicioCom("./verifica.sh"), Imagem: "alpine:3.20", Escrita: true}
	o.padroes()
	args := strings.Join(argsContainer(Alvo{Dir: "/tmp/clone"}, o), " ")

	if !strings.Contains(args, "--volume=/tmp/clone:/repo:rw,z") {
		t.Errorf("montagem com escrita ausente: %s", args)
	}
}

func TestVerificacaoDesatualizada(t *testing.T) {
	v := turma.Verificacao{Commit: "abc"}
	if !v.Desatualizada("def") {
		t.Error("commit diferente do da entrega deveria marcar a verificação como velha")
	}
	if v.Desatualizada("abc") {
		t.Error("o mesmo commit não pode estar desatualizado")
	}
	if (turma.Verificacao{}).Desatualizada("abc") {
		t.Error("verificação sem commit não tem como estar desatualizada")
	}
}
