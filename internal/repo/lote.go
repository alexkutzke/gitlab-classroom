package repo

import (
	"sync"
)

// Alvo é um clone a preparar.
type Alvo struct {
	Exercicio string
	GRR       string
	Nome      string
	Projeto   string // caminho completo no GitLab
	Commit    string // commit avaliado; vazio usa o ramo padrão
	Dir       string
	// URL sobrepõe o endereço derivado de Projeto. Existe para os testes
	// apontarem para um repositório local, sem rede.
	URL string
}

// Endereco devolve a URL de clone do alvo.
func (a Alvo) Endereco(host string) string {
	if a.URL != "" {
		return a.URL
	}
	return URLSSH(host, a.Projeto)
}

// Resultado é o que aconteceu com um alvo.
type Resultado struct {
	Alvo
	// Novo indica que o clone não existia antes desta execução.
	Novo bool
	// HEAD é o commit em que o clone ficou posicionado.
	HEAD string
	Erro error
}

// Sincronizar clona ou atualiza os alvos em paralelo e posiciona cada clone
// no commit avaliado.
//
// O paralelismo é o mesmo da coleta: são operações de rede curtas, e fazê-las
// em série numa turma de trinta alunos era o que tornava lento o script
// antigo.
func Sincronizar(alvos []Alvo, host string, paralelismo int, progresso func(feito, total int, a Alvo)) []Resultado {
	if len(alvos) == 0 {
		return nil
	}
	if paralelismo <= 0 {
		paralelismo = 8
	}
	if paralelismo > len(alvos) {
		paralelismo = len(alvos)
	}

	out := make([]Resultado, len(alvos))
	indices := make(chan int)
	var wg sync.WaitGroup
	var mu sync.Mutex
	feito := 0

	for w := 0; w < paralelismo; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range indices {
				a := alvos[i]
				res := Resultado{Alvo: a, Novo: !Existe(a.Dir)}

				r, err := Preparar(a.Dir, a.Endereco(host))
				if err != nil {
					res.Erro = err
				} else if head, err := r.Posicionar(NomeRamo(a.Exercicio), a.Commit); err != nil {
					res.Erro = err
				} else {
					res.HEAD = head
				}

				mu.Lock()
				out[i] = res
				feito++
				if progresso != nil {
					progresso(feito, len(alvos), a)
				}
				mu.Unlock()
			}
		}()
	}
	for i := range alvos {
		indices <- i
	}
	close(indices)
	wg.Wait()
	return out
}
