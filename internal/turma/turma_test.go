package turma

import (
	"testing"
	"time"
)

func TestCaminhoGrupo(t *testing.T) {
	c := Config{Codigo: "DS122", Semestre: "2026-02", Turno: "n",
		PadraoGrupo: "{codigo}-{ano}-{periodo}-{turno}-{grr}"}
	if got, quer := c.CaminhoGrupo("GRR20249999"), "ds122-2026-2-n-grr20249999"; got != quer {
		t.Errorf("CaminhoGrupo = %q, queria %q", got, quer)
	}
}

func TestAtrasoEmDias(t *testing.T) {
	prazo := NovaData(2026, time.September, 5)
	casos := []struct {
		nome   string
		commit time.Time
		quer   int
	}{
		{"antes do prazo", time.Date(2026, 9, 4, 10, 0, 0, 0, time.Local), 0},
		{"no fim do dia do prazo", time.Date(2026, 9, 5, 23, 50, 0, 0, time.Local), 0},
		{"logo depois da meia-noite", time.Date(2026, 9, 6, 0, 10, 0, 0, time.Local), 1},
		{"três dias depois", time.Date(2026, 9, 8, 9, 0, 0, 0, time.Local), 3},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if got := AtrasoEmDias(prazo, c.commit); got != c.quer {
				t.Errorf("AtrasoEmDias = %d, queria %d", got, c.quer)
			}
		})
	}
}

func TestFimDoDiaCobreODiaInteiro(t *testing.T) {
	d := NovaData(2026, time.September, 5)
	limite := d.FimDoDia()
	tarde := time.Date(2026, 9, 5, 23, 59, 58, 0, time.Local)
	if tarde.After(limite) {
		t.Errorf("commit às 23h59min58s deveria estar no prazo")
	}
	depois := time.Date(2026, 9, 6, 0, 0, 1, 0, time.Local)
	if !depois.After(limite) {
		t.Errorf("commit do dia seguinte deveria estar fora do prazo")
	}
}

func TestSubstituirEntregasPreservaOsOutrosExercicios(t *testing.T) {
	tur := &Turma{Entregas: []Entrega{
		{Exercicio: "html", GRR: "GRR1", Situacao: Entregue},
		{Exercicio: "js", GRR: "GRR1", Situacao: SemFork},
	}}
	tur.SubstituirEntregas("html", []Entrega{{Exercicio: "html", GRR: "GRR1", Situacao: SemCommitNoPrazo}})

	if e, _ := tur.Entrega("js", "GRR1"); e.Situacao != SemFork {
		t.Errorf("entrega de js foi alterada: %v", e.Situacao)
	}
	if e, _ := tur.Entrega("html", "GRR1"); e.Situacao != SemCommitNoPrazo {
		t.Errorf("entrega de html não foi substituída: %v", e.Situacao)
	}
	if len(tur.Entregas) != 2 {
		t.Errorf("esperava 2 entregas, tem %d", len(tur.Entregas))
	}
}

func TestExercicioValidar(t *testing.T) {
	casos := []struct {
		nome string
		e    Exercicio
		erro bool
	}{
		{"completo", Exercicio{ID: "html", Repo: "ds122-html", Prazo: NovaData(2026, 9, 5)}, false},
		{"sem id", Exercicio{Repo: "ds122-html", Prazo: NovaData(2026, 9, 5)}, true},
		{"sem repo", Exercicio{ID: "html", Prazo: NovaData(2026, 9, 5)}, true},
		{"sem prazo", Exercicio{ID: "html", Repo: "ds122-html"}, true},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if err := c.e.Validar(); (err != nil) != c.erro {
				t.Errorf("Validar = %v, queria erro=%t", err, c.erro)
			}
		})
	}
}

func TestChaveNomeIgnoraAcentoECaixa(t *testing.T) {
	if ChaveNome("JOÃO DA SILVA") != ChaveNome("joao  da silva") {
		t.Error("nomes equivalentes produziram chaves diferentes")
	}
}

func exerciciosParaMedia() []Exercicio {
	return []Exercicio{
		{ID: "prepare", Repo: "r1", Prazo: NovaData(2026, time.August, 15), Peso: 1, Situacao: ExercicioAtivo},
		{ID: "html", Repo: "r2", Prazo: NovaData(2026, time.September, 5), Peso: 3, Situacao: ExercicioAtivo},
		{ID: "js", Repo: "r3", Prazo: NovaData(2026, time.October, 9), Peso: 1, Situacao: ExercicioAtivo},
	}
}

func turmaComNotas() *Turma {
	return &Turma{
		Alunos: []Aluno{
			{GRR: "GRR1", Nome: "Ana", Situacao: Ativo},
			{GRR: "GRR2", Nome: "Bruno", Situacao: Ativo},
		},
		Exercicios: exerciciosParaMedia(),
		Notas: []Nota{
			{Exercicio: "prepare", GRR: "GRR1", Valor: 100},
			{Exercicio: "html", GRR: "GRR1", Valor: 80},
			{Exercicio: "prepare", GRR: "GRR2", Valor: 60},
		},
	}
}

func TestResultadosContamZeroParaPrazoVencidoSemNota(t *testing.T) {
	tur := turmaComNotas()
	hoje := NovaData(2026, time.September, 20) // js ainda em aberto

	res := tur.Resultados(exerciciosParaMedia(), false, hoje)

	// Ana: (100*1 + 80*3) / 4 = 85
	if got := res[0].Media; got != 85 {
		t.Errorf("média de Ana = %g, queria 85", got)
	}
	// Bruno: (60*1 + 0*3) / 4 = 15, o html vencido sem nota conta zero.
	if got := res[1].Media; got != 15 {
		t.Errorf("média de Bruno = %g, queria 15", got)
	}
	for _, r := range res {
		if len(r.Considerados) != 2 {
			t.Errorf("%s: exercício com prazo em aberto não podia entrar: %v",
				r.Aluno.Nome, r.Considerados)
		}
	}
}

func TestResultadosSomenteLancadas(t *testing.T) {
	tur := turmaComNotas()
	hoje := NovaData(2026, time.September, 20)

	res := tur.Resultados(exerciciosParaMedia(), true, hoje)

	// Bruno só tem a nota do prepare, então a média é ela mesma.
	if got := res[1].Media; got != 60 {
		t.Errorf("média de Bruno = %g, queria 60", got)
	}
	if res[1].Lancadas != 1 {
		t.Errorf("lançadas = %d, queria 1", res[1].Lancadas)
	}
}

func TestRegistrarNotaSubstituiSemDuplicar(t *testing.T) {
	tur := turmaComNotas()
	tur.RegistrarNota(Nota{Exercicio: "html", GRR: "GRR1", Valor: 95, Comentario: "corrigido"})

	n, ok := tur.Nota("html", "GRR1")
	if !ok || n.Valor != 95 || n.Comentario != "corrigido" {
		t.Errorf("nota não foi substituída: %+v", n)
	}
	contagem := 0
	for _, x := range tur.Notas {
		if x.Exercicio == "html" && x.GRR == "GRR1" {
			contagem++
		}
	}
	if contagem != 1 {
		t.Errorf("esperava 1 registro, tem %d", contagem)
	}
}

func TestRemoverNota(t *testing.T) {
	tur := turmaComNotas()
	if !tur.RemoverNota("html", "GRR1") {
		t.Fatal("remoção não encontrou a nota")
	}
	if _, ok := tur.Nota("html", "GRR1"); ok {
		t.Error("a nota continua no cadastro")
	}
	if tur.RemoverNota("html", "GRR1") {
		t.Error("remover duas vezes deveria devolver falso")
	}
}

// --- entregas compartilhadas ---

func turmaComDupla() *Turma {
	return &Turma{
		Alunos: []Aluno{
			{GRR: "GRR1", Nome: "Ana", Situacao: Ativo},
			{GRR: "GRR2", Nome: "Bruno", Situacao: Ativo},
			{GRR: "GRR3", Nome: "Carla", Situacao: Ativo},
		},
		Vinculos: []Vinculo{
			{Exercicio: "html", GRR: "GRR2", Dono: "GRR1", Origem: VinculoDescoberto},
		},
	}
}

func TestDonoEEquipe(t *testing.T) {
	tur := turmaComDupla()

	if got := tur.Dono("html", "GRR2"); got != "GRR1" {
		t.Errorf("dono = %q, queria GRR1", got)
	}
	if got := tur.Dono("html", "GRR3"); got != "GRR3" {
		t.Errorf("aluno sem vínculo é dono do próprio fork, veio %q", got)
	}
	if got := tur.Dono("js", "GRR2"); got != "GRR2" {
		t.Errorf("o vínculo vale por exercício; em js veio %q", got)
	}

	if equipe := tur.Equipe("html", "GRR1"); len(equipe) != 2 {
		t.Errorf("equipe pelo dono = %v, queria os dois", equipe)
	}
	if equipe := tur.Equipe("html", "GRR2"); len(equipe) != 2 {
		t.Errorf("equipe pelo integrante = %v, queria os dois", equipe)
	}
	if !tur.Compartilhada("html", "GRR1") || tur.Compartilhada("html", "GRR3") {
		t.Error("Compartilhada não separou quem entregou em dupla de quem entregou sozinho")
	}
}

func TestSubstituirVinculosDescobertosPreservaOsManuais(t *testing.T) {
	tur := turmaComDupla()
	tur.RegistrarVinculo(Vinculo{Exercicio: "html", GRR: "GRR3", Dono: "GRR1", Origem: VinculoManual})
	tur.RegistrarVinculo(Vinculo{Exercicio: "js", GRR: "GRR2", Dono: "GRR1", Origem: VinculoDescoberto})

	// Nova coleta de html: Bruno não aparece mais como membro do fork.
	tur.SubstituirVinculosDescobertos("html", nil)

	if _, ok := tur.VinculosDoExercicio("html")["GRR2"]; ok {
		t.Error("vínculo descoberto deveria sair quando a coleta não o encontra mais")
	}
	if v, ok := tur.VinculosDoExercicio("html")["GRR3"]; !ok || v.Origem != VinculoManual {
		t.Error("vínculo cadastrado à mão não podia ser apagado pela coleta")
	}
	if _, ok := tur.VinculosDoExercicio("js")["GRR2"]; !ok {
		t.Error("a coleta de html não podia mexer nos vínculos de js")
	}
}

func TestVinculoManualVenceODescoberto(t *testing.T) {
	tur := turmaComDupla()
	tur.RegistrarVinculo(Vinculo{Exercicio: "html", GRR: "GRR2", Dono: "GRR3", Origem: VinculoManual})

	tur.SubstituirVinculosDescobertos("html", []Vinculo{
		{Exercicio: "html", GRR: "GRR2", Dono: "GRR1", Origem: VinculoDescoberto},
	})

	if got := tur.Dono("html", "GRR2"); got != "GRR3" {
		t.Errorf("dono = %q, queria o cadastrado à mão", got)
	}
	if len(tur.VinculosDoExercicio("html")) != 1 {
		t.Errorf("o mesmo aluno não pode ter dois vínculos: %+v", tur.Vinculos)
	}
}

func TestRemoverVinculo(t *testing.T) {
	tur := turmaComDupla()
	if !tur.RemoverVinculo("html", "GRR2") {
		t.Fatal("remoção não encontrou o vínculo")
	}
	if got := tur.Dono("html", "GRR2"); got != "GRR2" {
		t.Errorf("depois de desvinculado, o dono é ele mesmo, veio %q", got)
	}
}
