package turma

import "testing"

func alunosExemplo() []Aluno {
	return []Aluno{
		{GRR: "GRR20259001", Nome: "ANA FLAVIA FORTESKI"},
		{GRR: "GRR20259002", Nome: "BRUNO ALESSANDRO SAITER"},
		{GRR: "GRR20259003", Nome: "EDUARDO MEIRA MOREIRA"},
		{GRR: "GRR20259004", Nome: "EDUARDO AUGUSTO SIQUEIRA MARQUEZINI"},
		{GRR: "GRR20259005", Nome: "LETICIA MAROBIM DE BARROS"},
	}
}

func TestSugerirPeloNomeDeExibicao(t *testing.T) {
	a, ok := Sugerir(alunosExemplo(), "Letícia Marobim", "")
	if !ok || a.GRR != "GRR20259005" {
		t.Errorf("aluno = %+v, ok = %v", a, ok)
	}
}

func TestSugerirPeloLogin(t *testing.T) {
	a, ok := Sugerir(alunosExemplo(), "", "bruno.saiter")
	if !ok || a.GRR != "GRR20259002" {
		t.Errorf("aluno = %+v, ok = %v", a, ok)
	}
}

func TestUmPedacoSoNaoBasta(t *testing.T) {
	if a, ok := Sugerir(alunosExemplo(), "Eduardo", ""); ok {
		t.Errorf("primeiro nome sozinho não pode virar sugestão: %+v", a)
	}
}

func TestEmpateNaoViraSugestao(t *testing.T) {
	alunos := []Aluno{
		{GRR: "GRR20259001", Nome: "JOAO PEDRO COSTA"},
		{GRR: "GRR20259002", Nome: "JOAO PEDRO ARRUDA"},
	}
	if a, ok := Sugerir(alunos, "Joao Pedro", ""); ok {
		t.Errorf("dois candidatos empatados não podem virar sugestão: %+v", a)
	}
}

func TestSemNomeNemLoginNaoSugere(t *testing.T) {
	if _, ok := Sugerir(alunosExemplo(), "", ""); ok {
		t.Error("sem nada para comparar não há sugestão")
	}
}

func TestParticulasNaoContam(t *testing.T) {
	// "de" e "barros" com "de" valendo ponto dariam sugestão errada.
	if a, ok := Sugerir(alunosExemplo(), "Marcos de Souza", ""); ok {
		t.Errorf("sugestão = %+v, queria nenhuma", a)
	}
}
