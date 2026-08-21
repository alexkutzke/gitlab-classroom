package gitlab

import (
	"errors"
	"sync"
	"testing"
)

func TestCacheBuscaUmaVezSo(t *testing.T) {
	c := novoCache[int]()
	buscas := 0
	for i := 0; i < 3; i++ {
		v, err := c.obter("grupos", func() (int, error) { buscas++; return 7, nil })
		if err != nil || v != 7 {
			t.Fatalf("v = %d, err = %v", v, err)
		}
	}
	if buscas != 1 {
		t.Errorf("buscas = %d, queria 1", buscas)
	}
}

func TestBuscasSimultaneasDaMesmaChaveViramUma(t *testing.T) {
	c := novoCache[int]()
	var mu sync.Mutex
	buscas := 0
	liberar := make(chan struct{})

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.obter("grupos", func() (int, error) {
				mu.Lock()
				buscas++
				mu.Unlock()
				<-liberar // segura a primeira busca até todos chegarem
				return 7, nil
			})
		}()
	}
	close(liberar)
	wg.Wait()

	if buscas != 1 {
		t.Errorf("buscas = %d, queria 1", buscas)
	}
}

func TestErroNaoFicaMemorizado(t *testing.T) {
	c := novoCache[int]()
	if _, err := c.obter("g", func() (int, error) { return 0, errors.New("rede") }); err == nil {
		t.Fatal("queria o erro de volta")
	}
	v, err := c.obter("g", func() (int, error) { return 7, nil })
	if err != nil || v != 7 {
		t.Errorf("v = %d, err = %v: a falha de rede não pode condenar a chave", v, err)
	}
}

func TestLimparForcaNovaBusca(t *testing.T) {
	c := novoCache[int]()
	buscar := func(n int) func() (int, error) {
		return func() (int, error) { return n, nil }
	}
	c.obter("grupos", buscar(1))
	c.limpar()

	v, err := c.obter("grupos", buscar(2))
	if err != nil {
		t.Fatal(err)
	}
	if v != 2 {
		t.Errorf("v = %d, queria 2: depois de limpar, o valor tem que vir do GitLab de novo", v)
	}
}
