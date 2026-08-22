package turma

import "strings"

// Sugerir tenta descobrir de quem é um login do GitLab que não está no
// cadastro, comparando o nome de exibição da conta e o próprio login com os
// nomes da turma.
//
// Só devolve resposta quando há um candidato único. Chute de nota é pior que
// silêncio: o professor confirma antes de gravar o login com
// `classroom alunos editar --usuario`.
func Sugerir(alunos []Aluno, nome, usuario string) (Aluno, bool) {
	procurados := partes(nome + " " + strings.NewReplacer(".", " ", "_", " ", "-", " ").Replace(usuario))
	if len(procurados) == 0 {
		return Aluno{}, false
	}

	melhor, empate, pontos := Aluno{}, false, 0
	for _, a := range alunos {
		n := comuns(partes(a.Nome), procurados)
		switch {
		case n > pontos:
			melhor, empate, pontos = a, false, n
		case n == pontos && n > 0:
			empate = true
		}
	}
	// Um pedaço em comum é fraco demais: "silva" e "eduardo" repetem na turma.
	if pontos < 2 || empate {
		return Aluno{}, false
	}
	return melhor, true
}

// partes quebra um nome nos pedaços que valem comparar, fora as partículas.
func partes(nome string) []string {
	var out []string
	for _, p := range strings.Fields(ChaveNome(nome)) {
		if len(p) < 3 || particulas[p] {
			continue
		}
		out = append(out, p)
	}
	return out
}

var particulas = map[string]bool{
	"de": true, "da": true, "do": true, "das": true, "dos": true, "e": true,
	"del": true, "los": true, "las": true,
}

func comuns(a, b []string) int {
	n := 0
	for _, x := range a {
		for _, y := range b {
			if x == y {
				n++
				break
			}
		}
	}
	return n
}
