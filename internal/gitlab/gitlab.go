// Package gitlab isola o acesso à API do gitlab.com.
//
// A coleta fala com a interface Cliente, e não com a biblioteca, para que os
// testes rodem sem rede: nenhum teste desta aplicação toca o gitlab.com.
package gitlab

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	api "gitlab.com/gitlab-org/api/client-go"
)

// Grupo é um grupo do GitLab, do jeito que interessa aqui.
type Grupo struct {
	ID      int64
	Caminho string // full_path, como "ds122-2026-2-n-grr20249999"
	Nome    string
	URL     string
	Membro  bool // o professor está associado com acesso de reporter ou mais
	Privado bool
}

// Projeto é um repositório do GitLab.
type Projeto struct {
	ID         int64
	Caminho    string // path, o nome do repositório dentro do grupo
	Completo   string // path_with_namespace
	URL        string
	RamoPadrao string
	ForkDe     string // caminho completo do projeto de origem, se for fork
	Vazio      bool
}

// Membro é alguém associado a um projeto. É por aqui que se descobre a
// entrega em dupla: o colega entra como membro do fork, e não do grupo.
type Membro struct {
	Usuario     string
	Nome        string
	NivelAcesso int
}

// Commit é um commit do ramo consultado.
type Commit struct {
	SHA    string
	Titulo string
	Autor  string
	Email  string
	Data   time.Time
}

// Issue é uma issue do GitLab, do jeito que interessa aqui.
type Issue struct {
	// IID é o número visto na interface e usado nas rotas da API, e não o
	// id global.
	IID    int64
	Titulo string
	URL    string
}

// Cliente é o que a coleta consome. As implementações precisam ser seguras
// para uso concorrente: a coleta roda vários alunos ao mesmo tempo.
type Cliente interface {
	// UsuarioExiste informa se há conta com aquele login.
	UsuarioExiste(login string) (bool, error)
	// Grupo devolve o grupo pelo caminho completo, ou nil quando ele não é
	// visível para o token em uso.
	Grupo(caminho string) (*Grupo, error)
	// GruposComAcesso lista os grupos em que o dono do token participa com
	// acesso de reporter ou mais e cujo nome casa com a busca. É a lista que
	// revela o grupo do aluno que errou o nome.
	//
	// A busca importa: quem dá aula há alguns semestres acumula centenas de
	// grupos, e a listagem completa leva dezenas de segundos. Filtrar pelo
	// prefixo da turma resolve em uma requisição.
	GruposComAcesso(busca string) ([]Grupo, error)
	// Eu devolve o login do dono do token. Serve de teste de conexão barato.
	Eu() (string, error)
	// MembroDoGrupo informa se o dono do token participa do grupo com acesso
	// de reporter ou mais, com uma consulta dirigida.
	//
	// É o caminho barato para a pergunta: a busca por nome tem limite
	// próprio no gitlab.com e passa a devolver 429 quando se faz uma por
	// aluno.
	MembroDoGrupo(caminho string) (bool, error)
	// ProjetosDoGrupo lista os repositórios de um grupo.
	ProjetosDoGrupo(grupo string) ([]Projeto, error)
	// Commits lista os commits de um projeto. Ramo vazio usa o ramo padrão;
	// todos inclui os commits de qualquer ramo.
	Commits(projeto string, ramo string, todos bool) ([]Commit, error)
	// Ramos lista os ramos de um projeto. Serve para varrer o histórico do
	// repositório-modelo sem passar pelas refs de merge request, que trazem
	// commits de fork de aluno para dentro do modelo.
	Ramos(projeto string) ([]string, error)
	// Membros lista quem está associado a um projeto, herança de grupo
	// incluída.
	Membros(projeto string) ([]Membro, error)
	// IssuesDoProjeto lista as issues abertas de um projeto. É o que permite
	// reconhecer a devolutiva já publicada sem depender do arquivo local.
	IssuesDoProjeto(projeto string) ([]Issue, error)
	// CriarIssue abre uma issue no projeto e devolve o que foi criado.
	CriarIssue(projeto, titulo, corpo string) (Issue, error)
	// ComentarIssue acrescenta um comentário a uma issue existente.
	ComentarIssue(projeto string, iid int64, corpo string) error
	// Renovar descarta as listagens memorizadas.
	//
	// O cache existe para os oito trabalhadores de uma coleta não pedirem a
	// mesma listagem ao mesmo tempo, e o tempo de vida dele é o de uma
	// operação. Na linha de comando o processo termina e o cache vai junto;
	// na TUI o cliente dura a sessão inteira, e sem isto a segunda coleta
	// responderia do cache, escondendo o grupo que o aluno acabou de criar.
	Renovar()
}

// paginaMaxima limita a varredura de commits de um repositório de exercício.
// Nenhum trabalho da disciplina chega perto disso, e o teto evita que um
// repositório com histórico importado consuma a cota de requisições.
const paginaMaxima = 20

const porPagina = 100

// clienteAPI implementa Cliente sobre a biblioteca oficial.
//
// Toda listagem passa por um cache com busca única: a coleta roda oito
// trabalhadores em paralelo e vários deles pedem a mesma coisa ao mesmo tempo,
// em especial os grupos do professor e os commits do repositório-modelo.
type clienteAPI struct {
	c *api.Client

	muEu sync.Mutex
	eu   *api.User

	grupos   *cache[[]Grupo]
	projetos *cache[[]Projeto]
	commits  *cache[[]Commit]
	ramos    *cache[[]string]
	membros  *cache[[]Membro]
	issues   *cache[[]Issue]
}

// Renovar descarta as listagens memorizadas na sessão anterior.
func (g *clienteAPI) Renovar() {
	g.grupos.limpar()
	g.projetos.limpar()
	g.commits.limpar()
	g.ramos.limpar()
	g.membros.limpar()
	g.issues.limpar()
}

// Novo abre um cliente autenticado.
func Novo(host, token string) (Cliente, error) {
	if token == "" {
		return nil, errors.New("token de acesso ausente")
	}
	var opts []api.ClientOptionFunc
	if host != "" && host != "https://gitlab.com" {
		opts = append(opts, api.WithBaseURL(strings.TrimRight(host, "/")+"/api/v4"))
	}
	c, err := api.NewClient(token, opts...)
	if err != nil {
		return nil, err
	}
	return &clienteAPI{
		c:        c,
		grupos:   novoCache[[]Grupo](),
		projetos: novoCache[[]Projeto](),
		commits:  novoCache[[]Commit](),
		ramos:    novoCache[[]string](),
		membros:  novoCache[[]Membro](),
		issues:   novoCache[[]Issue](),
	}, nil
}

func (g *clienteAPI) UsuarioExiste(login string) (bool, error) {
	us, _, err := g.c.Users.ListUsers(&api.ListUsersOptions{
		Username:    api.Ptr(login),
		ListOptions: api.ListOptions{PerPage: 1},
	})
	if err != nil {
		return false, traduzirErro(err, "consultando o usuário "+login)
	}
	return len(us) > 0, nil
}

func (g *clienteAPI) Grupo(caminho string) (*Grupo, error) {
	gr, resp, err := g.c.Groups.GetGroup(caminho, &api.GetGroupOptions{})
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return nil, nil
		}
		return nil, traduzirErro(err, "consultando o grupo "+caminho)
	}
	// GetGroup enxerga grupo público sem associação nenhuma, então quem
	// decide se o professor é reporter é a listagem filtrada de grupos dele,
	// consultada por quem chama.
	out := converterGrupo(gr)
	return &out, nil
}

func (g *clienteAPI) Eu() (string, error) {
	u, err := g.usuarioAtual()
	if err != nil {
		return "", err
	}
	return u.Username, nil
}

// usuarioAtual guarda o dono do token, consultado uma vez por sessão.
func (g *clienteAPI) usuarioAtual() (*api.User, error) {
	g.muEu.Lock()
	defer g.muEu.Unlock()
	if g.eu != nil {
		return g.eu, nil
	}
	u, _, err := g.c.Users.CurrentUser()
	if err != nil {
		return nil, traduzirErro(err, "consultando o dono do token")
	}
	g.eu = u
	return u, nil
}

func (g *clienteAPI) MembroDoGrupo(caminho string) (bool, error) {
	eu, err := g.usuarioAtual()
	if err != nil {
		return false, err
	}
	m, resp, err := g.c.GroupMembers.GetInheritedGroupMember(caminho, eu.ID)
	if err != nil {
		if resp != nil && (resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusForbidden) {
			return false, nil
		}
		return false, traduzirErro(err, "conferindo a sua associação a "+caminho)
	}
	return m != nil && m.AccessLevel >= api.ReporterPermissions, nil
}

func (g *clienteAPI) GruposComAcesso(busca string) ([]Grupo, error) {
	return g.grupos.obter(busca, func() ([]Grupo, error) {
		var out []Grupo
		opt := &api.ListGroupsOptions{
			MinAccessLevel: api.Ptr(api.ReporterPermissions),
			ListOptions:    api.ListOptions{PerPage: porPagina, Page: 1},
		}
		if busca != "" {
			opt.Search = api.Ptr(busca)
		}
		for {
			grs, resp, err := g.c.Groups.ListGroups(opt)
			if err != nil {
				return nil, traduzirErro(err, "listando os seus grupos")
			}
			for _, gr := range grs {
				x := converterGrupo(gr)
				x.Membro = true
				out = append(out, x)
			}
			if resp == nil || resp.NextPage == 0 {
				break
			}
			opt.Page = resp.NextPage
		}
		return out, nil
	})
}

func (g *clienteAPI) ProjetosDoGrupo(grupo string) ([]Projeto, error) {
	return g.projetos.obter(grupo, func() ([]Projeto, error) {
		var out []Projeto
		opt := &api.ListGroupProjectsOptions{
			IncludeSubGroups: api.Ptr(true),
			ListOptions:      api.ListOptions{PerPage: porPagina, Page: 1},
		}
		for {
			ps, resp, err := g.c.Groups.ListGroupProjects(grupo, opt)
			if err != nil {
				if resp != nil && resp.StatusCode == http.StatusNotFound {
					return nil, nil
				}
				return nil, traduzirErro(err, "listando os projetos de "+grupo)
			}
			for _, p := range ps {
				out = append(out, converterProjeto(p))
			}
			if resp == nil || resp.NextPage == 0 {
				break
			}
			opt.Page = resp.NextPage
		}
		return out, nil
	})
}

func (g *clienteAPI) Commits(projeto, ramo string, todos bool) ([]Commit, error) {
	chave := fmt.Sprintf("%s|%s|%t", projeto, ramo, todos)
	return g.commits.obter(chave, func() ([]Commit, error) {
		opt := &api.ListCommitsOptions{
			ListOptions: api.ListOptions{PerPage: porPagina, Page: 1},
		}
		if ramo != "" {
			opt.RefName = api.Ptr(ramo)
		}
		if todos {
			opt.All = api.Ptr(true)
		}

		var out []Commit
		for pagina := 0; pagina < paginaMaxima; pagina++ {
			cs, resp, err := g.c.Commits.ListCommits(projeto, opt)
			if err != nil {
				if resp != nil && resp.StatusCode == http.StatusNotFound {
					// Repositório vazio devolve 404 no lugar de lista vazia.
					return nil, nil
				}
				return nil, traduzirErro(err, "listando os commits de "+projeto)
			}
			for _, c := range cs {
				out = append(out, converterCommit(c))
			}
			if resp == nil || resp.NextPage == 0 {
				break
			}
			opt.Page = resp.NextPage
		}
		return out, nil
	})
}

func (g *clienteAPI) Ramos(projeto string) ([]string, error) {
	return g.ramos.obter(projeto, func() ([]string, error) {
		var out []string
		opt := &api.ListBranchesOptions{ListOptions: api.ListOptions{PerPage: porPagina, Page: 1}}
		for {
			bs, resp, err := g.c.Branches.ListBranches(projeto, opt)
			if err != nil {
				if resp != nil && resp.StatusCode == http.StatusNotFound {
					// Repositório vazio devolve 404 no lugar de lista vazia.
					return nil, nil
				}
				return nil, traduzirErro(err, "listando os ramos de "+projeto)
			}
			for _, b := range bs {
				out = append(out, b.Name)
			}
			if resp == nil || resp.NextPage == 0 {
				break
			}
			opt.Page = resp.NextPage
		}
		return out, nil
	})
}

func (g *clienteAPI) Membros(projeto string) ([]Membro, error) {
	return g.membros.obter(projeto, func() ([]Membro, error) {
		var out []Membro
		opt := &api.ListProjectMembersOptions{ListOptions: api.ListOptions{PerPage: porPagina, Page: 1}}
		for {
			// A listagem "all" inclui quem herdou acesso do grupo, que é onde
			// o dono do fork aparece.
			ms, resp, err := g.c.ProjectMembers.ListAllProjectMembers(projeto, opt)
			if err != nil {
				if resp != nil && resp.StatusCode == http.StatusNotFound {
					return nil, nil
				}
				return nil, traduzirErro(err, "listando os membros de "+projeto)
			}
			for _, m := range ms {
				out = append(out, Membro{
					Usuario:     m.Username,
					Nome:        m.Name,
					NivelAcesso: int(m.AccessLevel),
				})
			}
			if resp == nil || resp.NextPage == 0 {
				break
			}
			opt.Page = resp.NextPage
		}
		return out, nil
	})
}

func (g *clienteAPI) IssuesDoProjeto(projeto string) ([]Issue, error) {
	return g.issues.obter(projeto, func() ([]Issue, error) {
		var out []Issue
		opt := &api.ListProjectIssuesOptions{
			State:       api.Ptr("opened"),
			ListOptions: api.ListOptions{PerPage: porPagina, Page: 1},
		}
		for {
			is, resp, err := g.c.Issues.ListProjectIssues(projeto, opt)
			if err != nil {
				return nil, traduzirErroDeIssue(err, resp, "listando as issues de "+projeto)
			}
			for _, i := range is {
				out = append(out, Issue{IID: i.IID, Titulo: i.Title, URL: i.WebURL})
			}
			if resp == nil || resp.NextPage == 0 {
				break
			}
			opt.Page = resp.NextPage
		}
		return out, nil
	})
}

// CriarIssue abre a issue. Escrita não é memorizada, e a listagem de issues
// do projeto fica velha depois dela: quem publicar em série precisa contar
// com o que criou, e não com o cache.
func (g *clienteAPI) CriarIssue(projeto, titulo, corpo string) (Issue, error) {
	i, resp, err := g.c.Issues.CreateIssue(projeto, &api.CreateIssueOptions{
		Title:       api.Ptr(titulo),
		Description: api.Ptr(corpo),
	})
	if err != nil {
		return Issue{}, traduzirErroDeIssue(err, resp, "abrindo a issue em "+projeto)
	}
	return Issue{IID: i.IID, Titulo: i.Title, URL: i.WebURL}, nil
}

func (g *clienteAPI) ComentarIssue(projeto string, iid int64, corpo string) error {
	_, resp, err := g.c.Notes.CreateIssueNote(projeto, iid, &api.CreateIssueNoteOptions{
		Body: api.Ptr(corpo),
	})
	if err != nil {
		return traduzirErroDeIssue(err, resp,
			fmt.Sprintf("comentando a issue %d de %s", iid, projeto))
	}
	return nil
}

func converterGrupo(g *api.Group) Grupo {
	return Grupo{
		ID:      g.ID,
		Caminho: g.FullPath,
		Nome:    g.Name,
		URL:     g.WebURL,
		Privado: g.Visibility == api.PrivateVisibility,
	}
}

func converterProjeto(p *api.Project) Projeto {
	x := Projeto{
		ID:         p.ID,
		Caminho:    p.Path,
		Completo:   p.PathWithNamespace,
		URL:        p.WebURL,
		RamoPadrao: p.DefaultBranch,
		Vazio:      p.EmptyRepo,
	}
	if p.ForkedFromProject != nil {
		x.ForkDe = p.ForkedFromProject.PathWithNamespace
	}
	return x
}

func converterCommit(c *api.Commit) Commit {
	x := Commit{
		SHA:    c.ID,
		Titulo: c.Title,
		Autor:  c.AuthorName,
		Email:  c.AuthorEmail,
	}
	// CommittedDate é a data que o GitLab usa para ordenar e é a que importa
	// para o prazo: rebase e cherry-pick mudam o commit sem mudar a autoria.
	if c.CommittedDate != nil {
		x.Data = *c.CommittedDate
	} else if c.AuthoredDate != nil {
		x.Data = *c.AuthoredDate
	}
	return x
}

// traduzirErroDeIssue explica o 404 que o gitlab.com devolve quando o token
// não tem permissão sobre o projeto.
//
// Publicar devolutiva é escrita, e escrita pede o escopo api: o read_api da
// coleta lê tudo e não abre issue nenhuma. O projeto do aluno também some do
// token que perdeu a associação ao grupo, e os dois casos chegam aqui como
// "404 project not found", que sozinho não diz nada.
func traduzirErroDeIssue(err error, resp *api.Response, contexto string) error {
	if resp != nil && (resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusForbidden) {
		return fmt.Errorf("%s: o GitLab recusou por permissão. "+
			"Publicar issue exige token com escopo api, e não read_api, "+
			"e o professor precisa continuar associado ao grupo do aluno", contexto)
	}
	return traduzirErro(err, contexto)
}

// traduzirErro troca as falhas mais comuns por instrução acionável, em vez de
// devolver o código HTTP cru.
func traduzirErro(err error, contexto string) error {
	var resp *api.ErrorResponse
	if errors.As(err, &resp) && resp.Response != nil {
		switch resp.Response.StatusCode {
		case http.StatusUnauthorized:
			return fmt.Errorf("%s: token recusado; gere um token com escopo read_api, ou api para publicar devolutiva, em %s",
				contexto, "https://gitlab.com/-/user_settings/personal_access_tokens")
		case http.StatusForbidden:
			return fmt.Errorf("%s: token sem permissão; read_api basta para ler, e publicar devolutiva exige api", contexto)
		case http.StatusTooManyRequests:
			return fmt.Errorf("%s: limite de requisições do GitLab atingido; reduza paralelismo no config.toml", contexto)
		}
	}
	return fmt.Errorf("%s: %w", contexto, err)
}
