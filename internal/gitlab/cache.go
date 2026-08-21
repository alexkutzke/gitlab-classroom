package gitlab

import "sync"

// cache guarda o resultado de uma listagem por chave e garante que dois
// pedidos simultâneos da mesma chave façam uma requisição só.
//
// Sem isso, os oito trabalhadores da coleta partem juntos e cada um busca a
// listagem inteira: no caso dos grupos do professor, que hoje passam de
// quinhentos, são seis páginas multiplicadas por oito, e o comando fica
// dezenas de segundos parado antes de dar o primeiro sinal de vida.
type cache[T any] struct {
	mu      sync.Mutex
	valores map[string]T
	emCurso map[string]chan struct{}
}

func novoCache[T any]() *cache[T] {
	return &cache[T]{
		valores: map[string]T{},
		emCurso: map[string]chan struct{}{},
	}
}

// obter devolve o valor da chave, buscando-o só uma vez.
//
// Erro não é memorizado: uma falha de rede não pode condenar a chave pelo
// resto da sessão.
func (c *cache[T]) obter(chave string, buscar func() (T, error)) (T, error) {
	for {
		c.mu.Lock()
		if v, ok := c.valores[chave]; ok {
			c.mu.Unlock()
			return v, nil
		}
		if espera, ok := c.emCurso[chave]; ok {
			c.mu.Unlock()
			<-espera // outro trabalhador já foi buscar; aproveita o resultado
			continue
		}
		espera := make(chan struct{})
		c.emCurso[chave] = espera
		c.mu.Unlock()

		v, err := buscar()

		c.mu.Lock()
		delete(c.emCurso, chave)
		if err == nil {
			c.valores[chave] = v
		}
		c.mu.Unlock()
		close(espera)

		return v, err
	}
}
