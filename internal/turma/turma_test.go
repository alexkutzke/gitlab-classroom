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
