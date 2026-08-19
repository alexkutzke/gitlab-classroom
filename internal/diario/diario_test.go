package diario

import (
	"testing"

	"github.com/alexkutzke/gitlab-classroom/internal/turma"
)

func TestLer(t *testing.T) {
	alunos, err := Ler("testdata")
	if err != nil {
		t.Fatal(err)
	}
	if len(alunos) != 3 {
		t.Fatalf("esperava 3 alunos, veio %d", len(alunos))
	}
	if alunos[0].GRR != "GRR20259001" || alunos[0].Nome != "Ana Souza" {
		t.Errorf("primeiro aluno lido errado: %+v", alunos[0])
	}
	if alunos[2].Situacao != turma.Cancelado {
		t.Errorf("situação de cancelado não foi lida: %+v", alunos[2])
	}
}

func TestLerSemArquivo(t *testing.T) {
	_, err := Ler(t.TempDir())
	if _, ok := err.(*ErrSemCadastro); !ok {
		t.Errorf("esperava ErrSemCadastro, veio %v", err)
	}
}

func TestLerConfig(t *testing.T) {
	c, err := LerConfig("testdata")
	if err != nil {
		t.Fatal(err)
	}
	if c.Codigo != "DS122" || c.Turma != "TADSN2A" {
		t.Errorf("config lida errada: %+v", c)
	}
}

func TestImportarPreservaOApuradoNoGitLab(t *testing.T) {
	tur := &turma.Turma{Alunos: []turma.Aluno{{
		GRR: "GRR20259001", Nome: "Ana", Email: "antigo@ufpr.br",
		Usuario: "grr20259001", Grupo: "ds122-2026-2-n-outro-nome",
		Situacao: turma.Ativo, SituacaoConta: turma.ContaGrupoDivergente,
	}}}
	alunos, err := Ler("testdata")
	if err != nil {
		t.Fatal(err)
	}
	res := Importar(tur, alunos)

	a, _ := tur.AlunoPorGRR("GRR20259001")
	if a.Nome != "Ana Souza" || a.Email != "ana@ufpr.br" {
		t.Errorf("nome e e-mail deveriam vir do SIGA: %+v", a)
	}
	if a.Grupo != "ds122-2026-2-n-outro-nome" || a.SituacaoConta != turma.ContaGrupoDivergente {
		t.Errorf("o que foi apurado no GitLab não podia ser sobrescrito: %+v", a)
	}
	if len(res.Novos) != 2 {
		t.Errorf("esperava 2 alunos novos, veio %v", res.Novos)
	}
}

func TestImportarCancelaQuemSaiuDaLista(t *testing.T) {
	tur := &turma.Turma{Alunos: []turma.Aluno{
		{GRR: "GRR20259099", Nome: "Quem Trancou", Situacao: turma.Ativo},
	}}
	alunos, err := Ler("testdata")
	if err != nil {
		t.Fatal(err)
	}
	res := Importar(tur, alunos)

	a, ok := tur.AlunoPorGRR("GRR20259099")
	if !ok {
		t.Fatal("aluno fora da lista do SIGA foi apagado")
	}
	if a.Situacao != turma.Cancelado {
		t.Errorf("situação = %v, queria cancelado", a.Situacao)
	}
	if len(res.Cancelados) != 1 {
		t.Errorf("cancelamento não foi relatado: %+v", res)
	}
}
