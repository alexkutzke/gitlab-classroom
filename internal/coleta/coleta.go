// Package coleta percorre o GitLab e classifica a situação de cada aluno em
// cada exercício.
package coleta

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	gl "github.com/alexkutzke/gitlab-classroom/internal/gitlab"
	"github.com/alexkutzke/gitlab-classroom/internal/turma"
)

// Coletor executa a coleta contra um cliente do GitLab.
type Coletor struct {
	Cliente gl.Cliente
	Config  turma.Config
	// Progresso é chamado a cada aluno concluído, de qualquer goroutine.
	// Pode ser nil.
	Progresso func(feito, total int, aluno turma.Aluno)

	resultados *apuradas
}

// Resultado é o que a coleta apurou.
type Resultado struct {
	Entregas []turma.Entrega
	// Alunos traz o cadastro com grupo e situação da conta atualizados: a
	// coleta descobre isso de graça, e gravar poupa um sync depois.
	Alunos []turma.Aluno
}

// Reconciliar resolve o grupo e a situação da conta de cada aluno, sem olhar
// exercício nenhum. É o que o comando sync usa.
func (c *Coletor) Reconciliar(alunos []turma.Aluno) ([]turma.Aluno, error) {
	return c.executar(alunos, nil)
}

// Coletar apura as entregas dos alunos nos exercícios informados.
func (c *Coletor) Coletar(alunos []turma.Aluno, exercicios []turma.Exercicio) (Resultado, error) {
	atualizados, err := c.executar(alunos, exercicios)
	if err != nil {
		return Resultado{}, err
	}
	res := Resultado{Alunos: atualizados}
	for _, a := range atualizados {
		res.Entregas = append(res.Entregas, c.entregasDe(a)...)
	}
	// entregasDe guarda o que foi apurado por aluno durante executar; aqui só
	// se junta tudo em ordem estável.
	sort.SliceStable(res.Entregas, func(i, j int) bool {
		if res.Entregas[i].Exercicio != res.Entregas[j].Exercicio {
			return res.Entregas[i].Exercicio < res.Entregas[j].Exercicio
		}
		return res.Entregas[i].GRR < res.Entregas[j].GRR
	})
	return res, nil
}

// apuradas guarda as entregas por GRR entre executar e Coletar.
type apuradas struct {
	mu sync.Mutex
	m  map[string][]turma.Entrega
}

func (a *apuradas) guardar(grr string, es []turma.Entrega) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.m == nil {
		a.m = map[string][]turma.Entrega{}
	}
	a.m[grr] = es
}

func (c *Coletor) entregasDe(a turma.Aluno) []turma.Entrega {
	if c.resultados == nil {
		return nil
	}
	c.resultados.mu.Lock()
	defer c.resultados.mu.Unlock()
	return c.resultados.m[a.GRR]
}

// executar roda a reconciliação e, quando há exercícios, também a coleta,
// com um pool de trabalhadores do tamanho configurado.
func (c *Coletor) executar(alunos []turma.Aluno, exercicios []turma.Exercicio) ([]turma.Aluno, error) {
	if len(alunos) == 0 {
		return nil, nil
	}
	c.resultados = &apuradas{}

	n := c.Config.Paralelismo
	if n <= 0 {
		n = 8
	}
	if n > len(alunos) {
		n = len(alunos)
	}

	out := make([]turma.Aluno, len(alunos))
	copy(out, alunos)

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		feito   int
		indices = make(chan int)
	)

	for w := 0; w < n; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range indices {
				a := out[i]
				grupo, sit := c.resolverGrupo(a)
				a.Grupo, a.SituacaoConta, a.VerificadoEm = grupo, sit, time.Now()
				if len(exercicios) > 0 {
					c.resultados.guardar(a.GRR, c.coletarAluno(a, exercicios))
				}
				mu.Lock()
				out[i] = a
				feito++
				if c.Progresso != nil {
					c.Progresso(feito, len(alunos), a)
				}
				mu.Unlock()
			}
		}()
	}
	for i := range out {
		indices <- i
	}
	close(indices)
	wg.Wait()

	return out, nil
}

// resolverGrupo procura o grupo do aluno e classifica o que foi encontrado.
//
// A ordem das tentativas é: o grupo já gravado (que pode ter sido corrigido à
// mão), o nome do padrão, e por último a lista de grupos do professor, que é
// onde aparece quem batizou o grupo de outro jeito.
func (c *Coletor) resolverGrupo(a turma.Aluno) (string, turma.SituacaoConta) {
	candidatos := []string{}
	if a.Grupo != "" {
		candidatos = append(candidatos, a.Grupo)
	}
	if esperado := c.Config.CaminhoGrupo(a.GRR); esperado != "" && esperado != a.Grupo {
		candidatos = append(candidatos, esperado)
	}

	esperado := c.Config.CaminhoGrupo(a.GRR)
	for _, cand := range candidatos {
		g, err := c.Cliente.Grupo(cand)
		if err != nil || g == nil {
			continue
		}
		if !g.Membro {
			return g.Caminho, turma.ContaSemAcesso
		}
		if g.Caminho != esperado {
			return g.Caminho, turma.ContaGrupoDivergente
		}
		return g.Caminho, turma.ContaOK
	}

	// Grupo com nome fora do padrão: procura pelo GRR entre os grupos em que
	// o professor foi associado.
	if grupos, err := c.Cliente.GruposDoProfessor(); err == nil {
		login := turma.UsuarioGitLab(a.GRR)
		for _, g := range grupos {
			if strings.Contains(strings.ToLower(g.Caminho), login) {
				return g.Caminho, turma.ContaGrupoDivergente
			}
		}
	}

	if existe, err := c.Cliente.UsuarioExiste(turma.UsuarioGitLab(a.GRR)); err == nil && !existe {
		return "", turma.ContaSemUsuario
	}
	return "", turma.ContaGrupoInvisivel
}

// coletarAluno apura a entrega do aluno em cada exercício.
func (c *Coletor) coletarAluno(a turma.Aluno, exercicios []turma.Exercicio) []turma.Entrega {
	agora := time.Now()
	var out []turma.Entrega

	// Sem grupo acessível, o problema é anterior à entrega e vale para todos
	// os exercícios de uma vez.
	if sit, bloqueado := situacaoBloqueio(a.SituacaoConta); bloqueado {
		for _, e := range exercicios {
			out = append(out, turma.Entrega{
				Exercicio: e.ID, GRR: a.GRR, Situacao: sit,
				Projeto: a.Grupo, ColetadoEm: agora,
			})
		}
		return out
	}

	projetos, err := c.Cliente.ProjetosDoGrupo(a.Grupo)
	if err != nil {
		for _, e := range exercicios {
			out = append(out, turma.Entrega{
				Exercicio: e.ID, GRR: a.GRR, Situacao: turma.Erro,
				Detalhe: err.Error(), ColetadoEm: agora,
			})
		}
		return out
	}

	for _, e := range exercicios {
		out = append(out, c.coletarEntrega(a, e, projetos, agora))
	}
	return out
}

func (c *Coletor) coletarEntrega(a turma.Aluno, e turma.Exercicio, projetos []gl.Projeto, agora time.Time) turma.Entrega {
	base := turma.Entrega{Exercicio: e.ID, GRR: a.GRR, ColetadoEm: agora}

	p, ok := acharProjeto(projetos, e, c.Config)
	if !ok {
		base.Situacao = turma.SemFork
		return base
	}
	base.Projeto = p.Completo

	if p.Vazio {
		base.Situacao = turma.ForkSemCommit
		base.Detalhe = "repositório vazio"
		return base
	}

	modelo, err := c.shasDoModelo(e)
	if err != nil {
		base.Situacao = turma.Erro
		base.Detalhe = err.Error()
		return base
	}

	commits, err := c.Cliente.Commits(p.Completo, p.RamoPadrao, false)
	if err != nil {
		base.Situacao = turma.Erro
		base.Detalhe = err.Error()
		return base
	}

	proprios := semOsDoModelo(commits, modelo)
	if len(proprios) == 0 {
		base.Situacao = turma.ForkSemCommit
		// Aluno que trabalhou fora do ramo padrão perderia a entrega sem
		// nenhum aviso. Vale conferir antes de dizer que não fez nada.
		if todos, err := c.Cliente.Commits(p.Completo, "", true); err == nil {
			if fora := semOsDoModelo(todos, modelo); len(fora) > 0 {
				base.Detalhe = fmt.Sprintf("%d commit(s) fora do ramo %s", len(fora), p.RamoPadrao)
				base.Commits = len(fora)
			}
		}
		return base
	}

	sort.SliceStable(proprios, func(i, j int) bool { return proprios[i].Data.After(proprios[j].Data) })
	base.Commits = len(proprios)
	base.UltimoCommit = proprios[0].SHA
	base.DataUltimo = proprios[0].Data
	base.AtrasoDias = turma.AtrasoEmDias(e.Prazo, proprios[0].Data)

	limite := e.Prazo.FimDoDia()
	for _, cm := range proprios {
		if !cm.Data.After(limite) {
			base.Situacao = turma.Entregue
			base.Commit = cm.SHA
			base.DataCommit = cm.Data
			return base
		}
	}
	base.Situacao = turma.SemCommitNoPrazo
	return base
}

// shasDoModelo devolve os commits do repositório-modelo, para separar o que é
// trabalho do aluno do que veio junto no fork.
//
// Comparar com o modelo é mais confiável que filtrar por nome do autor, que
// era o que os scripts antigos faziam: o aluno que deixa o git configurado com
// outro nome, ou o modelo que recebeu commit de terceiro, quebravam o filtro.
func (c *Coletor) shasDoModelo(e turma.Exercicio) (map[string]bool, error) {
	caminho := c.Config.CaminhoModelo(e.Repo)
	commits, err := c.Cliente.Commits(caminho, "", true)
	if err != nil {
		return nil, err
	}
	m := make(map[string]bool, len(commits))
	for _, cm := range commits {
		m[cm.SHA] = true
	}
	return m, nil
}

func semOsDoModelo(commits []gl.Commit, modelo map[string]bool) []gl.Commit {
	var out []gl.Commit
	for _, c := range commits {
		if !modelo[c.SHA] {
			out = append(out, c)
		}
	}
	return out
}

// acharProjeto localiza o fork do exercício dentro do grupo do aluno.
//
// O nome costuma ser igual ao do modelo, mas o aluno pode renomear. Por isso
// a origem do fork também serve de chave.
func acharProjeto(projetos []gl.Projeto, e turma.Exercicio, cfg turma.Config) (gl.Projeto, bool) {
	alvo := strings.ToLower(e.Repo)
	for _, p := range projetos {
		if strings.ToLower(p.Caminho) == alvo {
			return p, true
		}
	}
	modelo := strings.ToLower(cfg.CaminhoModelo(e.Repo))
	for _, p := range projetos {
		if strings.ToLower(p.ForkDe) == modelo {
			return p, true
		}
	}
	for _, p := range projetos {
		if strings.Contains(strings.ToLower(p.Caminho), alvo) {
			return p, true
		}
	}
	return gl.Projeto{}, false
}

// situacaoBloqueio traduz um problema de conta em situação de entrega.
func situacaoBloqueio(s turma.SituacaoConta) (turma.SituacaoEntrega, bool) {
	switch s {
	case turma.ContaSemUsuario:
		return turma.SemConta, true
	case turma.ContaSemAcesso:
		return turma.SemAcesso, true
	case turma.ContaGrupoInvisivel:
		return turma.GrupoInvisivel, true
	}
	return "", false
}
