package acoes

import (
	"time"

	"github.com/alexkutzke/gitlab-classroom/internal/turma"
)

// VersaoResumo é a versão do contrato de `classroom resumo --json`.
//
// O painel lê esse JSON e recusa versão que não conhece. Muda quando um campo
// existente muda de sentido ou some; campo novo não muda a versão, porque o
// painel ignora o que não conhece.
const VersaoResumo = 1

// ResumoTurma é o contrato que `classroom resumo --json` imprime para o
// painel. Leva só contagens: o detalhe por aluno continua no status e na TUI.
type ResumoTurma struct {
	Versao   int       `json:"versao"`
	GeradoEm time.Time `json:"gerado_em"`
	// ColetadoEm é o instante da coleta mais recente registrada em
	// entregas.csv, e fica ausente quando a turma nunca foi coletada. Não é a
	// data de modificação do arquivo, que muda com a sincronização da pasta.
	ColetadoEm *time.Time                `json:"coletado_em,omitempty"`
	Cadastro   ResumoCadastro            `json:"cadastro"`
	Exercicios []ResumoExercicioContagem `json:"exercicios"`
}

// ResumoCadastro conta os alunos ativos por situação da conta no GitLab.
type ResumoCadastro struct {
	// Ativos inclui os que estão em ordem.
	Ativos          int `json:"ativos"`
	SemConta        int `json:"sem_conta"`
	SemAcesso       int `json:"sem_acesso"`
	GrupoInvisivel  int `json:"grupo_invisivel"`
	GrupoDivergente int `json:"grupo_divergente"`
	// SemReconciliacao é o aluno que ainda não passou por `classroom sync`.
	SemReconciliacao int `json:"sem_reconciliacao"`
}

// ResumoExercicioContagem é a linha de um exercício ativo no resumo.
//
// Entregues, Atrasadas, SemEntrega e Erros são disjuntos. As situações de
// cadastro ficam em ResumoCadastro, e aluno sem linha em entregas.csv não
// entra em nenhuma, então as quatro não somam necessariamente os ativos.
type ResumoExercicioContagem struct {
	ID        string  `json:"id"`
	Titulo    string  `json:"titulo"`
	Categoria string  `json:"categoria"`
	Prazo     string  `json:"prazo"`
	Peso      float64 `json:"peso"`

	Entregues  int `json:"entregues"`
	Atrasadas  int `json:"atrasadas"`
	SemEntrega int `json:"sem_entrega"`
	Erros      int `json:"erros"`

	Corrigidas  int `json:"corrigidas"`
	PorCorrigir int `json:"por_corrigir"`

	VerificacoesDesatualizadas int `json:"verificacoes_desatualizadas"`
	// DevolutivasPendentes conta forks, e não alunos: a devolutiva de uma
	// equipe é uma issue só.
	DevolutivasPendentes int `json:"devolutivas_pendentes"`
}

// ResumoDe monta o resumo da turma a partir do que está em .classroom/, sem
// consultar o GitLab.
//
// As contagens por exercício vêm de PanoramaDe, a mesma conta do status e da
// tela inicial: uma segunda definição acabaria divergindo. Nenhum campo
// depende da data do dia, e por isso PanoramaDe recebe a data zero.
func ResumoDe(t *turma.Turma, geradoEm time.Time) ResumoTurma {
	p := PanoramaDe(t, turma.Data{})
	r := ResumoTurma{
		Versao:     VersaoResumo,
		GeradoEm:   geradoEm,
		Cadastro:   ResumoCadastro{Ativos: p.Ativos},
		Exercicios: []ResumoExercicioContagem{},
	}

	var coletado time.Time
	for _, en := range t.Entregas {
		if en.ColetadoEm.After(coletado) {
			coletado = en.ColetadoEm
		}
	}
	if !coletado.IsZero() {
		r.ColetadoEm = &coletado
	}

	for _, a := range p.ContasPendentes {
		switch a.SituacaoConta {
		case turma.ContaSemUsuario:
			r.Cadastro.SemConta++
		case turma.ContaSemAcesso:
			r.Cadastro.SemAcesso++
		case turma.ContaGrupoInvisivel:
			r.Cadastro.GrupoInvisivel++
		case turma.ContaGrupoDivergente:
			r.Cadastro.GrupoDivergente++
		case turma.ContaDesconhecida:
			r.Cadastro.SemReconciliacao++
		}
	}

	ativos := t.Ativos()
	for _, re := range p.Exercicios {
		e := re.Exercicio
		s := re.Situacoes
		r.Exercicios = append(r.Exercicios, ResumoExercicioContagem{
			ID:                         e.ID,
			Titulo:                     e.Titulo,
			Categoria:                  e.CategoriaDe(),
			Prazo:                      e.Prazo.String(),
			Peso:                       e.Peso,
			Entregues:                  s[turma.Entregue],
			Atrasadas:                  s[turma.SemCommitNoPrazo],
			SemEntrega:                 s[turma.SemFork] + s[turma.ForkSemCommit],
			Erros:                      s[turma.Erro],
			Corrigidas:                 re.Notas,
			PorCorrigir:                re.SemNota,
			VerificacoesDesatualizadas: re.VerificacoesVelhas,
			DevolutivasPendentes:       devolutivasPendentes(t, e, ativos),
		})
	}
	return r
}
