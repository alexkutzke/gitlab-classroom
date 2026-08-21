package coleta

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	gl "github.com/alexkutzke/gitlab-classroom/internal/gitlab"
	"github.com/alexkutzke/gitlab-classroom/internal/turma"
)

// clienteFalso dubla o GitLab. Nenhum teste desta aplicação usa rede.
type clienteFalso struct {
	usuarios []string
	grupos   map[string]gl.Grupo // por caminho
	meus     []gl.Grupo          // grupos com o professor associado
	projetos map[string][]gl.Projeto
	commits  map[string][]gl.Commit // por caminho completo do projeto
	membros  map[string][]gl.Membro // membros por caminho completo do fork
	erro     error
	// erroMembros imita a consulta de membros indisponível, que é a falha que
	// desliga a descoberta de entregas em dupla.
	erroMembros error
	// renovacoes conta os pedidos de descarte do cache.
	renovacoes int
}

func (c *clienteFalso) Renovar() { c.renovacoes++ }

func (c *clienteFalso) UsuarioExiste(login string) (bool, error) {
	for _, u := range c.usuarios {
		if u == login {
			return true, nil
		}
	}
	return false, nil
}

func (c *clienteFalso) Grupo(caminho string) (*gl.Grupo, error) {
	g, ok := c.grupos[caminho]
	if !ok {
		return nil, nil
	}
	return &g, nil
}

func (c *clienteFalso) Eu() (string, error) { return "alexkutzke", nil }

func (c *clienteFalso) MembroDoGrupo(caminho string) (bool, error) {
	for _, g := range c.meus {
		if strings.EqualFold(g.Caminho, caminho) {
			return true, nil
		}
	}
	return false, nil
}

// GruposComAcesso imita o filtro do GitLab: devolve os grupos do professor
// cujo caminho contém a busca.
func (c *clienteFalso) GruposComAcesso(busca string) ([]gl.Grupo, error) {
	if busca == "" {
		return c.meus, nil
	}
	var out []gl.Grupo
	for _, g := range c.meus {
		if strings.Contains(strings.ToLower(g.Caminho), strings.ToLower(busca)) {
			out = append(out, g)
		}
	}
	return out, nil
}

func (c *clienteFalso) ProjetosDoGrupo(grupo string) ([]gl.Projeto, error) {
	if c.erro != nil {
		return nil, c.erro
	}
	return c.projetos[grupo], nil
}

func (c *clienteFalso) Commits(projeto, ramo string, todos bool) ([]gl.Commit, error) {
	return c.commits[projeto], nil
}

func (c *clienteFalso) Membros(projeto string) ([]gl.Membro, error) {
	if c.erroMembros != nil {
		return nil, c.erroMembros
	}
	return c.membros[projeto], nil
}

const grupoAna = "ds122-2026-2-n-grr20259001"

func configExemplo() turma.Config {
	c := turma.Config{
		Codigo: "DS122", Semestre: "2026-02", Turno: "n",
		NamespaceModelos: "ds122-alexkutzke",
		PadraoGrupo:      "{codigo}-{ano}-{periodo}-{turno}-{grr}",
		Paralelismo:      2,
	}
	c.Padroes()
	return c
}

func exercicioExemplo() turma.Exercicio {
	return turma.Exercicio{
		ID: "html", Repo: "ds122-html-assignment",
		Prazo: turma.NovaData(2026, time.September, 5), Peso: 1,
		Situacao: turma.ExercicioAtivo,
	}
}

func ana() turma.Aluno {
	return turma.Aluno{GRR: "GRR20259001", Nome: "Ana Souza", Situacao: turma.Ativo}
}

// baseFalsa monta o cenário comum: aluno com grupo em ordem e o modelo com
// dois commits herdados pelo fork.
func baseFalsa() *clienteFalso {
	g := gl.Grupo{ID: 1, Caminho: grupoAna, Membro: true}
	return &clienteFalso{
		usuarios: []string{"grr20259001"},
		grupos:   map[string]gl.Grupo{grupoAna: g},
		meus:     []gl.Grupo{g},
		projetos: map[string][]gl.Projeto{
			grupoAna: {{
				ID: 10, Caminho: "ds122-html-assignment",
				Completo:   grupoAna + "/ds122-html-assignment",
				RamoPadrao: "main",
				ForkDe:     "ds122-alexkutzke/ds122-html-assignment",
			}},
		},
		commits: map[string][]gl.Commit{
			"ds122-alexkutzke/ds122-html-assignment": {
				{SHA: "modelo1", Data: time.Date(2026, 8, 1, 10, 0, 0, 0, time.Local)},
				{SHA: "modelo2", Data: time.Date(2026, 8, 2, 10, 0, 0, 0, time.Local)},
			},
		},
	}
}

func coletarUm(t *testing.T, c *clienteFalso) turma.Entrega {
	t.Helper()
	col := &Coletor{Cliente: c, Config: configExemplo()}
	res, err := col.Coletar(context.Background(), []turma.Aluno{ana()}, []turma.Exercicio{exercicioExemplo()})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Entregas) != 1 {
		t.Fatalf("esperava 1 entrega, veio %d", len(res.Entregas))
	}
	return res.Entregas[0]
}

func TestEntregaNoPrazo(t *testing.T) {
	c := baseFalsa()
	fork := grupoAna + "/ds122-html-assignment"
	c.commits[fork] = []gl.Commit{
		{SHA: "aluno2", Data: time.Date(2026, 9, 5, 22, 30, 0, 0, time.Local)},
		{SHA: "aluno1", Data: time.Date(2026, 9, 3, 15, 0, 0, 0, time.Local)},
		{SHA: "modelo2", Data: time.Date(2026, 8, 2, 10, 0, 0, 0, time.Local)},
		{SHA: "modelo1", Data: time.Date(2026, 8, 1, 10, 0, 0, 0, time.Local)},
	}
	e := coletarUm(t, c)

	if e.Situacao != turma.Entregue {
		t.Errorf("situação = %v, queria entregue", e.Situacao)
	}
	if e.Commit != "aluno2" {
		t.Errorf("commit avaliado = %q, queria o mais recente até o prazo", e.Commit)
	}
	if e.Commits != 2 {
		t.Errorf("commits do aluno = %d, queria 2 (os do modelo não contam)", e.Commits)
	}
	if e.AtrasoDias != 0 {
		t.Errorf("atraso = %d, queria 0", e.AtrasoDias)
	}
}

func TestEntregaComCommitDepoisDoPrazoUsaOCommitDoPrazo(t *testing.T) {
	c := baseFalsa()
	fork := grupoAna + "/ds122-html-assignment"
	c.commits[fork] = []gl.Commit{
		{SHA: "depois", Data: time.Date(2026, 9, 8, 9, 0, 0, 0, time.Local)},
		{SHA: "noprazo", Data: time.Date(2026, 9, 4, 9, 0, 0, 0, time.Local)},
		{SHA: "modelo1", Data: time.Date(2026, 8, 1, 10, 0, 0, 0, time.Local)},
	}
	e := coletarUm(t, c)

	if e.Situacao != turma.Entregue || e.Commit != "noprazo" {
		t.Errorf("deveria avaliar o commit do prazo: %+v", e)
	}
	if e.UltimoCommit != "depois" || e.AtrasoDias != 3 {
		t.Errorf("o trabalho posterior ao prazo deveria ficar registrado: %+v", e)
	}
}

func TestSoCommitsDepoisDoPrazo(t *testing.T) {
	c := baseFalsa()
	fork := grupoAna + "/ds122-html-assignment"
	c.commits[fork] = []gl.Commit{
		{SHA: "atrasado", Data: time.Date(2026, 9, 7, 9, 0, 0, 0, time.Local)},
		{SHA: "modelo1", Data: time.Date(2026, 8, 1, 10, 0, 0, 0, time.Local)},
	}
	e := coletarUm(t, c)

	if e.Situacao != turma.SemCommitNoPrazo {
		t.Errorf("situação = %v, queria sem_commit_no_prazo", e.Situacao)
	}
	if e.AtrasoDias != 2 {
		t.Errorf("atraso = %d, queria 2", e.AtrasoDias)
	}
	if e.Commit != "" {
		t.Errorf("não deveria haver commit avaliado: %q", e.Commit)
	}
}

func TestForkSemCommitDoAluno(t *testing.T) {
	c := baseFalsa()
	fork := grupoAna + "/ds122-html-assignment"
	c.commits[fork] = []gl.Commit{
		{SHA: "modelo2", Data: time.Date(2026, 8, 2, 10, 0, 0, 0, time.Local)},
		{SHA: "modelo1", Data: time.Date(2026, 8, 1, 10, 0, 0, 0, time.Local)},
	}
	if e := coletarUm(t, c); e.Situacao != turma.ForkSemCommit {
		t.Errorf("situação = %v, queria fork_sem_commit", e.Situacao)
	}
}

func TestForkNaoEncontrado(t *testing.T) {
	c := baseFalsa()
	c.projetos[grupoAna] = nil
	if e := coletarUm(t, c); e.Situacao != turma.SemFork {
		t.Errorf("situação = %v, queria sem_fork", e.Situacao)
	}
}

func TestGrupoComNomeForaDoPadraoAindaEncontraOFork(t *testing.T) {
	outro := "ds122-2026-noturno-grr20259001"
	g := gl.Grupo{ID: 2, Caminho: outro, Membro: true}
	c := baseFalsa()
	c.grupos = map[string]gl.Grupo{outro: g}
	c.meus = []gl.Grupo{g}
	c.projetos = map[string][]gl.Projeto{outro: {{
		ID: 11, Caminho: "ds122-html-assignment", Completo: outro + "/ds122-html-assignment",
		RamoPadrao: "main",
	}}}
	c.commits[outro+"/ds122-html-assignment"] = []gl.Commit{
		{SHA: "aluno1", Data: time.Date(2026, 9, 1, 10, 0, 0, 0, time.Local)},
	}

	col := &Coletor{Cliente: c, Config: configExemplo()}
	res, err := col.Coletar(context.Background(), []turma.Aluno{ana()}, []turma.Exercicio{exercicioExemplo()})
	if err != nil {
		t.Fatal(err)
	}
	if res.Alunos[0].SituacaoConta != turma.ContaGrupoDivergente {
		t.Errorf("conta = %v, queria grupo_divergente", res.Alunos[0].SituacaoConta)
	}
	if res.Entregas[0].Situacao != turma.Entregue {
		t.Errorf("a entrega no grupo fora do padrão deveria ser encontrada: %+v", res.Entregas[0])
	}
}

// Falta de associação vira aviso de cadastro, e não descarte da entrega: o
// grupo do aluno costuma continuar legível quando o repositório é fork de um
// modelo da disciplina.
func TestGrupoLegivelSemOProfessorAssociadoAindaTemAEntregaColetada(t *testing.T) {
	c := baseFalsa()
	c.grupos[grupoAna] = gl.Grupo{ID: 1, Caminho: grupoAna}
	c.meus = nil
	c.commits[grupoAna+"/ds122-html-assignment"] = []gl.Commit{
		{SHA: "aluno1", Data: time.Date(2026, 9, 1, 10, 0, 0, 0, time.Local)},
	}

	col := &Coletor{Cliente: c, Config: configExemplo()}
	res, err := col.Coletar(context.Background(), []turma.Aluno{ana()}, []turma.Exercicio{exercicioExemplo()})
	if err != nil {
		t.Fatal(err)
	}
	if res.Alunos[0].SituacaoConta != turma.ContaSemAcesso {
		t.Errorf("conta = %v, queria sem_acesso", res.Alunos[0].SituacaoConta)
	}
	if res.Entregas[0].Situacao != turma.Entregue {
		t.Errorf("situação = %v, queria entregue", res.Entregas[0].Situacao)
	}
}

func TestGrupoSemAssociacaoEIlegivelFicaComoSemAcesso(t *testing.T) {
	c := baseFalsa()
	c.grupos[grupoAna] = gl.Grupo{ID: 1, Caminho: grupoAna}
	c.meus = nil
	c.projetos = nil // o grupo existe no cadastro do GitLab, mas não abre

	col := &Coletor{Cliente: c, Config: configExemplo()}
	res, err := col.Coletar(context.Background(), []turma.Aluno{ana()}, []turma.Exercicio{exercicioExemplo()})
	if err != nil {
		t.Fatal(err)
	}
	if res.Entregas[0].Situacao != turma.SemAcesso {
		t.Errorf("situação = %v, queria sem_acesso", res.Entregas[0].Situacao)
	}
	if res.Entregas[0].Projeto != grupoAna {
		t.Errorf("projeto = %q, queria o grupo para o aviso apontar para algum lugar", res.Entregas[0].Projeto)
	}
}

func TestSemContaNoGitLab(t *testing.T) {
	c := baseFalsa()
	c.grupos = nil
	c.meus = nil
	c.usuarios = nil

	col := &Coletor{Cliente: c, Config: configExemplo()}
	res, err := col.Coletar(context.Background(), []turma.Aluno{ana()}, []turma.Exercicio{exercicioExemplo()})
	if err != nil {
		t.Fatal(err)
	}
	if res.Entregas[0].Situacao != turma.SemConta {
		t.Errorf("situação = %v, queria sem_conta", res.Entregas[0].Situacao)
	}
}

func TestGrupoInvisivelQuandoAContaExisteMasOGrupoNao(t *testing.T) {
	c := baseFalsa()
	c.grupos = nil
	c.meus = nil

	col := &Coletor{Cliente: c, Config: configExemplo()}
	res, err := col.Coletar(context.Background(), []turma.Aluno{ana()}, []turma.Exercicio{exercicioExemplo()})
	if err != nil {
		t.Fatal(err)
	}
	if res.Entregas[0].Situacao != turma.GrupoInvisivel {
		t.Errorf("situação = %v, queria grupo_invisivel", res.Entregas[0].Situacao)
	}
}

func TestFalhaDeRedeViraErroENaoVereditoSobreOAluno(t *testing.T) {
	c := baseFalsa()
	c.erro = errors.New("conexão recusada")

	e := coletarUm(t, c)
	if e.Situacao != turma.Erro {
		t.Errorf("situação = %v, queria erro", e.Situacao)
	}
	if !strings.Contains(e.Detalhe, "conexão recusada") {
		t.Errorf("detalhe = %q, queria a causa da falha", e.Detalhe)
	}
}

func TestColetaDeVariosAlunosEmParalelo(t *testing.T) {
	c := baseFalsa()
	c.commits[grupoAna+"/ds122-html-assignment"] = []gl.Commit{
		{SHA: "aluno1", Data: time.Date(2026, 9, 1, 10, 0, 0, 0, time.Local)},
	}
	alunos := []turma.Aluno{
		ana(),
		{GRR: "GRR20259002", Nome: "Bruno Lima", Situacao: turma.Ativo},
		{GRR: "GRR20259003", Nome: "Carla Dias", Situacao: turma.Ativo},
	}
	col := &Coletor{Cliente: c, Config: configExemplo()}
	res, err := col.Coletar(context.Background(), alunos, []turma.Exercicio{exercicioExemplo()})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Entregas) != 3 {
		t.Fatalf("esperava 3 entregas, veio %d", len(res.Entregas))
	}
	for _, e := range res.Entregas {
		if e.GRR == "GRR20259001" && e.Situacao != turma.Entregue {
			t.Errorf("Ana deveria estar entregue: %+v", e)
		}
		if e.GRR != "GRR20259001" && e.Situacao != turma.SemConta {
			t.Errorf("os demais não têm conta: %+v", e)
		}
	}
}

func TestAlunoComUsuarioDiferenteDoGRR(t *testing.T) {
	// O aluno não conseguiu criar a conta com o GRR e usou outro login; o
	// grupo seguiu o padrão, mas com esse login no lugar do GRR.
	grupo := "ds122-2026-2-n-ana.souza"
	g := gl.Grupo{ID: 3, Caminho: grupo, Membro: true}
	c := baseFalsa()
	c.usuarios = []string{"ana.souza"}
	c.grupos = map[string]gl.Grupo{grupo: g}
	c.meus = []gl.Grupo{g}
	c.projetos = map[string][]gl.Projeto{grupo: {{
		ID: 12, Caminho: "ds122-html-assignment", Completo: grupo + "/ds122-html-assignment",
		RamoPadrao: "main",
	}}}
	c.commits[grupo+"/ds122-html-assignment"] = []gl.Commit{
		{SHA: "aluno1", Data: time.Date(2026, 9, 1, 10, 0, 0, 0, time.Local)},
	}

	aluna := ana()
	aluna.Usuario = "ana.souza"

	col := &Coletor{Cliente: c, Config: configExemplo()}
	res, err := col.Coletar(context.Background(), []turma.Aluno{aluna}, []turma.Exercicio{exercicioExemplo()})
	if err != nil {
		t.Fatal(err)
	}
	if res.Alunos[0].SituacaoConta != turma.ContaOK {
		t.Errorf("conta = %v, queria ok: o grupo segue o padrão com o usuário cadastrado",
			res.Alunos[0].SituacaoConta)
	}
	if res.Entregas[0].Situacao != turma.Entregue {
		t.Errorf("entrega = %+v, queria entregue", res.Entregas[0])
	}
}

func TestGrupoFixadoAMaoTemPrioridade(t *testing.T) {
	grupo := "outro-nome-qualquer"
	g := gl.Grupo{ID: 4, Caminho: grupo, Membro: true}
	c := baseFalsa()
	c.grupos = map[string]gl.Grupo{grupo: g}
	c.meus = []gl.Grupo{g}
	c.projetos = map[string][]gl.Projeto{grupo: {{
		ID: 13, Caminho: "ds122-html-assignment", Completo: grupo + "/ds122-html-assignment",
		RamoPadrao: "main",
	}}}
	c.commits[grupo+"/ds122-html-assignment"] = []gl.Commit{
		{SHA: "aluno1", Data: time.Date(2026, 9, 1, 10, 0, 0, 0, time.Local)},
	}

	aluna := ana()
	aluna.Grupo = grupo

	col := &Coletor{Cliente: c, Config: configExemplo()}
	res, err := col.Coletar(context.Background(), []turma.Aluno{aluna}, []turma.Exercicio{exercicioExemplo()})
	if err != nil {
		t.Fatal(err)
	}
	if res.Alunos[0].SituacaoConta != turma.ContaGrupoDivergente {
		t.Errorf("conta = %v, queria grupo_divergente", res.Alunos[0].SituacaoConta)
	}
	if res.Entregas[0].Situacao != turma.Entregue {
		t.Errorf("o grupo fixado à mão deveria ser usado: %+v", res.Entregas[0])
	}
}

// O caso da aluna que digitou um dígito a mais no nome do grupo, percebeu o
// erro e criou um grupo novo com o nome certo em vez de mudar a URL do
// primeiro. Os dois passam a existir, e o gravado é o errado.
func TestGrupoNovoComNomeCertoVenceOGravadoComNomeErrado(t *testing.T) {
	errado := grupoAna + "1"
	c := baseFalsa() // baseFalsa já traz o grupo certo, com o fork e o professor
	c.grupos[errado] = gl.Grupo{ID: 5, Caminho: errado, Membro: true}
	c.meus = append(c.meus, c.grupos[errado])
	c.projetos[errado] = []gl.Projeto{{
		ID: 14, Caminho: "teste", Completo: errado + "/teste", RamoPadrao: "main",
	}}
	c.commits[grupoAna+"/ds122-html-assignment"] = []gl.Commit{
		{SHA: "aluno1", Data: time.Date(2026, 9, 1, 10, 0, 0, 0, time.Local)},
	}

	aluna := ana()
	aluna.Grupo = errado

	col := &Coletor{Cliente: c, Config: configExemplo()}
	res, err := col.Coletar(context.Background(), []turma.Aluno{aluna}, []turma.Exercicio{exercicioExemplo()})
	if err != nil {
		t.Fatal(err)
	}
	if res.Alunos[0].Grupo != grupoAna {
		t.Errorf("grupo = %q, queria o do padrão: o abandonado não pode prender a coleta", res.Alunos[0].Grupo)
	}
	if res.Alunos[0].SituacaoConta != turma.ContaOK {
		t.Errorf("conta = %v, queria conta_ok", res.Alunos[0].SituacaoConta)
	}
	if res.Entregas[0].Situacao != turma.Entregue {
		t.Errorf("a entrega está no grupo novo e deveria ser achada: %+v", res.Entregas[0])
	}
}

// A aluna criou o grupo com o nome certo, errou o nome num segundo grupo e
// adicionou o professor só nesse segundo. A associação aponta para o grupo
// vazio, e o trabalho está no outro.
func TestConviteNoGrupoErradoNaoEscondeOGrupoDoPadrao(t *testing.T) {
	errado := grupoAna + "1"
	c := baseFalsa()
	certo := c.grupos[grupoAna]
	certo.Membro = false
	c.grupos[grupoAna] = certo // existe e é legível, mas sem o professor
	c.grupos[errado] = gl.Grupo{ID: 5, Caminho: errado, Membro: true}
	c.meus = []gl.Grupo{c.grupos[errado]} // o convite ficou só aqui
	c.projetos[errado] = []gl.Projeto{{
		ID: 14, Caminho: "teste", Completo: errado + "/teste", RamoPadrao: "main",
	}}
	c.commits[grupoAna+"/ds122-html-assignment"] = []gl.Commit{
		{SHA: "aluno1", Data: time.Date(2026, 9, 1, 10, 0, 0, 0, time.Local)},
	}

	aluna := ana()
	aluna.Grupo = errado

	col := &Coletor{Cliente: c, Config: configExemplo()}
	res, err := col.Coletar(context.Background(), []turma.Aluno{aluna}, []turma.Exercicio{exercicioExemplo()})
	if err != nil {
		t.Fatal(err)
	}
	if res.Alunos[0].Grupo != grupoAna {
		t.Errorf("grupo = %q, queria o do padrão", res.Alunos[0].Grupo)
	}
	if res.Alunos[0].SituacaoConta != turma.ContaSemAcesso {
		t.Errorf("conta = %v, queria sem_acesso: o professor precisa ser convidado no grupo certo",
			res.Alunos[0].SituacaoConta)
	}
	if res.Entregas[0].Situacao != turma.Entregue {
		t.Errorf("situação = %v, queria entregue: o fork está no grupo do padrão", res.Entregas[0].Situacao)
	}
}

func TestUsuarioCadastradoEUsadoNaChecagemDeConta(t *testing.T) {
	c := baseFalsa()
	c.grupos = nil
	c.meus = nil
	c.usuarios = []string{"grr20259001"} // o GRR existe, o login cadastrado não

	aluna := ana()
	aluna.Usuario = "ana.souza"

	col := &Coletor{Cliente: c, Config: configExemplo()}
	res, err := col.Coletar(context.Background(), []turma.Aluno{aluna}, []turma.Exercicio{exercicioExemplo()})
	if err != nil {
		t.Fatal(err)
	}
	if res.Entregas[0].Situacao != turma.SemConta {
		t.Errorf("situação = %v, queria sem_conta: a checagem usa o login cadastrado",
			res.Entregas[0].Situacao)
	}
}

// --- entregas em dupla ---

const grupoBruno = "ds122-2026-2-n-grr20259002"

func bruno() turma.Aluno {
	return turma.Aluno{GRR: "GRR20259002", Nome: "Bruno Lima", Situacao: turma.Ativo}
}

// duplaFalsa monta o cenário das tarefas em dupla: só Ana bifurcou, no grupo
// dela, e adicionou Bruno como membro do projeto.
func duplaFalsa() *clienteFalso {
	c := baseFalsa()
	c.usuarios = append(c.usuarios, "grr20259002")

	g := gl.Grupo{ID: 2, Caminho: grupoBruno, Membro: true}
	c.grupos[grupoBruno] = g
	c.meus = append(c.meus, g)
	c.projetos[grupoBruno] = nil // o grupo de Bruno está vazio

	fork := c.projetos[grupoAna][0]
	c.commits[fork.Completo] = []gl.Commit{
		{SHA: "aluno1", Data: time.Date(2026, 9, 1, 10, 0, 0, 0, time.Local)},
	}
	c.membros = map[string][]gl.Membro{
		fork.Completo: {
			{Usuario: "grr20259001", NivelAcesso: 50},
			{Usuario: "grr20259002", NivelAcesso: 30},
			{Usuario: "alexkutzke", NivelAcesso: 20},
		},
	}
	return c
}

func coletarDupla(t *testing.T, c *clienteFalso) Resultado {
	t.Helper()
	col := &Coletor{Cliente: c, Config: configExemplo()}
	res, err := col.Coletar(context.Background(), []turma.Aluno{ana(), bruno()}, []turma.Exercicio{exercicioExemplo()})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func entregaDe(t *testing.T, res Resultado, grr string) turma.Entrega {
	t.Helper()
	for _, e := range res.Entregas {
		if e.GRR == grr {
			return e
		}
	}
	t.Fatalf("nenhuma entrega para %s", grr)
	return turma.Entrega{}
}

func TestEntregaEmDuplaContaParaOsDois(t *testing.T) {
	res := coletarDupla(t, duplaFalsa())

	naDupla := entregaDe(t, res, "GRR20259002")
	if naDupla.Situacao != turma.Entregue {
		t.Errorf("situação de Bruno = %v, queria entregue: ele é membro do fork de Ana",
			naDupla.Situacao)
	}
	if naDupla.Projeto != grupoAna+"/ds122-html-assignment" {
		t.Errorf("projeto de Bruno = %q, queria o fork de Ana", naDupla.Projeto)
	}
	if !strings.Contains(naDupla.Detalhe, "compartilhada") {
		t.Errorf("detalhe = %q, queria dizer que a entrega é compartilhada", naDupla.Detalhe)
	}
	if entregaDe(t, res, "GRR20259001").Situacao != turma.Entregue {
		t.Error("a entrega de Ana deveria seguir normal")
	}
}

func TestEntregaEmDuplaGeraVinculo(t *testing.T) {
	res := coletarDupla(t, duplaFalsa())

	if len(res.Vinculos) != 1 {
		t.Fatalf("esperava 1 vínculo, veio %d: %+v", len(res.Vinculos), res.Vinculos)
	}
	v := res.Vinculos[0]
	if v.GRR != "GRR20259002" || v.Dono != "GRR20259001" {
		t.Errorf("vínculo = %+v, queria Bruno apontando para Ana", v)
	}
	if v.Origem != turma.VinculoDescoberto {
		t.Errorf("origem = %v, queria gitlab", v.Origem)
	}
	if v.Exercicio != "html" {
		t.Errorf("exercício = %q", v.Exercicio)
	}
}

func TestForkPróprioVenceAParticipacaoNoForkDoColega(t *testing.T) {
	c := duplaFalsa()
	// Bruno também bifurcou, no grupo dele, e mandou commit para lá.
	forkDele := gl.Projeto{
		ID: 20, Caminho: "ds122-html-assignment",
		Completo:   grupoBruno + "/ds122-html-assignment",
		RamoPadrao: "main",
	}
	c.projetos[grupoBruno] = []gl.Projeto{forkDele}
	c.commits[forkDele.Completo] = []gl.Commit{
		{SHA: "bruno1", Data: time.Date(2026, 9, 2, 10, 0, 0, 0, time.Local)},
	}

	res := coletarDupla(t, c)

	if e := entregaDe(t, res, "GRR20259002"); e.Projeto != forkDele.Completo {
		t.Errorf("projeto de Bruno = %q, queria o fork dele", e.Projeto)
	}
	if len(res.Vinculos) != 0 {
		t.Errorf("quem tem fork próprio não é integrante de equipe: %+v", res.Vinculos)
	}
}

func TestIntegranteSemGrupoProprioAindaEncontraAEntrega(t *testing.T) {
	c := duplaFalsa()
	// Bruno nem criou grupo, porque a Ana cuidou do fork.
	delete(c.grupos, grupoBruno)
	c.meus = c.meus[:1]
	delete(c.projetos, grupoBruno)

	res := coletarDupla(t, c)

	if e := entregaDe(t, res, "GRR20259002"); e.Situacao != turma.Entregue {
		t.Errorf("situação de Bruno = %v (%s), queria entregue", e.Situacao, e.Detalhe)
	}
	if len(res.Vinculos) != 1 {
		t.Errorf("esperava o vínculo mesmo sem grupo próprio: %+v", res.Vinculos)
	}
}

func TestMembroForaDoCadastroEhIgnorado(t *testing.T) {
	c := duplaFalsa()
	fork := c.projetos[grupoAna][0]
	c.membros[fork.Completo] = append(c.membros[fork.Completo],
		gl.Membro{Usuario: "monitor-da-disciplina", NivelAcesso: 30})

	res := coletarDupla(t, c)

	for _, v := range res.Vinculos {
		if v.GRR == "" || v.GRR == "MONITOR-DA-DISCIPLINA" {
			t.Errorf("membro fora do cadastro virou vínculo: %+v", v)
		}
	}
	if len(res.Vinculos) != 1 {
		t.Errorf("esperava só o vínculo de Bruno, veio %+v", res.Vinculos)
	}
}

func TestFalhaNaDescobertaDeEquipesNaoDerrubaAColeta(t *testing.T) {
	c := duplaFalsa()
	c.erroMembros = errors.New("indisponível") // a consulta de membros falhou

	res := coletarDupla(t, c)

	if entregaDe(t, res, "GRR20259001").Situacao != turma.Entregue {
		t.Error("sem a descoberta, quem tem fork próprio continua sendo avaliado")
	}
	if e := entregaDe(t, res, "GRR20259002"); e.Situacao != turma.SemFork {
		t.Errorf("situação de Bruno = %v, queria sem_fork", e.Situacao)
	}
}
