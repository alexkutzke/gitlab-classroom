package acoes

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/alexkutzke/gitlab-classroom/internal/turma"
)

// turmaDoResumo tem dois alunos em ordem, ambos com entrega no prazo do html
// e nenhuma nota. Cada caso do teste acrescenta a borda que quer conferir.
func turmaDoResumo() *turma.Turma {
	coleta := time.Date(2026, time.October, 4, 9, 12, 0, 0, time.FixedZone("-03", -3*60*60))
	t := &turma.Turma{
		Config: turma.Config{Codigo: "DS122", Semestre: "2026-02", Turno: "n"},
		Exercicios: []turma.Exercicio{{
			ID: "html", Repo: "ds122-html-assignment", Titulo: "HTML e CSS",
			Prazo: turma.NovaData(2026, time.September, 5), Peso: 1,
		}},
		Alunos: []turma.Aluno{
			{GRR: "GRR20259001", Nome: "ANA SOUZA", SituacaoConta: turma.ContaOK},
			{GRR: "GRR20259002", Nome: "BETO LIMA", SituacaoConta: turma.ContaOK},
		},
		Entregas: []turma.Entrega{
			{Exercicio: "html", GRR: "GRR20259001", Situacao: turma.Entregue,
				Projeto: forkAna, Commit: "abc123", ColetadoEm: coleta},
			{Exercicio: "html", GRR: "GRR20259002", Situacao: turma.Entregue,
				Projeto: forkBeto, Commit: "def456", ColetadoEm: coleta},
		},
	}
	t.Ordenar()
	return t
}

func exercicioDoResumo(t *testing.T, r ResumoTurma, id string) ResumoExercicioContagem {
	t.Helper()
	for _, e := range r.Exercicios {
		if e.ID == id {
			return e
		}
	}
	t.Fatalf("exercício %s ausente do resumo: %+v", id, r.Exercicios)
	return ResumoExercicioContagem{}
}

func TestResumoDe(t *testing.T) {
	geradoEm := time.Date(2026, time.October, 6, 14, 0, 0, 0, time.UTC)

	casos := []struct {
		nome     string
		preparar func(*turma.Turma)
		conferir func(*testing.T, ResumoTurma)
	}{
		{
			nome: "entrega no prazo com commit depois conta como entregue",
			preparar: func(tu *turma.Turma) {
				en, _ := tu.Entrega("html", "GRR20259001")
				en.AtrasoDias = 3
			},
			conferir: func(t *testing.T, r ResumoTurma) {
				e := exercicioDoResumo(t, r, "html")
				if e.Entregues != 2 || e.Atrasadas != 0 {
					t.Errorf("entregues %d, atrasadas %d; esperado 2 e 0", e.Entregues, e.Atrasadas)
				}
			},
		},
		{
			nome: "situações de entrega caem cada uma na sua contagem",
			preparar: func(tu *turma.Turma) {
				tu.Alunos = append(tu.Alunos,
					turma.Aluno{GRR: "GRR20259003", SituacaoConta: turma.ContaOK},
					turma.Aluno{GRR: "GRR20259004", SituacaoConta: turma.ContaOK},
					turma.Aluno{GRR: "GRR20259005", SituacaoConta: turma.ContaOK},
					turma.Aluno{GRR: "GRR20259006", SituacaoConta: turma.ContaSemUsuario})
				tu.Entregas = append(tu.Entregas,
					turma.Entrega{Exercicio: "html", GRR: "GRR20259003", Situacao: turma.SemCommitNoPrazo},
					turma.Entrega{Exercicio: "html", GRR: "GRR20259004", Situacao: turma.ForkSemCommit},
					turma.Entrega{Exercicio: "html", GRR: "GRR20259005", Situacao: turma.Erro},
					turma.Entrega{Exercicio: "html", GRR: "GRR20259006", Situacao: turma.SemConta})
			},
			conferir: func(t *testing.T, r ResumoTurma) {
				e := exercicioDoResumo(t, r, "html")
				if e.Entregues != 2 || e.Atrasadas != 1 || e.SemEntrega != 1 || e.Erros != 1 {
					t.Errorf("entregues %d, atrasadas %d, sem entrega %d, erros %d; esperado 2, 1, 1, 1",
						e.Entregues, e.Atrasadas, e.SemEntrega, e.Erros)
				}
				if r.Cadastro.SemConta != 1 {
					t.Errorf("sem_conta no cadastro = %d, esperado 1", r.Cadastro.SemConta)
				}
			},
		},
		{
			nome: "dupla conta por aluno nas entregas e uma vez nas devolutivas",
			preparar: func(tu *turma.Turma) {
				beto, _ := tu.Entrega("html", "GRR20259002")
				beto.Projeto = forkAna
				tu.RegistrarVinculo(turma.Vinculo{Exercicio: "html", GRR: "GRR20259002",
					Dono: "GRR20259001", Origem: turma.VinculoDescoberto})
				for _, grr := range []string{"GRR20259001", "GRR20259002"} {
					tu.RegistrarNota(turma.Nota{Exercicio: "html", GRR: grr, Valor: 80,
						Comentario: "faltou o label"})
				}
			},
			conferir: func(t *testing.T, r ResumoTurma) {
				e := exercicioDoResumo(t, r, "html")
				if e.Entregues != 2 {
					t.Errorf("entregues = %d, esperado 2, um por aluno", e.Entregues)
				}
				if e.Corrigidas != 2 {
					t.Errorf("corrigidas = %d, esperado 2, um por aluno", e.Corrigidas)
				}
				if e.DevolutivasPendentes != 1 {
					t.Errorf("devolutivas pendentes = %d, esperado 1, uma issue por fork", e.DevolutivasPendentes)
				}
			},
		},
		{
			nome: "devolutiva com comentário alterado depois de publicada fica pendente",
			preparar: func(tu *turma.Turma) {
				tu.RegistrarNota(turma.Nota{Exercicio: "html", GRR: "GRR20259001", Valor: 80,
					Comentario: "texto revisto"})
				tu.RegistrarNota(turma.Nota{Exercicio: "html", GRR: "GRR20259002", Valor: 90,
					Comentario: "texto publicado"})
				tu.RegistrarDevolutiva(turma.Devolutiva{Exercicio: "html", GRR: "GRR20259001",
					Projeto: forkAna, Issue: 1, Hash: turma.HashComentario("texto original")})
				tu.RegistrarDevolutiva(turma.Devolutiva{Exercicio: "html", GRR: "GRR20259002",
					Projeto: forkBeto, Issue: 1, Hash: turma.HashComentario("texto publicado")})
			},
			conferir: func(t *testing.T, r ResumoTurma) {
				if n := exercicioDoResumo(t, r, "html").DevolutivasPendentes; n != 1 {
					t.Errorf("devolutivas pendentes = %d, esperado 1, só a desatualizada", n)
				}
			},
		},
		{
			nome: "nota sem comentário não deixa devolutiva pendente",
			preparar: func(tu *turma.Turma) {
				tu.RegistrarNota(turma.Nota{Exercicio: "html", GRR: "GRR20259001", Valor: 80})
				tu.RegistrarNota(turma.Nota{Exercicio: "html", GRR: "GRR20259002", Valor: 70,
					Comentario: "   "})
			},
			conferir: func(t *testing.T, r ResumoTurma) {
				e := exercicioDoResumo(t, r, "html")
				if e.DevolutivasPendentes != 0 {
					t.Errorf("devolutivas pendentes = %d, esperado 0", e.DevolutivasPendentes)
				}
				if e.Corrigidas != 2 || e.PorCorrigir != 0 {
					t.Errorf("corrigidas %d, por corrigir %d; esperado 2 e 0", e.Corrigidas, e.PorCorrigir)
				}
			},
		},
		{
			nome: "aluno com grupo divergente entra no cadastro",
			preparar: func(tu *turma.Turma) {
				a, _ := tu.AlunoPorGRR("GRR20259002")
				a.SituacaoConta = turma.ContaGrupoDivergente
				tu.Alunos = append(tu.Alunos, turma.Aluno{GRR: "GRR20259003"})
			},
			conferir: func(t *testing.T, r ResumoTurma) {
				c := r.Cadastro
				if c.Ativos != 3 || c.GrupoDivergente != 1 || c.SemReconciliacao != 1 {
					t.Errorf("cadastro %+v; esperado 3 ativos, 1 grupo divergente, 1 sem reconciliação", c)
				}
			},
		},
		{
			nome: "aluno cancelado fica fora das contagens",
			preparar: func(tu *turma.Turma) {
				a, _ := tu.AlunoPorGRR("GRR20259002")
				a.Situacao = turma.Cancelado
				a.SituacaoConta = turma.ContaSemUsuario
				tu.RegistrarNota(turma.Nota{Exercicio: "html", GRR: "GRR20259002", Valor: 70,
					Comentario: "texto"})
			},
			conferir: func(t *testing.T, r ResumoTurma) {
				e := exercicioDoResumo(t, r, "html")
				if r.Cadastro.Ativos != 1 || r.Cadastro.SemConta != 0 {
					t.Errorf("cadastro %+v; esperado 1 ativo e nenhum sem conta", r.Cadastro)
				}
				if e.Entregues != 1 || e.Corrigidas != 0 || e.DevolutivasPendentes != 0 {
					t.Errorf("exercício %+v; esperado 1 entregue, 0 corrigidas, 0 pendentes", e)
				}
			},
		},
		{
			nome: "turma nunca coletada sai sem coletado_em",
			preparar: func(tu *turma.Turma) {
				tu.Entregas = nil
			},
			conferir: func(t *testing.T, r ResumoTurma) {
				if r.ColetadoEm != nil {
					t.Errorf("coletado_em = %v, esperado ausente", r.ColetadoEm)
				}
				js, err := json.Marshal(r)
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(string(js), "coletado_em") {
					t.Errorf("o JSON traz coletado_em sem coleta: %s", js)
				}
			},
		},
		{
			nome: "coletado_em é a coleta mais recente",
			preparar: func(tu *turma.Turma) {
				en, _ := tu.Entrega("html", "GRR20259002")
				en.ColetadoEm = time.Date(2026, time.October, 5, 8, 0, 0, 0, time.UTC)
			},
			conferir: func(t *testing.T, r ResumoTurma) {
				esperado := time.Date(2026, time.October, 5, 8, 0, 0, 0, time.UTC)
				if r.ColetadoEm == nil || !r.ColetadoEm.Equal(esperado) {
					t.Errorf("coletado_em = %v, esperado %v", r.ColetadoEm, esperado)
				}
			},
		},
		{
			nome: "exercício arquivado fica fora da lista",
			preparar: func(tu *turma.Turma) {
				tu.Exercicios = append(tu.Exercicios, turma.Exercicio{
					ID: "antigo", Repo: "ds122-antigo", Prazo: turma.NovaData(2026, time.August, 1),
					Situacao: turma.ExercicioArquivado,
				})
			},
			conferir: func(t *testing.T, r ResumoTurma) {
				for _, e := range r.Exercicios {
					if e.ID == "antigo" {
						t.Errorf("exercício arquivado no resumo: %+v", e)
					}
				}
			},
		},
		{
			nome: "turma sem exercício ativo sai com lista vazia, e não nula",
			preparar: func(tu *turma.Turma) {
				tu.Exercicios = nil
			},
			conferir: func(t *testing.T, r ResumoTurma) {
				js, err := json.Marshal(r)
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(string(js), `"exercicios":[]`) {
					t.Errorf("lista de exercícios não sai como []: %s", js)
				}
			},
		},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			tu := turmaDoResumo()
			c.preparar(tu)
			tu.Ordenar()
			r := ResumoDe(tu, geradoEm)
			if r.Versao != VersaoResumo || !r.GeradoEm.Equal(geradoEm) {
				t.Errorf("versão %d, gerado em %v", r.Versao, r.GeradoEm)
			}
			c.conferir(t, r)
		})
	}
}
