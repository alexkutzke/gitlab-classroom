package acoes

import (
	"sort"
	"strconv"

	"github.com/alexkutzke/gitlab-classroom/internal/turma"
)

// Panorama resume o estado da turma para a tela inicial e para o comando
// status. É cálculo, não desenho: quem exibe decide o formato.
type Panorama struct {
	Ativos          int
	ContasPendentes []turma.Aluno
	Exercicios      []ResumoExercicio
	// Categorias são as categorias de exercício em uso, na ordem do
	// semestre. Com mais de uma, as telas precisam dizer a que categoria
	// cada linha pertence, porque as médias são separadas.
	Categorias []string
}

// VariasCategorias informa se a turma tem exercício em sala e trabalho ao
// mesmo tempo.
func (p Panorama) VariasCategorias() bool { return len(p.Categorias) > 1 }

// ResumoExercicio conta o que já foi apurado em um exercício.
type ResumoExercicio struct {
	Exercicio turma.Exercicio
	// Coletado informa se há alguma entrega registrada.
	Coletado  bool
	Vencido   bool
	Situacoes map[turma.SituacaoEntrega]int
	// Pendentes são os alunos ativos sem entrega no prazo, o que inclui
	// quem não bifurcou e quem só commitou depois.
	Pendentes int
	// Notas conta quantos alunos ativos já têm nota lançada.
	Notas int
	// SemNota conta quem entregou e ainda não foi corrigido.
	SemNota        int
	Compartilhadas int
	Verificacoes   map[turma.SituacaoVerificacao]int
	// VerificacoesVelhas são as feitas sobre commit anterior ao da entrega
	// corrente, e que por isso não valem mais.
	VerificacoesVelhas int
}

// TemSuite repete a informação do exercício, para a tela não precisar
// alcançá-lo.
func (r ResumoExercicio) TemSuite() bool { return r.Exercicio.TemSuite() }

// Panorama monta o resumo da turma na data informada.
func PanoramaDe(t *turma.Turma, hoje turma.Data) Panorama {
	ativos := t.Ativos()
	p := Panorama{Ativos: len(ativos)}

	for _, a := range ativos {
		if a.SituacaoConta != turma.ContaOK {
			p.ContasPendentes = append(p.ContasPendentes, a)
		}
	}

	p.Categorias = turma.CategoriasAtivas(t.ExerciciosAtivos())

	for _, e := range t.ExerciciosAtivos() {
		entregas := t.EntregasDoExercicio(e.ID)
		verificacoes := t.VerificacoesDoExercicio(e.ID)

		r := ResumoExercicio{
			Exercicio:      e,
			Coletado:       len(entregas) > 0,
			Vencido:        !hoje.IsZero() && !e.Prazo.Depois(hoje),
			Situacoes:      map[turma.SituacaoEntrega]int{},
			Verificacoes:   map[turma.SituacaoVerificacao]int{},
			Compartilhadas: len(t.VinculosDoExercicio(e.ID)),
		}

		for _, a := range ativos {
			en, temEntrega := entregas[a.GRR]
			if temEntrega {
				r.Situacoes[en.Situacao]++
				if en.Situacao != turma.Entregue {
					r.Pendentes++
				}
			}
			if _, temNota := t.Nota(e.ID, a.GRR); temNota {
				r.Notas++
			} else if temEntrega && en.Situacao == turma.Entregue {
				r.SemNota++
			}
			if v, ok := verificacoes[a.GRR]; ok {
				r.Verificacoes[v.Situacao]++
				if v.Desatualizada(en.Commit) {
					r.VerificacoesVelhas++
				}
			}
		}
		p.Exercicios = append(p.Exercicios, r)
	}

	sort.SliceStable(p.Exercicios, func(i, j int) bool {
		return p.Exercicios[i].Exercicio.Prazo.Antes(p.Exercicios[j].Exercicio.Prazo)
	})
	return p
}

// Pendencias devolve as frases do que falta fazer, na ordem em que convém
// resolver: cadastro, coleta, correção, verificação.
//
// Lista vazia é o estado bom, e a tela pode dizer isso em uma linha.
func (p Panorama) Pendencias() []string {
	var out []string
	if n := len(p.ContasPendentes); n > 0 {
		out = append(out, plural(n, "aluno sem grupo em ordem no GitLab",
			"alunos sem grupo em ordem no GitLab"))
	}
	for _, r := range p.Exercicios {
		if !r.Vencido {
			continue
		}
		if !r.Coletado {
			out = append(out, r.Exercicio.ID+" venceu e nunca foi coletado")
			continue
		}
		if r.Pendentes > 0 {
			out = append(out, r.Exercicio.ID+": "+plural(r.Pendentes,
				"aluno sem entrega no prazo", "alunos sem entrega no prazo"))
		}
		if r.SemNota > 0 {
			out = append(out, r.Exercicio.ID+": "+plural(r.SemNota,
				"entrega sem nota", "entregas sem nota"))
		}
		if r.VerificacoesVelhas > 0 {
			out = append(out, r.Exercicio.ID+": "+plural(r.VerificacoesVelhas,
				"verificação feita sobre commit antigo", "verificações feitas sobre commit antigo"))
		}
	}
	return out
}

func plural(n int, singular, plural string) string {
	if n == 1 {
		return "1 " + singular
	}
	return strconv.Itoa(n) + " " + plural
}
