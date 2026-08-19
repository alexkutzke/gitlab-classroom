// Package turma contém o modelo de domínio da turma, dos exercícios e das
// entregas, com as regras que classificam o que o GitLab devolve.
package turma

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Situacao indica se o aluno segue matriculado, espelhando o cadastro do SIGA
// mantido pelo diario.
type Situacao string

const (
	Ativo     Situacao = "ativo"
	Cancelado Situacao = "cancelado"
)

// SituacaoConta é o que se sabe sobre a conta do aluno no GitLab. Vem do
// comando sync e serve para separar problema de cadastro de problema de
// entrega.
type SituacaoConta string

const (
	// ContaDesconhecida é o estado antes da primeira reconciliação.
	ContaDesconhecida SituacaoConta = ""
	// ContaOK é usuário encontrado e grupo acessível com o nome do padrão.
	ContaOK SituacaoConta = "ok"
	// ContaSemUsuario é GRR que não corresponde a nenhum usuário do GitLab.
	ContaSemUsuario SituacaoConta = "sem_conta"
	// ContaGrupoDivergente é grupo acessível, porém com nome fora do padrão.
	// Acontece toda turma, e o relatório precisa dizer qual nome foi usado.
	ContaGrupoDivergente SituacaoConta = "grupo_divergente"
	// ContaSemAcesso é grupo que existe e é visível, mas sem o professor
	// associado como reporter.
	ContaSemAcesso SituacaoConta = "sem_acesso"
	// ContaGrupoInvisivel é o caso genuinamente ambíguo: o grupo não aparece
	// nem pelo nome do padrão nem entre os grupos do professor. Pode não ter
	// sido criado, ou ter sido criado privado sem a associação. A API não
	// distingue os dois, e o rótulo diz isso em vez de fingir que sabe.
	ContaGrupoInvisivel SituacaoConta = "grupo_invisivel"
)

// Aluno é um estudante da turma. O GRR é a chave, como no diario.
type Aluno struct {
	GRR   string
	Nome  string
	Email string
	// Usuario é o login no GitLab. Por convenção é o GRR em minúsculas, mas
	// fica gravado porque o aluno pode ter usado outro.
	Usuario string
	// Grupo é o caminho do grupo onde estão os forks, preenchido pelo sync.
	Grupo string
	// Situacao vem do cadastro do SIGA; SituacaoConta, da reconciliação com
	// o GitLab. São coisas diferentes: aluno ativo pode estar sem conta.
	Situacao      Situacao
	SituacaoConta SituacaoConta
	VerificadoEm  time.Time
	Observacao    string
}

// EstaAtivo informa se o aluno segue matriculado.
func (a Aluno) EstaAtivo() bool { return a.Situacao != Cancelado }

// UsuarioEsperado devolve o login gravado ou, na falta dele, o derivado do GRR.
func (a Aluno) UsuarioEsperado() string {
	if a.Usuario != "" {
		return a.Usuario
	}
	return UsuarioGitLab(a.GRR)
}

// SituacaoExercicio distingue o exercício em uso do que já saiu do ar.
type SituacaoExercicio string

const (
	ExercicioAtivo     SituacaoExercicio = "ativo"
	ExercicioArquivado SituacaoExercicio = "arquivado"
)

// Exercicio é uma tarefa entregue por fork de um repositório-modelo.
type Exercicio struct {
	// ID é o apelido curto usado na linha de comando, como "html".
	ID string
	// Repo é o nome do projeto-modelo dentro do namespace da disciplina,
	// como "ds122-html-assignment". É também o nome do fork do aluno.
	Repo   string
	Titulo string
	Prazo  Data
	Peso   float64
	// Verificacao é o comando da suíte automatizada, relativo à raiz do
	// repositório. Vazio quando o exercício só é corrigido à mão, que é o
	// caso da maioria.
	Verificacao string
	Situacao    SituacaoExercicio
}

// EstaAtivo informa se o exercício entra nas coletas e relatórios correntes.
func (e Exercicio) EstaAtivo() bool { return e.Situacao != ExercicioArquivado }

// Validar recusa exercício sem os campos que a coleta exige.
func (e Exercicio) Validar() error {
	if e.ID == "" {
		return fmt.Errorf("exercício sem id")
	}
	if e.Repo == "" {
		return fmt.Errorf("exercício %s sem repositório-modelo", e.ID)
	}
	if e.Prazo.IsZero() {
		return fmt.Errorf("exercício %s sem prazo", e.ID)
	}
	return nil
}

// SituacaoEntrega é o resultado da coleta para um aluno em um exercício.
type SituacaoEntrega string

const (
	// SemConta, SemAcesso e GrupoInvisivel são problemas anteriores à
	// entrega: o aluno não chegou a ter onde publicar o fork.
	SemConta       SituacaoEntrega = "sem_conta"
	SemAcesso      SituacaoEntrega = "sem_acesso"
	GrupoInvisivel SituacaoEntrega = "grupo_invisivel"
	// SemFork é grupo acessível sem o fork daquele exercício.
	SemFork SituacaoEntrega = "sem_fork"
	// ForkSemCommit é fork feito sem nenhum commit do próprio aluno.
	ForkSemCommit SituacaoEntrega = "fork_sem_commit"
	// SemCommitNoPrazo é fork com commits do aluno, todos depois do prazo.
	SemCommitNoPrazo SituacaoEntrega = "sem_commit_no_prazo"
	Entregue         SituacaoEntrega = "entregue"
	// Erro é falha de rede ou de API, e não um veredito sobre o aluno.
	// Recoletar costuma resolver, e por isso ele não se confunde com os
	// demais.
	Erro SituacaoEntrega = "erro"
)

// rotulos traz o texto exibido nos relatórios para cada situação.
var rotulos = map[SituacaoEntrega]string{
	SemConta:         "sem conta no GitLab",
	SemAcesso:        "grupo sem o professor",
	GrupoInvisivel:   "grupo não criado ou não compartilhado",
	SemFork:          "sem fork",
	ForkSemCommit:    "fork sem commit do aluno",
	SemCommitNoPrazo: "commits só depois do prazo",
	Entregue:         "entregue",
	Erro:             "erro na coleta",
}

// Rotulo devolve a descrição da situação em português.
func (s SituacaoEntrega) Rotulo() string {
	if r, ok := rotulos[s]; ok {
		return r
	}
	return string(s)
}

// Entrega é o que a coleta apurou para um aluno em um exercício.
type Entrega struct {
	Exercicio string
	GRR       string
	Situacao  SituacaoEntrega
	// Projeto é o caminho completo do fork no GitLab.
	Projeto string
	// Commit é o commit avaliado, o mais recente do aluno até o prazo.
	Commit     string
	DataCommit time.Time
	// Commits conta os commits do aluno, isto é, os que não vieram do
	// repositório-modelo.
	Commits int
	// UltimoCommit é o commit mais recente do aluno, com ou sem prazo. É o
	// que separa "não fez" de "fez depois", encaminhamentos diferentes.
	UltimoCommit string
	DataUltimo   time.Time
	AtrasoDias   int
	ColetadoEm   time.Time
	Detalhe      string
}

// NoPrazo informa se há commit do aluno dentro do prazo.
func (e Entrega) NoPrazo() bool { return e.Situacao == Entregue }

// TemAtraso informa se o aluno trabalhou no repositório depois do prazo.
func (e Entrega) TemAtraso() bool { return e.AtrasoDias > 0 }

// Descricao resume a entrega em uma linha, para o terminal e o markdown.
func (e Entrega) Descricao() string {
	switch e.Situacao {
	case Entregue:
		if e.TemAtraso() {
			return fmt.Sprintf("entregue (e mexeu depois, +%dd)", e.AtrasoDias)
		}
		return "entregue"
	case SemCommitNoPrazo:
		return fmt.Sprintf("fora do prazo (+%dd)", e.AtrasoDias)
	case Erro:
		if e.Detalhe != "" {
			return "erro: " + e.Detalhe
		}
	}
	return e.Situacao.Rotulo()
}

// AtrasoEmDias mede quantos dias separam o prazo do commit informado.
//
// Conta dias de calendário, não múltiplos de 24 horas: entregar às 00h10 do
// dia seguinte é um dia de atraso, e é assim que a turma entende o prazo.
func AtrasoEmDias(prazo Data, commit time.Time) int {
	if prazo.IsZero() || commit.IsZero() {
		return 0
	}
	dias := int(DataDe(commit).Sub(prazo.Time).Hours() / 24)
	if dias < 0 {
		return 0
	}
	return dias
}

// Nota é a avaliação lançada pelo professor. Fica em arquivo separado das
// entregas justamente para que recoletar do GitLab nunca a sobrescreva.
type Nota struct {
	Exercicio   string
	GRR         string
	Valor       float64
	Comentario  string
	CorrigidoEm time.Time
}

// Config são os metadados da turma, persistidos em config.toml.
type Config struct {
	Codigo     string `toml:"codigo"`
	Disciplina string `toml:"disciplina"`
	Turma      string `toml:"turma"`
	Semestre   string `toml:"semestre"`
	Turno      string `toml:"turno"`

	Host             string `toml:"host"`
	NamespaceModelos string `toml:"namespace_modelos"`
	// PadraoGrupo é o nome do grupo do aluno com marcadores {ano},
	// {periodo}, {turno} e {grr}, resolvidos por CaminhoGrupo.
	PadraoGrupo   string `toml:"padrao_grupo"`
	PastaEntregas string `toml:"pasta_entregas"`
	// PastaDiario é onde procurar o cadastro do SIGA, relativo à pasta da
	// turma.
	PastaDiario string `toml:"pasta_diario"`

	NotaMaxima  float64 `toml:"nota_maxima"`
	Paralelismo int     `toml:"paralelismo"`
	// TokenArquivo é o caminho de um arquivo com o token de acesso. Fica
	// fora do .classroom/ de propósito.
	TokenArquivo string `toml:"token_arquivo"`
}

// Ano e Periodo saem do semestre no formato "2026-02".
func (c Config) Ano() string {
	ano, _, _ := strings.Cut(c.Semestre, "-")
	return ano
}

func (c Config) Periodo() string {
	_, p, ok := strings.Cut(c.Semestre, "-")
	if !ok {
		return ""
	}
	return strings.TrimLeft(p, "0")
}

// CaminhoGrupo resolve o padrão de nome do grupo para um aluno.
func (c Config) CaminhoGrupo(grr string) string {
	r := strings.NewReplacer(
		"{codigo}", strings.ToLower(c.Codigo),
		"{ano}", c.Ano(),
		"{periodo}", c.Periodo(),
		"{semestre}", c.Periodo(),
		"{turno}", strings.ToLower(c.Turno),
		"{grr}", UsuarioGitLab(grr),
	)
	return r.Replace(c.PadraoGrupo)
}

// CaminhoModelo devolve o caminho completo do repositório-modelo.
func (c Config) CaminhoModelo(repo string) string {
	if c.NamespaceModelos == "" {
		return repo
	}
	return c.NamespaceModelos + "/" + repo
}

// Descricao devolve um rótulo curto do tipo "DS122 (TADSN2A)".
func (c Config) Descricao() string {
	if c.Turma == "" {
		return c.Codigo
	}
	return c.Codigo + " (" + c.Turma + ")"
}

// Padroes preenche os campos que têm valor razoável por omissão.
func (c *Config) Padroes() {
	if c.Host == "" {
		c.Host = "https://gitlab.com"
	}
	if c.PastaEntregas == "" {
		c.PastaEntregas = "entregas"
	}
	if c.PastaDiario == "" {
		c.PastaDiario = ".diario"
	}
	if c.NotaMaxima <= 0 {
		c.NotaMaxima = 100
	}
	if c.Paralelismo <= 0 {
		c.Paralelismo = 8
	}
}

// Turma agrega a configuração e todos os registros.
type Turma struct {
	Config     Config
	Alunos     []Aluno
	Exercicios []Exercicio
	Entregas   []Entrega
	Notas      []Nota
}

// AlunoPorGRR devolve o aluno com o GRR informado.
func (t *Turma) AlunoPorGRR(grr string) (*Aluno, bool) {
	grr = NormalizarGRR(grr)
	for i := range t.Alunos {
		if t.Alunos[i].GRR == grr {
			return &t.Alunos[i], true
		}
	}
	return nil, false
}

// Ativos devolve os alunos ainda matriculados, em ordem de nome.
func (t *Turma) Ativos() []Aluno {
	var out []Aluno
	for _, a := range t.Alunos {
		if a.EstaAtivo() {
			out = append(out, a)
		}
	}
	ordenarAlunos(out)
	return out
}

// Exercicio devolve o exercício pelo id, aceitando também o nome do
// repositório, que é como ele aparece no GitLab.
func (t *Turma) Exercicio(id string) (*Exercicio, bool) {
	id = strings.ToLower(strings.TrimSpace(id))
	for i := range t.Exercicios {
		if strings.ToLower(t.Exercicios[i].ID) == id {
			return &t.Exercicios[i], true
		}
	}
	for i := range t.Exercicios {
		if strings.ToLower(t.Exercicios[i].Repo) == id {
			return &t.Exercicios[i], true
		}
	}
	return nil, false
}

// ExerciciosAtivos devolve os exercícios em uso, em ordem de prazo.
func (t *Turma) ExerciciosAtivos() []Exercicio {
	var out []Exercicio
	for _, e := range t.Exercicios {
		if e.EstaAtivo() {
			out = append(out, e)
		}
	}
	ordenarExercicios(out)
	return out
}

// RegistrarExercicio insere ou substitui um exercício pelo id.
func (t *Turma) RegistrarExercicio(e Exercicio) *Exercicio {
	if p, ok := t.Exercicio(e.ID); ok {
		*p = e
		return p
	}
	t.Exercicios = append(t.Exercicios, e)
	ordenarExercicios(t.Exercicios)
	p, _ := t.Exercicio(e.ID)
	return p
}

// Entrega devolve o que foi coletado de um aluno em um exercício.
func (t *Turma) Entrega(exercicio, grr string) (*Entrega, bool) {
	grr = NormalizarGRR(grr)
	for i := range t.Entregas {
		if t.Entregas[i].Exercicio == exercicio && t.Entregas[i].GRR == grr {
			return &t.Entregas[i], true
		}
	}
	return nil, false
}

// EntregasDoExercicio devolve as entregas de um exercício, por GRR.
func (t *Turma) EntregasDoExercicio(exercicio string) map[string]Entrega {
	out := map[string]Entrega{}
	for _, e := range t.Entregas {
		if e.Exercicio == exercicio {
			out[e.GRR] = e
		}
	}
	return out
}

// SubstituirEntregas troca as entregas de um exercício pelas informadas,
// preservando as dos demais.
//
// É o que torna a coleta idempotente: recoletar um exercício não mexe no que
// foi apurado nos outros, nem em nota nenhuma.
func (t *Turma) SubstituirEntregas(exercicio string, novas []Entrega) {
	mantidas := t.Entregas[:0:0]
	for _, e := range t.Entregas {
		if e.Exercicio != exercicio {
			mantidas = append(mantidas, e)
		}
	}
	t.Entregas = append(mantidas, novas...)
	ordenarEntregas(t.Entregas)
}

// Nota devolve a nota lançada para um aluno em um exercício.
func (t *Turma) Nota(exercicio, grr string) (*Nota, bool) {
	grr = NormalizarGRR(grr)
	for i := range t.Notas {
		if t.Notas[i].Exercicio == exercicio && t.Notas[i].GRR == grr {
			return &t.Notas[i], true
		}
	}
	return nil, false
}

// Ordenar coloca todas as coleções em ordem estável, para que a gravação
// produza sempre o mesmo arquivo.
func (t *Turma) Ordenar() {
	ordenarAlunos(t.Alunos)
	ordenarExercicios(t.Exercicios)
	ordenarEntregas(t.Entregas)
	ordenarNotas(t.Notas)
}

func ordenarAlunos(as []Aluno) {
	sort.SliceStable(as, func(i, j int) bool {
		ni, nj := ChaveNome(as[i].Nome), ChaveNome(as[j].Nome)
		if ni != nj {
			return ni < nj
		}
		return as[i].GRR < as[j].GRR
	})
}

func ordenarExercicios(es []Exercicio) {
	sort.SliceStable(es, func(i, j int) bool {
		if !es[i].Prazo.Equal(es[j].Prazo.Time) {
			return es[i].Prazo.Antes(es[j].Prazo)
		}
		return es[i].ID < es[j].ID
	})
}

func ordenarEntregas(es []Entrega) {
	sort.SliceStable(es, func(i, j int) bool {
		if es[i].Exercicio != es[j].Exercicio {
			return es[i].Exercicio < es[j].Exercicio
		}
		return es[i].GRR < es[j].GRR
	})
}

func ordenarNotas(ns []Nota) {
	sort.SliceStable(ns, func(i, j int) bool {
		if ns[i].Exercicio != ns[j].Exercicio {
			return ns[i].Exercicio < ns[j].Exercicio
		}
		return ns[i].GRR < ns[j].GRR
	})
}

// Busca filtra alunos por trecho do nome, GRR ou e-mail, sem diferenciar
// acentuação nem caixa.
func Busca(alunos []Aluno, termo string) []Aluno {
	termo = ChaveNome(termo)
	if termo == "" {
		return alunos
	}
	var out []Aluno
	for _, a := range alunos {
		alvo := ChaveNome(a.Nome + " " + a.GRR + " " + a.Email + " " + a.Usuario)
		if strings.Contains(alvo, termo) {
			out = append(out, a)
		}
	}
	return out
}
