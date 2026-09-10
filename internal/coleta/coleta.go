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
	// Progresso é chamado a cada item concluído, de qualquer goroutine. O
	// rótulo diz em que fase a coleta está, porque as três levam tempo e o
	// silêncio de uma delas parece travamento.
	Progresso func(feito, total int, rotulo string)
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
	// Desconhecidos são os membros de fork que não casam com nenhum aluno.
	Desconhecidos []MembroDesconhecido
}

// MembroDesconhecido é alguém adicionado ao fork de um aluno cujo login não
// corresponde a nenhum cadastro da turma.
//
// Costuma ser o colega de dupla que não conseguiu criar a conta com o GRR e
// usou outro login. Enquanto o login não for cadastrado, a entrega em dupla
// não é atribuída a ele, e o relatório publicado o mostra como quem não
// entregou.
type MembroDesconhecido struct {
	Exercicio string
	Usuario   string
	Nome      string
	Projeto   string
	// DonoGRR e DonoNome identificam o aluno em cujo fork o membro está.
	DonoGRR  string
	DonoNome string
}

// Reconciliar resolve o grupo e a situação da conta de cada aluno, sem olhar
// exercício nenhum. É o que o comando sync usa.
func (c *Coletor) Reconciliar(ctx context.Context, alunos []turma.Aluno) ([]turma.Aluno, error) {
	out := c.resolverGrupos(ctx, alunos, "")
	return out, ctx.Err()
}

// Coletar apura as entregas dos alunos nos exercícios informados.
//
// São três fases: resolver o grupo de cada aluno, descobrir os forks
// compartilhados por mais de um aluno, e classificar cada entrega. A do meio
// existe porque a entrega em dupla mora no grupo de um só dos integrantes, e
// só a lista de membros do fork revela o outro.
func (c *Coletor) Coletar(ctx context.Context, alunos []turma.Aluno, exercicios []turma.Exercicio) (Resultado, error) {
	atualizados := c.resolverGrupos(ctx, alunos, "grupos")
	if err := ctx.Err(); err != nil {
		return Resultado{}, err
	}
	equipes, desconhecidos := c.descobrirEquipes(ctx, atualizados, exercicios)
	if err := ctx.Err(); err != nil {
		return Resultado{}, err
	}

	entregas, vinculos := c.coletarTodos(ctx, atualizados, exercicios, equipes)
	if err := ctx.Err(); err != nil {
		// Coleta interrompida deixaria fora quem não foi visitado, e aplicar
		// isso apagaria a entrega deles. Melhor não devolver nada.
		return Resultado{}, err
	}
	return Resultado{
		Entregas: entregas, Alunos: atualizados,
		Vinculos: vinculos, Desconhecidos: desconhecidos,
	}, nil
}

// --- fase 1: grupos ---

// resolverGrupos descobre o grupo e a situação da conta de cada aluno, em
// paralelo.
func (c *Coletor) resolverGrupos(ctx context.Context, alunos []turma.Aluno, fase string) []turma.Aluno {
	out := make([]turma.Aluno, len(alunos))
	copy(out, alunos)

	c.emParalelo(ctx, len(out), func(i int) {
		a := out[i]
		grupo, sit := c.resolverGrupo(a)
		a.Grupo, a.SituacaoConta, a.VerificadoEm = grupo, sit, time.Now()
		out[i] = a
	}, func(feito, total, i int) {
		c.avisar(feito, total, fase, out[i].Nome)
	})
	return out
}

// avisar monta o rótulo do progresso, com a fase na frente quando há mais de
// uma.
func (c *Coletor) avisar(feito, total int, fase, item string) {
	if c.Progresso == nil {
		return
	}
	rotulo := item
	if fase != "" {
		rotulo = fase + ": " + item
	}
	c.Progresso(feito, total, rotulo)
}

// resolverGrupo procura o grupo do aluno e classifica o que foi encontrado.
//
// A ordem das tentativas é: o nome do padrão com o GRR, o nome do padrão com
// o usuário cadastrado (que difere do GRR quando o aluno não conseguiu criar
// a conta com ele), o grupo já gravado, e por último a lista de grupos do
// professor, que é onde aparece quem batizou o grupo de outro jeito.
//
// O nome esperado vem antes do gravado por causa do aluno que erra o nome,
// descobre o erro e cria um grupo novo em vez de mudar a URL do primeiro. Os
// dois passam a existir, e insistir no gravado deixaria a coleta presa no
// grupo abandonado enquanto o fork está no outro. Fixar à mão um nome fora do
// padrão continua valendo: nesse caso não há grupo com o nome esperado para
// competir.
func (c *Coletor) resolverGrupo(a turma.Aluno) (string, turma.SituacaoConta) {
	esperados := c.esperados(a)

	var candidatos []string
	for _, e := range esperados {
		if e != "" {
			candidatos = append(candidatos, e)
		}
	}
	if a.Grupo != "" && !contem(esperados, a.Grupo) {
		candidatos = append(candidatos, a.Grupo)
	}

	// A listagem dos grupos da turma responde de uma vez, para todos os
	// alunos, a pergunta cara: em quais deles o professor é reporter.
	meus := c.gruposDaTurma()
	for _, cand := range candidatos {
		g, ok := meus[strings.ToLower(cand)]
		if !ok {
			continue
		}
		if contem(esperados, g.Caminho) {
			return g.Caminho, turma.ContaOK
		}
		// Associação a um grupo fora do padrão. Antes de aceitar, conferir se
		// existe um com o nome esperado: o aluno que erra o nome costuma
		// criar um grupo novo em vez de mudar a URL, e esquecer de repetir o
		// convite. O trabalho fica no grupo novo, e apontar para o antigo
		// esconderia a entrega.
		if outro := c.grupoEsperadoVisivel(esperados); outro != "" {
			return outro, turma.ContaSemAcesso
		}
		return g.Caminho, turma.ContaGrupoDivergente
	}

	// Fora da listagem da turma: o grupo pode ter nome que não casa com o
	// prefixo, ou existir sem a associação do professor. A consulta direta,
	// mais a checagem dirigida de associação, separa os dois casos sem gastar
	// busca por texto.
	for _, cand := range candidatos {
		g, err := c.Cliente.Grupo(cand)
		if err != nil || g == nil {
			continue
		}
		membro, err := c.Cliente.MembroDoGrupo(g.Caminho)
		if err != nil || !membro {
			return g.Caminho, turma.ContaSemAcesso
		}
		if !contem(esperados, g.Caminho) {
			return g.Caminho, turma.ContaGrupoDivergente
		}
		return g.Caminho, turma.ContaOK
	}

	// Sobrou o grupo com nome fora do padrão. A busca por texto vem antes da
	// checagem da conta porque o aluno que não conseguiu criar a conta com o
	// GRR ainda pode ter um grupo com o GRR no nome: perguntar primeiro pela
	// conta o marcaria como sem conta e esconderia a entrega dele.
	if login := turma.UsuarioGitLab(a.GRR); login != "" {
		if grupos, err := c.Cliente.GruposComAcesso(login); err == nil {
			for _, g := range grupos {
				if strings.Contains(strings.ToLower(g.Caminho), login) {
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

// grupoEsperadoVisivel devolve o primeiro nome do padrão que existe no
// GitLab, mesmo sem o professor associado. Só é consultado para o aluno cuja
// associação caiu num grupo fora do padrão, que são poucos por turma.
func (c *Coletor) grupoEsperadoVisivel(esperados []string) string {
	for _, e := range esperados {
		if e == "" {
			continue
		}
		if g, err := c.Cliente.Grupo(e); err == nil && g != nil {
			return g.Caminho
		}
	}
	return ""
}

// gruposDaTurma devolve, por caminho em minúsculas, os grupos da turma em que
// o professor tem acesso de reporter ou mais.
func (c *Coletor) gruposDaTurma() map[string]gl.Grupo {
	grupos, err := c.Cliente.GruposComAcesso(c.Config.PrefixoGrupo())
	if err != nil {
		return nil
	}
	out := make(map[string]gl.Grupo, len(grupos))
	for _, g := range grupos {
		out[strings.ToLower(g.Caminho)] = g
	}
	return out
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

// descobrirEquipes procura, nos forks que os alunos têm em seus grupos, quem
// mais foi adicionado como membro.
//
// Nas tarefas em dupla, só um dos dois faz o fork, no grupo dele, e adiciona o
// colega como membro do projeto. Procurar apenas no grupo de cada aluno
// deixaria o colega marcado como quem não entregou, que é falso e chega ao
// aluno pelo relatório publicado.
//
// A varredura é pelos grupos da turma, e não pelos forks do repositório
// modelo: o modelo acumula os forks de todos os semestres, e listá-los custa
// dezenas de segundos para depois descartar quase tudo. Os projetos de cada
// grupo já são consultados na fase seguinte, então aqui eles saem do cache.
//
// Falha aqui não interrompe a coleta: sem a descoberta, cada aluno é avaliado
// pelo próprio grupo, que era o comportamento anterior.
func (c *Coletor) descobrirEquipes(ctx context.Context, alunos []turma.Aluno, exercicios []turma.Exercicio) (equipes, []MembroDesconhecido) {
	if len(exercicios) == 0 {
		return nil, nil
	}
	porUsuario := map[string]turma.Aluno{}
	for _, a := range alunos {
		porUsuario[turma.UsuarioGitLab(a.UsuarioEsperado())] = a
		porUsuario[turma.UsuarioGitLab(a.GRR)] = a
	}
	// O dono do token entra na lista de membros de todo fork, por herança do
	// grupo, e não é aluno nenhum.
	eu, _ := c.Cliente.Eu()

	comGrupo := make([]turma.Aluno, 0, len(alunos))
	for _, a := range alunos {
		if a.Grupo != "" {
			comGrupo = append(comGrupo, a)
		}
	}
	if len(comGrupo) == 0 {
		return nil, nil
	}

	var desconhecidos []MembroDesconhecido
	out := equipes{}
	for _, e := range exercicios {
		type achado struct {
			fork    gl.Projeto
			dono    turma.Aluno
			membros []gl.Membro
		}
		achados := make([]achado, len(comGrupo))

		c.emParalelo(ctx, len(comGrupo), func(i int) {
			a := comGrupo[i]
			projetos, err := c.Cliente.ProjetosDoGrupo(a.Grupo)
			if err != nil {
				return
			}
			p, ok := acharProjeto(projetos, e, c.Config)
			if !ok {
				return
			}
			membros, err := c.Cliente.Membros(p.Completo)
			if err != nil {
				return
			}
			achados[i] = achado{fork: p, dono: a, membros: membros}
		}, func(feito, total, i int) {
			c.avisar(feito, total, "duplas em "+e.ID, comGrupo[i].Nome)
		})

		doExercicio := map[string]convite{}
		for _, ac := range achados {
			if ac.dono.GRR == "" {
				continue
			}
			for _, m := range ac.membros {
				login := turma.UsuarioGitLab(m.Usuario)
				colega, ok := porUsuario[login]
				if !ok {
					if login != turma.UsuarioGitLab(eu) {
						desconhecidos = append(desconhecidos, MembroDesconhecido{
							Exercicio: e.ID, Usuario: m.Usuario, Nome: m.Nome,
							Projeto: ac.fork.Completo,
							DonoGRR: ac.dono.GRR, DonoNome: ac.dono.Nome,
						})
					}
					continue
				}
				if colega.GRR == ac.dono.GRR {
					continue
				}
				doExercicio[colega.GRR] = convite{Dono: ac.dono, Projeto: ac.fork}
			}
		}
		if len(doExercicio) > 0 {
			out[e.ID] = doExercicio
		}
	}
	ordenarDesconhecidos(desconhecidos)
	return out, desconhecidos
}

// ordenarDesconhecidos deixa a lista estável para o comando e o relatório não
// mudarem de ordem entre execuções.
func ordenarDesconhecidos(ds []MembroDesconhecido) {
	sort.SliceStable(ds, func(i, j int) bool {
		if ds[i].Exercicio != ds[j].Exercicio {
			return ds[i].Exercicio < ds[j].Exercicio
		}
		if ds[i].DonoNome != ds[j].DonoNome {
			return ds[i].DonoNome < ds[j].DonoNome
		}
		return ds[i].Usuario < ds[j].Usuario
	})
}

// MembrosDesconhecidos varre os forks da turma e devolve só os membros que
// não casam com nenhum aluno. É a fase 2 da coleta, isolada, para o professor
// poder rodar sem mexer no que já foi apurado.
func (c *Coletor) MembrosDesconhecidos(ctx context.Context, alunos []turma.Aluno, exercicios []turma.Exercicio) ([]MembroDesconhecido, error) {
	comGrupo := c.resolverGrupos(ctx, alunos, "grupos")
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	_, desconhecidos := c.descobrirEquipes(ctx, comGrupo, exercicios)
	return desconhecidos, ctx.Err()
}

// --- fase 3: entregas ---

func (c *Coletor) coletarTodos(ctx context.Context, alunos []turma.Aluno, exercicios []turma.Exercicio, eq equipes) ([]turma.Entrega, []turma.Vinculo) {
	porAluno := make([][]turma.Entrega, len(alunos))
	vinculosPorAluno := make([][]turma.Vinculo, len(alunos))

	c.emParalelo(ctx, len(alunos), func(i int) {
		porAluno[i], vinculosPorAluno[i] = c.coletarAluno(alunos[i], exercicios, eq)
	}, func(feito, total, i int) {
		c.avisar(feito, total, "entregas", alunos[i].Nome)
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
		conv, temConvite := eq.convite(e.ID, a.GRR)
		// Sem fork no próprio grupo, ou com fork que não rendeu entrega: pode
		// ser entrega em dupla, dentro do fork de quem adicionou o aluno como
		// membro.
		if !temConvite || (achou && !podeMelhorar(entrega.Situacao)) {
			entregas = append(entregas, entrega)
			continue
		}

		compartilhada := c.coletarNoProjeto(a, e, conv.Projeto, agora)
		compartilhada.Detalhe = "entrega compartilhada, fork de " + conv.Dono.Nome
		// Empate fica com o fork próprio: é o repositório do aluno, e trocá-lo
		// por um alheio de mesmo veredito só confundiria a correção.
		if achou && precedencia(compartilhada.Situacao) >= precedencia(entrega.Situacao) {
			entregas = append(entregas, entrega)
			continue
		}
		if achou {
			// O fork abandonado explica por que o clone do aluno pode apontar
			// para outro lugar do que ele mesmo criou.
			compartilhada.Detalhe += " (fork próprio em " + entrega.Situacao.Rotulo() + ")"
		}
		entregas = append(entregas, compartilhada)
		vinculos = append(vinculos, turma.Vinculo{
			Exercicio: e.ID, GRR: a.GRR, Dono: conv.Dono.GRR,
			Origem: turma.VinculoDescoberto, AtualizadoEm: agora,
		})
	}
	return entregas, vinculos
}

// podeMelhorar diz se vale consultar o convite mesmo com o fork próprio já
// classificado.
//
// O aluno que bifurca por conta e depois vai trabalhar no repositório do colega
// deixa para trás um fork vazio, ou só com os commits do modelo. Fechar o
// veredito nesse fork faria a entrega em dupla passar despercebida, que foi o
// que aconteceu no exercício prepare de 2026/2. O fork que rende entrega, e o
// veredito que não é sobre o repositório (erro, bloqueio de conta), encerram a
// busca.
func podeMelhorar(s turma.SituacaoEntrega) bool {
	return s == turma.ForkSemCommit || s == turma.SemCommitNoPrazo
}

// precedencia ordena as situações da melhor para a pior, para escolher entre o
// fork próprio e o do colega. As de fora da escala (erro e bloqueio de conta)
// ficam por último: não são veredito sobre o trabalho, e não devem ganhar de
// nada.
func precedencia(s turma.SituacaoEntrega) int {
	switch s {
	case turma.Entregue:
		return 0
	case turma.SemCommitNoPrazo:
		return 1
	case turma.ForkSemCommit:
		return 2
	case turma.SemFork:
		return 3
	}
	return 99
}

// projetosDoAluno devolve os repositórios do grupo do aluno, ou o erro que
// impediu de olhar.
func (c *Coletor) projetosDoAluno(a turma.Aluno) ([]gl.Projeto, error) {
	if a.Grupo == "" {
		return nil, nil
	}
	// sem_acesso não impede de olhar: grupo sem a associação do professor
	// costuma continuar legível quando o repositório é fork de um modelo da
	// disciplina, e nesse caso a entrega conta.
	if sit, bloqueado := situacaoBloqueio(a.SituacaoConta); bloqueado && sit != turma.SemAcesso {
		return nil, nil
	}
	return c.Cliente.ProjetosDoGrupo(a.Grupo)
}

// coletarNoGrupo classifica a entrega olhando só o grupo do aluno. O segundo
// retorno diz se o fork foi encontrado ali; quando não foi, quem chama adota a
// entrega compartilhada se houver convite. Encontrar o fork não fecha o
// veredito sozinho: se ele não rendeu entrega, quem chama ainda compara com o
// fork do colega.
func (c *Coletor) coletarNoGrupo(a turma.Aluno, e turma.Exercicio, projetos []gl.Projeto, erroDoGrupo error, agora time.Time) (turma.Entrega, bool) {
	base := turma.Entrega{Exercicio: e.ID, GRR: a.GRR, ColetadoEm: agora}

	if sit, bloqueado := situacaoBloqueio(a.SituacaoConta); bloqueado {
		if sit != turma.SemAcesso || erroDoGrupo != nil || len(projetos) == 0 {
			base.Situacao, base.Projeto = sit, a.Grupo
			// Aluno sem grupo próprio ainda pode ter entregado no fork do
			// colega.
			return base, false
		}
		// Grupo legível apesar da falta de associação: segue a coleta normal.
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
