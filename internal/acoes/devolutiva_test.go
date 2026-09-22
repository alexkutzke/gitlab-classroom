package acoes

import (
	"context"
	"strings"
	"testing"
	"time"

	gl "github.com/alexkutzke/gitlab-classroom/internal/gitlab"
	"github.com/alexkutzke/gitlab-classroom/internal/turma"
)

// clienteIssues dubla a parte da API que publica devolutiva. Nenhum teste
// desta aplicação toca o gitlab.com.
type clienteIssues struct {
	// issues são as já abertas em cada projeto, por caminho completo.
	issues map[string][]gl.Issue
	// criadas e comentadas registram as escritas, que é o que os testes
	// conferem.
	criadas   []criada
	comentada []comentada
	proximo   int64
}

type criada struct {
	Projeto, Titulo, Corpo string
}

type comentada struct {
	Projeto string
	IID     int64
	Corpo   string
}

func (c *clienteIssues) IssuesDoProjeto(projeto string) ([]gl.Issue, error) {
	return c.issues[projeto], nil
}

func (c *clienteIssues) CriarIssue(projeto, titulo, corpo string) (gl.Issue, error) {
	c.criadas = append(c.criadas, criada{projeto, titulo, corpo})
	c.proximo++
	return gl.Issue{
		IID:    c.proximo,
		Titulo: titulo,
		URL:    "https://gitlab.com/" + projeto + "/-/issues/1",
	}, nil
}

func (c *clienteIssues) ComentarIssue(projeto string, iid int64, corpo string) error {
	c.comentada = append(c.comentada, comentada{projeto, iid, corpo})
	return nil
}

func (c *clienteIssues) Renovar()                                     {}
func (c *clienteIssues) UsuarioExiste(string) (bool, error)           { return true, nil }
func (c *clienteIssues) Grupo(string) (*gl.Grupo, error)              { return nil, nil }
func (c *clienteIssues) GruposComAcesso(string) ([]gl.Grupo, error)   { return nil, nil }
func (c *clienteIssues) Eu() (string, error)                          { return "alexkutzke", nil }
func (c *clienteIssues) MembroDoGrupo(string) (bool, error)           { return true, nil }
func (c *clienteIssues) ProjetosDoGrupo(string) ([]gl.Projeto, error) { return nil, nil }
func (c *clienteIssues) Ramos(string) ([]string, error)               { return nil, nil }
func (c *clienteIssues) Membros(string) ([]gl.Membro, error)          { return nil, nil }

func (c *clienteIssues) Commits(string, string, bool) ([]gl.Commit, error) { return nil, nil }

const (
	forkAna  = "ds122-2026-2-n-grr20259001/ds122-html-assignment"
	forkBeto = "ds122-2026-2-n-grr20259002/ds122-html-assignment"
)

// turmaExemplo monta uma turma com três alunos: Ana entregou e foi corrigida,
// Beto entregou e ainda não tem nota, Carla não tem fork.
func turmaExemplo() *turma.Turma {
	e := turma.Exercicio{
		ID: "html", Repo: "ds122-html-assignment", Titulo: "HTML e CSS",
		Prazo: turma.NovaData(2026, time.September, 5), Peso: 1,
	}
	t := &turma.Turma{
		Config:     turma.Config{Codigo: "DS122", Semestre: "2026-02", Turno: "n"},
		Exercicios: []turma.Exercicio{e},
		Alunos: []turma.Aluno{
			{GRR: "GRR20259001", Nome: "ANA SOUZA", Usuario: "grr20259001"},
			{GRR: "GRR20259002", Nome: "BETO LIMA", Usuario: "grr20259002"},
			{GRR: "GRR20259003", Nome: "CARLA DIAS", Usuario: "grr20259003"},
		},
		Entregas: []turma.Entrega{
			{Exercicio: "html", GRR: "GRR20259001", Situacao: turma.Entregue,
				Projeto: forkAna, Commit: "abc123def456"},
			{Exercicio: "html", GRR: "GRR20259002", Situacao: turma.Entregue,
				Projeto: forkBeto, Commit: "999888777666"},
			{Exercicio: "html", GRR: "GRR20259003", Situacao: turma.GrupoInvisivel,
				Projeto: "ds122-2026-2-n-grr20259003"},
		},
		Notas: []turma.Nota{
			{Exercicio: "html", GRR: "GRR20259001", Valor: 80,
				Comentario: "faltou o label nos campos de contato.html"},
			{Exercicio: "html", GRR: "GRR20259003", Valor: 0, Comentario: "não entregou"},
		},
	}
	t.Ordenar()
	return t
}

func exercicioHTML(t *testing.T, tu *turma.Turma) []turma.Exercicio {
	t.Helper()
	e, ok := tu.Exercicio("html")
	if !ok {
		t.Fatal("exercício html não encontrado na turma de exemplo")
	}
	return []turma.Exercicio{*e}
}

func TestDevolutivasEnsaioNaoPublica(t *testing.T) {
	tu := turmaExemplo()
	cli := &clienteIssues{}

	res, err := Devolutivas(context.Background(), tu, cli, exercicioHTML(t, tu),
		OpcoesDevolutiva{}, nil)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(cli.criadas) != 0 || len(cli.comentada) != 0 {
		t.Fatalf("o ensaio escreveu no GitLab: %d issues, %d comentários",
			len(cli.criadas), len(cli.comentada))
	}
	if res.Publicadas != 1 {
		t.Fatalf("issues a publicar = %d, esperado 1", res.Publicadas)
	}
	if len(tu.Devolutivas) != 0 {
		t.Fatalf("o ensaio registrou %d devolutiva(s)", len(tu.Devolutivas))
	}
}

func TestDevolutivasPublicaERegistra(t *testing.T) {
	tu := turmaExemplo()
	cli := &clienteIssues{}

	res, err := Devolutivas(context.Background(), tu, cli, exercicioHTML(t, tu),
		OpcoesDevolutiva{Aplicar: true, Prazo: turma.NovaData(2026, time.October, 5)}, nil)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(cli.criadas) != 1 {
		t.Fatalf("issues criadas = %d, esperado 1", len(cli.criadas))
	}
	nova := cli.criadas[0]
	if nova.Projeto != forkAna {
		t.Errorf("issue aberta em %s, esperado %s", nova.Projeto, forkAna)
	}
	if nova.Titulo != "Devolutiva: HTML e CSS" {
		t.Errorf("título = %q", nova.Titulo)
	}
	for _, trecho := range []string{
		"@grr20259001", "faltou o label", "Commit avaliado: abc123de", "Comentários até 05/10/2026",
	} {
		if !strings.Contains(nova.Corpo, trecho) {
			t.Errorf("corpo sem %q:\n%s", trecho, nova.Corpo)
		}
	}
	if strings.Contains(nova.Corpo, "80") {
		t.Errorf("a nota vazou para o corpo da issue:\n%s", nova.Corpo)
	}

	d, ok := tu.Devolutiva("html", "GRR20259001")
	if !ok {
		t.Fatal("a publicação não foi registrada na turma")
	}
	if d.Issue != 1 || d.Projeto != forkAna {
		t.Errorf("registro = issue %d em %s", d.Issue, d.Projeto)
	}
	if d.Hash != turma.HashComentario("faltou o label nos campos de contato.html") {
		t.Errorf("hash gravado %q não corresponde ao comentário publicado", d.Hash)
	}
	if res.Publicadas != 1 {
		t.Errorf("publicadas = %d", res.Publicadas)
	}
}

func TestDevolutivasSegundaRodadaNaoRepublica(t *testing.T) {
	tu := turmaExemplo()
	cli := &clienteIssues{}
	o := OpcoesDevolutiva{Aplicar: true}

	if _, err := Devolutivas(context.Background(), tu, cli, exercicioHTML(t, tu), o, nil); err != nil {
		t.Fatalf("primeira rodada: %v", err)
	}
	res, err := Devolutivas(context.Background(), tu, cli, exercicioHTML(t, tu), o, nil)
	if err != nil {
		t.Fatalf("segunda rodada: %v", err)
	}
	if len(cli.criadas) != 1 || len(cli.comentada) != 0 {
		t.Fatalf("a segunda rodada publicou de novo: %d issues, %d comentários",
			len(cli.criadas), len(cli.comentada))
	}
	if res.Fora[MotivoJaPublicada] != 1 {
		t.Errorf("motivo %q = %d, esperado 1", MotivoJaPublicada, res.Fora[MotivoJaPublicada])
	}
}

func TestDevolutivasComentarioAlteradoSoSaiComRefazer(t *testing.T) {
	tu := turmaExemplo()
	cli := &clienteIssues{}
	if _, err := Devolutivas(context.Background(), tu, cli, exercicioHTML(t, tu),
		OpcoesDevolutiva{Aplicar: true}, nil); err != nil {
		t.Fatalf("primeira rodada: %v", err)
	}

	n, _ := tu.Nota("html", "GRR20259001")
	n.Comentario = "faltou o label nos três campos e o README está com o texto de fábrica"

	res, err := Devolutivas(context.Background(), tu, cli, exercicioHTML(t, tu),
		OpcoesDevolutiva{Aplicar: true}, nil)
	if err != nil {
		t.Fatalf("rodada sem refazer: %v", err)
	}
	if res.Fora[MotivoDesatualizada] != 1 {
		t.Fatalf("motivo %q = %d, esperado 1", MotivoDesatualizada, res.Fora[MotivoDesatualizada])
	}
	if len(cli.comentada) != 0 {
		t.Fatalf("republicou sem --refazer")
	}

	res, err = Devolutivas(context.Background(), tu, cli, exercicioHTML(t, tu),
		OpcoesDevolutiva{Aplicar: true, Refazer: true}, nil)
	if err != nil {
		t.Fatalf("rodada com refazer: %v", err)
	}
	if res.Comentadas != 1 {
		t.Errorf("comentadas = %d, esperado 1", res.Comentadas)
	}
	if len(cli.criadas) != 1 {
		t.Errorf("--refazer abriu issue nova: %d ao todo", len(cli.criadas))
	}
	if len(cli.comentada) != 1 {
		t.Fatalf("comentários na issue = %d, esperado 1", len(cli.comentada))
	}
	c := cli.comentada[0]
	if c.IID != 1 || c.Projeto != forkAna {
		t.Errorf("comentário foi para %s #%d", c.Projeto, c.IID)
	}
	if !strings.Contains(c.Corpo, "texto de fábrica") {
		t.Errorf("o comentário não traz a devolutiva nova:\n%s", c.Corpo)
	}

	d, _ := tu.Devolutiva("html", "GRR20259001")
	if d.Desatualizada(n.Comentario) {
		t.Error("o hash não acompanhou a republicação")
	}
}

func TestDevolutivasMotivosDeExclusao(t *testing.T) {
	tu := turmaExemplo()
	// Beto tem fork, mas a nota veio sem comentário.
	tu.RegistrarNota(turma.Nota{Exercicio: "html", GRR: "GRR20259002", Valor: 70})
	// Ninguém mais tem nota: o caso "sem nota" precisa de um aluno a mais.
	tu.Alunos = append(tu.Alunos, turma.Aluno{
		GRR: "GRR20259004", Nome: "DANI ROCHA", Usuario: "grr20259004",
	})
	tu.Entregas = append(tu.Entregas, turma.Entrega{
		Exercicio: "html", GRR: "GRR20259004", Situacao: turma.Entregue,
		Projeto: "ds122-2026-2-n-grr20259004/ds122-html-assignment", Commit: "aaa",
	})
	tu.Ordenar()

	res, err := Devolutivas(context.Background(), tu, &clienteIssues{}, exercicioHTML(t, tu),
		OpcoesDevolutiva{}, nil)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	for motivo, esperado := range map[string]int{
		MotivoSemNota:       1, // Dani
		MotivoSemComentario: 1, // Beto
		MotivoSemFork:       1, // Carla, que nem grupo visível tem
	} {
		if res.Fora[motivo] != esperado {
			t.Errorf("motivo %q = %d, esperado %d", motivo, res.Fora[motivo], esperado)
		}
	}
}

func TestDevolutivasReconheceIssueExistente(t *testing.T) {
	tu := turmaExemplo()
	cli := &clienteIssues{issues: map[string][]gl.Issue{
		forkAna: {{IID: 7, Titulo: "Devolutiva: HTML e CSS",
			URL: "https://gitlab.com/" + forkAna + "/-/issues/7"}},
	}}

	res, err := Devolutivas(context.Background(), tu, cli, exercicioHTML(t, tu),
		OpcoesDevolutiva{Aplicar: true}, nil)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(cli.criadas) != 0 || len(cli.comentada) != 0 {
		t.Fatalf("publicou sobre issue já existente")
	}
	if res.Reconhecidas != 1 {
		t.Errorf("reconhecidas = %d, esperado 1", res.Reconhecidas)
	}
	d, ok := tu.Devolutiva("html", "GRR20259001")
	if !ok || d.Issue != 7 {
		t.Fatalf("linha reconstruída = %+v", d)
	}
}

func TestDevolutivasEntregaCompartilhadaGeraUmaIssue(t *testing.T) {
	tu := turmaExemplo()
	// Beto entregou dentro do fork da Ana, e a nota foi propagada para os dois.
	beto, _ := tu.Entrega("html", "GRR20259002")
	beto.Projeto, beto.Commit = forkAna, "abc123def456"
	tu.RegistrarVinculo(turma.Vinculo{
		Exercicio: "html", GRR: "GRR20259002", Dono: "GRR20259001",
		Origem: turma.VinculoDescoberto,
	})
	tu.RegistrarNota(turma.Nota{Exercicio: "html", GRR: "GRR20259002", Valor: 80,
		Comentario: "faltou o label nos campos de contato.html"})

	cli := &clienteIssues{}
	res, err := Devolutivas(context.Background(), tu, cli, exercicioHTML(t, tu),
		OpcoesDevolutiva{Aplicar: true}, nil)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(cli.criadas) != 1 {
		t.Fatalf("issues criadas = %d, esperado 1", len(cli.criadas))
	}
	if res.Publicadas != 1 {
		t.Errorf("publicadas = %d, esperado 1", res.Publicadas)
	}
	corpo := cli.criadas[0].Corpo
	for _, u := range []string{"@grr20259001", "@grr20259002"} {
		if !strings.Contains(corpo, u) {
			t.Errorf("corpo sem a menção a %s:\n%s", u, corpo)
		}
	}
	for _, grr := range []string{"GRR20259001", "GRR20259002"} {
		d, ok := tu.Devolutiva("html", grr)
		if !ok {
			t.Fatalf("%s ficou sem registro da devolutiva", grr)
		}
		if d.Issue != 1 || d.Projeto != forkAna {
			t.Errorf("%s registrado em %s #%d", grr, d.Projeto, d.Issue)
		}
	}
}
