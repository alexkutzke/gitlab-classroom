// Package coleta percorre o GitLab e classifica a situação de cada aluno em
// cada exercício.
package coleta

import (
	"context"
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
}

// Resultado é o que a coleta apurou.
type Resultado struct {
	Entregas []turma.Entrega
	// Alunos traz o cadastro com grupo e situação da conta atualizados: a
	// coleta descobre isso de graça, e gravar poupa um sync depois.
	Alunos []turma.Aluno
	// Vinculos são as entregas compartilhadas descobertas no GitLab, uma
	// linha por integrante que não é dono do fork.
	Vinculos []turma.Vinculo
}

// Reconciliar resolve o grupo e a situação da conta de cada aluno, sem olhar
// exercício nenhum. É o que o comando sync usa.
func (c *Coletor) Reconciliar(ctx context.Context, alunos []turma.Aluno) ([]turma.Aluno, error) {
	out := c.resolverGrupos(ctx, alunos, c.Progresso)
	return out, ctx.Err()
}

// Coletar apura as entregas dos alunos nos exercícios informados.
//
// São três fases: resolver o grupo de cada aluno, descobrir os forks
// compartilhados por mais de um aluno, e classificar cada entrega. A do meio
// existe porque a entrega em dupla mora no grupo de um só dos integrantes, e
// só a lista de membros do fork revela o outro.
func (c *Coletor) Coletar(ctx context.Context, alunos []turma.Aluno, exercicios []turma.Exercicio) (Resultado, error) {
	atualizados := c.resolverGrupos(ctx, alunos, nil)
	if err := ctx.Err(); err != nil {
		return Resultado{}, err
	}
	equipes := c.descobrirEquipes(atualizados, exercicios)

	entregas, vinculos := c.coletarTodos(ctx, atualizados, exercicios, equipes)
	if err := ctx.Err(); err != nil {
		// Coleta interrompida deixaria fora quem não foi visitado, e aplicar
		// isso apagaria a entrega deles. Melhor não devolver nada.
		return Resultado{}, err
	}
	return Resultado{Entregas: entregas, Alunos: atualizados, Vinculos: vinculos}, nil
}

// --- fase 1: grupos ---

// resolverGrupos descobre o grupo e a situação da conta de cada aluno, em
// paralelo.
func (c *Coletor) resolverGrupos(ctx context.Context, alunos []turma.Aluno, progresso func(int, int, turma.Aluno)) []turma.Aluno {
	out := make([]turma.Aluno, len(alunos))
	copy(out, alunos)

	c.emParalelo(ctx, len(out), func(i int) {
		a := out[i]
		grupo, sit := c.resolverGrupo(a)
		a.Grupo, a.SituacaoConta, a.VerificadoEm = grupo, sit, time.Now()
		out[i] = a
	}, func(feito, total, i int) {
		if progresso != nil {
			progresso(feito, total, out[i])
		}
	})
	return out
}

// resolverGrupo procura o grupo do aluno e classifica o que foi encontrado.
//
// A ordem das tentativas é: o grupo já gravado (que pode ter sido fixado à
// mão), o nome do padrão com o GRR, o nome do padrão com o usuário cadastrado
// (que difere do GRR quando o aluno não conseguiu criar a conta com ele), e
// por último a lista de grupos do professor, que é onde aparece quem batizou
// o grupo de outro jeito.
func (c *Coletor) resolverGrupo(a turma.Aluno) (string, turma.SituacaoConta) {
	esperados := c.esperados(a)

	var candidatos []string
	if a.Grupo != "" {
		candidatos = append(candidatos, a.Grupo)
	}
	for _, e := range esperados {
		if e != "" && e != a.Grupo {
			candidatos = append(candidatos, e)
		}
	}

	for _, cand := range candidatos {
		g, err := c.Cliente.Grupo(cand)
		if err != nil || g == nil {
			continue
		}
		if !g.Membro {
			return g.Caminho, turma.ContaSemAcesso
		}
		if !contem(esperados, g.Caminho) {
			return g.Caminho, turma.ContaGrupoDivergente
		}
		return g.Caminho, turma.ContaOK
	}

	// Grupo com nome fora do padrão: procura entre os grupos em que o
	// professor foi associado, tanto pelo GRR quanto pelo usuário cadastrado.
	if grupos, err := c.Cliente.GruposDoProfessor(); err == nil {
		chaves := []string{turma.UsuarioGitLab(a.GRR), turma.UsuarioGitLab(a.UsuarioEsperado())}
		for _, g := range grupos {
			caminho := strings.ToLower(g.Caminho)
			for _, k := range chaves {
				if k != "" && strings.Contains(caminho, k) {
					return g.Caminho, turma.ContaGrupoDivergente
				}
			}
		}
	}

	if existe, err := c.Cliente.UsuarioExiste(a.UsuarioEsperado()); err == nil && !existe {
		return "", turma.ContaSemUsuario
	}
	return "", turma.ContaGrupoInvisivel
}

// esperados devolve os nomes de grupo aceitos como dentro do padrão para o
// aluno. São dois quando o usuário cadastrado difere do GRR: o aluno que
// precisou de outro login pode ter batizado o grupo com qualquer um dos dois.
func (c *Coletor) esperados(a turma.Aluno) []string {
	out := []string{c.Config.CaminhoGrupo(a.GRR)}
	if u := a.UsuarioEsperado(); turma.UsuarioGitLab(u) != turma.UsuarioGitLab(a.GRR) {
		out = append(out, c.Config.CaminhoGrupo(u))
	}
	return out
}

// --- fase 2: equipes ---

// convite é a possibilidade de o aluno ter entregado dentro do fork de outro.
type convite struct {
	Dono    turma.Aluno
	Projeto gl.Projeto
}

// equipes indexa, por exercício e por GRR do integrante, o fork de outro
// aluno em que ele foi adicionado como membro.
type equipes map[string]map[string]convite

func (e equipes) convite(exercicio, grr string) (convite, bool) {
	if e == nil {
		return convite{}, false
	}
	c, ok := e[exercicio][grr]
	return c, ok
}

// descobrirEquipes lista os forks de cada repositório-modelo e, para cada um,
// quem são os membros.
//
// Nas tarefas em dupla, só um dos dois faz o fork, no grupo dele, e adiciona o
// colega como membro do projeto. Procurar apenas no grupo de cada aluno
// deixaria o colega marcado como quem não entregou, que é falso e chega ao
// aluno pelo relatório publicado.
//
// Falha aqui não interrompe a coleta: sem a descoberta, cada aluno é avaliado
// pelo próprio grupo, que era o comportamento anterior.
func (c *Coletor) descobrirEquipes(alunos []turma.Aluno, exercicios []turma.Exercicio) equipes {
	if len(exercicios) == 0 {
		return nil
	}
	porUsuario := map[string]turma.Aluno{}
	porGrupo := map[string]turma.Aluno{}
	for _, a := range alunos {
		porUsuario[turma.UsuarioGitLab(a.UsuarioEsperado())] = a
		porUsuario[turma.UsuarioGitLab(a.GRR)] = a
		if a.Grupo != "" {
			porGrupo[strings.ToLower(a.Grupo)] = a
		}
	}

	out := equipes{}
	for _, e := range exercicios {
		forks, err := c.Cliente.Forks(c.Config.CaminhoModelo(e.Repo))
		if err != nil || len(forks) == 0 {
			continue
		}
		doExercicio := map[string]convite{}
		for _, f := range forks {
			dono, ok := donoDoFork(f, porGrupo, porUsuario)
			if !ok {
				continue
			}
			membros, err := c.Cliente.Membros(f.Completo)
			if err != nil {
				continue
			}
			for _, m := range membros {
				a, ok := porUsuario[turma.UsuarioGitLab(m.Usuario)]
				if !ok || a.GRR == dono.GRR {
					continue
				}
				doExercicio[a.GRR] = convite{Dono: dono, Projeto: f}
			}
		}
		if len(doExercicio) > 0 {
			out[e.ID] = doExercicio
		}
	}
	return out
}

// donoDoFork identifica de quem é o grupo onde o fork está.
func donoDoFork(f gl.Projeto, porGrupo, porUsuario map[string]turma.Aluno) (turma.Aluno, bool) {
	espaco, _, ok := strings.Cut(f.Completo, "/")
	if !ok {
		return turma.Aluno{}, false
	}
	espaco = strings.ToLower(espaco)
	if a, ok := porGrupo[espaco]; ok {
		return a, true
	}
	// Grupo ainda não resolvido para nenhum aluno: o nome do padrão termina
	// com o GRR, então procurar o login dentro do caminho resolve.
	for login, a := range porUsuario {
		if login != "" && strings.Contains(espaco, login) {
			return a, true
		}
	}
	return turma.Aluno{}, false
}

// --- fase 3: entregas ---

func (c *Coletor) coletarTodos(ctx context.Context, alunos []turma.Aluno, exercicios []turma.Exercicio, eq equipes) ([]turma.Entrega, []turma.Vinculo) {
	porAluno := make([][]turma.Entrega, len(alunos))
	vinculosPorAluno := make([][]turma.Vinculo, len(alunos))

	c.emParalelo(ctx, len(alunos), func(i int) {
		porAluno[i], vinculosPorAluno[i] = c.coletarAluno(alunos[i], exercicios, eq)
	}, func(feito, total, i int) {
		if c.Progresso != nil {
			c.Progresso(feito, total, alunos[i])
		}
	})

	var entregas []turma.Entrega
	var vinculos []turma.Vinculo
	for i := range alunos {
		entregas = append(entregas, porAluno[i]...)
		vinculos = append(vinculos, vinculosPorAluno[i]...)
	}
	sort.SliceStable(entregas, func(i, j int) bool {
		if entregas[i].Exercicio != entregas[j].Exercicio {
			return entregas[i].Exercicio < entregas[j].Exercicio
		}
		return entregas[i].GRR < entregas[j].GRR
	})
	sort.SliceStable(vinculos, func(i, j int) bool {
		if vinculos[i].Exercicio != vinculos[j].Exercicio {
			return vinculos[i].Exercicio < vinculos[j].Exercicio
		}
		return vinculos[i].GRR < vinculos[j].GRR
	})
	return entregas, vinculos
}

// coletarAluno apura a entrega do aluno em cada exercício.
func (c *Coletor) coletarAluno(a turma.Aluno, exercicios []turma.Exercicio, eq equipes) ([]turma.Entrega, []turma.Vinculo) {
	agora := time.Now()
	var entregas []turma.Entrega
	var vinculos []turma.Vinculo

	projetos, erroDoGrupo := c.projetosDoAluno(a)

	for _, e := range exercicios {
		entrega, achou := c.coletarNoGrupo(a, e, projetos, erroDoGrupo, agora)
		if achou {
			entregas = append(entregas, entrega)
			continue
		}
		// Sem fork no próprio grupo: pode ser entrega em dupla, dentro do
		// fork de quem adicionou o aluno como membro.
		if conv, ok := eq.convite(e.ID, a.GRR); ok {
			compartilhada := c.coletarNoProjeto(a, e, conv.Projeto, agora)
			compartilhada.Detalhe = "entrega compartilhada, fork de " + conv.Dono.Nome
			entregas = append(entregas, compartilhada)
			vinculos = append(vinculos, turma.Vinculo{
				Exercicio: e.ID, GRR: a.GRR, Dono: conv.Dono.GRR,
				Origem: turma.VinculoDescoberto, AtualizadoEm: agora,
			})
			continue
		}
		entregas = append(entregas, entrega)
	}
	return entregas, vinculos
}

// projetosDoAluno devolve os repositórios do grupo do aluno, ou o erro que
// impediu de olhar.
func (c *Coletor) projetosDoAluno(a turma.Aluno) ([]gl.Projeto, error) {
	if _, bloqueado := situacaoBloqueio(a.SituacaoConta); bloqueado {
		return nil, nil
	}
	return c.Cliente.ProjetosDoGrupo(a.Grupo)
}

// coletarNoGrupo classifica a entrega olhando só o grupo do aluno. O segundo
// retorno diz se o veredito está fechado; quando não está, quem chama ainda
// procura uma entrega compartilhada antes de aceitá-lo.
func (c *Coletor) coletarNoGrupo(a turma.Aluno, e turma.Exercicio, projetos []gl.Projeto, erroDoGrupo error, agora time.Time) (turma.Entrega, bool) {
	base := turma.Entrega{Exercicio: e.ID, GRR: a.GRR, ColetadoEm: agora}

	if sit, bloqueado := situacaoBloqueio(a.SituacaoConta); bloqueado {
		base.Situacao, base.Projeto = sit, a.Grupo
		// Aluno sem grupo próprio ainda pode ter entregado no fork do colega.
		return base, false
	}
	if erroDoGrupo != nil {
		base.Situacao, base.Detalhe = turma.Erro, erroDoGrupo.Error()
		return base, true
	}

	p, ok := acharProjeto(projetos, e, c.Config)
	if !ok {
		base.Situacao = turma.SemFork
		return base, false
	}
	return c.coletarNoProjeto(a, e, p, agora), true
}

// coletarNoProjeto apura a entrega de um aluno dentro de um fork concreto.
func (c *Coletor) coletarNoProjeto(a turma.Aluno, e turma.Exercicio, p gl.Projeto, agora time.Time) turma.Entrega {
	base := turma.Entrega{
		Exercicio: e.ID, GRR: a.GRR, ColetadoEm: agora, Projeto: p.Completo,
	}

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

// emParalelo roda tarefa sobre os índices de 0 a n, com o pool do tamanho
// configurado, chamando concluido a cada item terminado.
func (c *Coletor) emParalelo(ctx context.Context, n int, tarefa func(i int), concluido func(feito, total, i int)) {
	if n == 0 {
		return
	}
	trabalhadores := c.Config.Paralelismo
	if trabalhadores <= 0 {
		trabalhadores = 8
	}
	if trabalhadores > n {
		trabalhadores = n
	}

	indices := make(chan int)
	var wg sync.WaitGroup
	var mu sync.Mutex
	feito := 0

	for w := 0; w < trabalhadores; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range indices {
				if ctx.Err() != nil {
					continue // desiste do que falta, sem matar o que já roda
				}
				tarefa(i)
				mu.Lock()
				feito++
				concluido(feito, n, i)
				mu.Unlock()
			}
		}()
	}
	for i := 0; i < n; i++ {
		indices <- i
	}
	close(indices)
	wg.Wait()
}

func contem(lista []string, valor string) bool {
	for _, v := range lista {
		if v == valor {
			return true
		}
	}
	return false
}
