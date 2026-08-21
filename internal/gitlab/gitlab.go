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

// Cliente é o que a coleta consome. As implementações precisam ser seguras
// para uso concorrente: a coleta roda vários alunos ao mesmo tempo.
type Cliente interface {
	// UsuarioExiste informa se há conta com aquele login.
	UsuarioExiste(login string) (bool, error)
	// Grupo devolve o grupo pelo caminho completo, ou nil quando ele não é
	// visível para o token em uso.
	Grupo(caminho string) (*Grupo, error)
	// GruposDoProfessor lista os grupos em que o dono do token participa com
	// acesso de reporter ou mais. É a lista que revela o grupo do aluno que
	// errou o nome.
	GruposDoProfessor() ([]Grupo, error)
	// ProjetosDoGrupo lista os repositórios de um grupo.
	ProjetosDoGrupo(grupo string) ([]Projeto, error)
	// Commits lista os commits de um projeto. Ramo vazio usa o ramo padrão;
	// todos inclui os commits de qualquer ramo.
	Commits(projeto string, ramo string, todos bool) ([]Commit, error)
	// Forks lista os forks visíveis de um projeto-modelo.
	Forks(modelo string) ([]Projeto, error)
	// Membros lista quem está associado a um projeto, herança de grupo
	// incluída.
	Membros(projeto string) ([]Membro, error)
}

// paginaMaxima limita a varredura de commits de um repositório de exercício.
// Nenhum trabalho da disciplina chega perto disso, e o teto evita que um
// repositório com histórico importado consuma a cota de requisições.
const paginaMaxima = 20

const porPagina = 100

// clienteAPI implementa Cliente sobre a biblioteca oficial.
type clienteAPI struct {
	c *api.Client

	mu       sync.Mutex
	grupos   []Grupo // cache de GruposDoProfessor
	projetos map[string][]Projeto
	commits  map[string][]Commit
	forks    map[string][]Projeto
	membros  map[string][]Membro
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
		projetos: map[string][]Projeto{},
		commits:  map[string][]Commit{},
		forks:    map[string][]Projeto{},
		membros:  map[string][]Membro{},
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
	out := converterGrupo(gr)
	// GetGroup enxerga grupo público sem associação nenhuma. Quem decide se o
	// professor tem acesso de reporter é a lista de grupos dele.
	meus, err := g.GruposDoProfessor()
	if err != nil {
		return nil, err
	}
	for _, m := range meus {
		if m.ID == out.ID {
			out.Membro = true
			break
		}
	}
	return &out, nil
}

func (g *clienteAPI) GruposDoProfessor() ([]Grupo, error) {
	g.mu.Lock()
	if g.grupos != nil {
		defer g.mu.Unlock()
		return g.grupos, nil
	}
	g.mu.Unlock()

	var out []Grupo
	opt := &api.ListGroupsOptions{
		MinAccessLevel: api.Ptr(api.ReporterPermissions),
		ListOptions:    api.ListOptions{PerPage: porPagina, Page: 1},
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

	g.mu.Lock()
	g.grupos = out
	g.mu.Unlock()
	return out, nil
}

func (g *clienteAPI) ProjetosDoGrupo(grupo string) ([]Projeto, error) {
	g.mu.Lock()
	if p, ok := g.projetos[grupo]; ok {
		g.mu.Unlock()
		return p, nil
	}
	g.mu.Unlock()

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

	g.mu.Lock()
	g.projetos[grupo] = out
	g.mu.Unlock()
	return out, nil
}

func (g *clienteAPI) Commits(projeto, ramo string, todos bool) ([]Commit, error) {
	chave := fmt.Sprintf("%s|%s|%t", projeto, ramo, todos)
	g.mu.Lock()
	if c, ok := g.commits[chave]; ok {
		g.mu.Unlock()
		return c, nil
	}
	g.mu.Unlock()

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

	g.mu.Lock()
	g.commits[chave] = out
	g.mu.Unlock()
	return out, nil
}

func (g *clienteAPI) Forks(modelo string) ([]Projeto, error) {
	g.mu.Lock()
	if f, ok := g.forks[modelo]; ok {
		g.mu.Unlock()
		return f, nil
	}
	g.mu.Unlock()

	var out []Projeto
	opt := &api.ListProjectsOptions{ListOptions: api.ListOptions{PerPage: porPagina, Page: 1}}
	for {
		ps, resp, err := g.c.Projects.ListProjectForks(modelo, opt)
		if err != nil {
			if resp != nil && resp.StatusCode == http.StatusNotFound {
				return nil, nil
			}
			return nil, traduzirErro(err, "listando os forks de "+modelo)
		}
		for _, p := range ps {
			out = append(out, converterProjeto(p))
		}
		if resp == nil || resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}

	g.mu.Lock()
	g.forks[modelo] = out
	g.mu.Unlock()
	return out, nil
}

func (g *clienteAPI) Membros(projeto string) ([]Membro, error) {
	g.mu.Lock()
	if m, ok := g.membros[projeto]; ok {
		g.mu.Unlock()
		return m, nil
	}
	g.mu.Unlock()

	var out []Membro
	opt := &api.ListProjectMembersOptions{ListOptions: api.ListOptions{PerPage: porPagina, Page: 1}}
	for {
		// A listagem "all" inclui quem herdou acesso do grupo, que é onde o
		// dono do fork aparece.
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

	g.mu.Lock()
	g.membros[projeto] = out
	g.mu.Unlock()
	return out, nil
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

// traduzirErro troca as falhas mais comuns por instrução acionável, em vez de
// devolver o código HTTP cru.
func traduzirErro(err error, contexto string) error {
	var resp *api.ErrorResponse
	if errors.As(err, &resp) && resp.Response != nil {
		switch resp.Response.StatusCode {
		case http.StatusUnauthorized:
			return fmt.Errorf("%s: token recusado; gere um token com escopo read_api em %s",
				contexto, "https://gitlab.com/-/user_settings/personal_access_tokens")
		case http.StatusForbidden:
			return fmt.Errorf("%s: token sem permissão; o escopo read_api é o necessário", contexto)
		case http.StatusTooManyRequests:
			return fmt.Errorf("%s: limite de requisições do GitLab atingido; reduza paralelismo no config.toml", contexto)
		}
	}
	return fmt.Errorf("%s: %w", contexto, err)
}
