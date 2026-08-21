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
	// caso da maioria: nem todo enunciado é testável.
	Verificacao string
	// Imagem é o contêiner onde a suíte roda. Vazio usa a imagem padrão da
	// configuração.
	Imagem   string
	Situacao SituacaoExercicio
}

// TemSuite informa se o exercício tem verificação automatizada.
func (e Exercicio) TemSuite() bool { return strings.TrimSpace(e.Verificacao) != "" }

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

// SituacaoVerificacao é o veredito da suíte automatizada.
type SituacaoVerificacao string

const (
	Aprovado  SituacaoVerificacao = "aprovado"
	Reprovado SituacaoVerificacao = "reprovado"
	// SemSuite é exercício sem verificação cadastrada. Não prejudica nota:
	// significa que a correção é toda à mão.
	SemSuite SituacaoVerificacao = "sem_suite"
	// SemClone é a suíte que não rodou porque o repositório não foi baixado.
	SemClone SituacaoVerificacao = "sem_clone"
	// ErroVerificacao é falha da execução em si, e não do trabalho do aluno:
	// imagem ausente, tempo esgotado, contêiner que não subiu.
	ErroVerificacao SituacaoVerificacao = "erro"
)

// Verificacao é o resultado de uma execução da suíte sobre o clone de um
// aluno.
//
// Fica em arquivo próprio, e não junto da entrega, porque a coleta regrava as
// entregas por inteiro e apagaria o resultado a cada recoleta.
type Verificacao struct {
	Exercicio string
	GRR       string
	Situacao  SituacaoVerificacao
	Aprovados int
	Total     int
	// Commit é o que foi verificado. Quando difere do commit da entrega, o
	// resultado está velho e o relatório precisa dizer isso.
	Commit      string
	Duracao     time.Duration
	ExecutadoEm time.Time
	Detalhe     string
}

// Resumo descreve o resultado em uma linha.
func (v Verificacao) Resumo() string {
	switch v.Situacao {
	case Aprovado, Reprovado:
		if v.Total > 0 {
			return fmt.Sprintf("%s (%d/%d)", v.Situacao, v.Aprovados, v.Total)
		}
		return string(v.Situacao)
	case ErroVerificacao:
		if v.Detalhe != "" {
			return "erro: " + v.Detalhe
		}
	}
	return string(v.Situacao)
}

// Desatualizada informa se a verificação foi feita sobre outro commit.
func (v Verificacao) Desatualizada(commitDaEntrega string) bool {
	return v.Commit != "" && commitDaEntrega != "" && v.Commit != commitDaEntrega
}

// OrigemVinculo diz quem afirmou que a entrega é compartilhada.
type OrigemVinculo string

const (
	// VinculoDescoberto veio da API: o aluno é membro de um fork que está no
	// grupo de outro. A coleta regrava esses a cada passada.
	VinculoDescoberto OrigemVinculo = "gitlab"
	// VinculoManual foi cadastrado pelo professor e a coleta não o toca.
	// Cobre a dupla que trabalhou junto sem adicionar o colega ao projeto.
	VinculoManual OrigemVinculo = "manual"
)

// Vinculo registra que um aluno entregou dentro do fork de outro.
//
// Só os integrantes que não são donos do fork têm linha: a equipe de uma
// entrega é o dono mais quem aponta para ele.
type Vinculo struct {
	Exercicio    string
	GRR          string // o integrante
	Dono         string // GRR de quem tem o fork no próprio grupo
	Origem       OrigemVinculo
	AtualizadoEm time.Time
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

	NotaMaxima float64 `toml:"nota_maxima"`
	// ImagemVerificacao é o contêiner padrão das suítes automatizadas.
	ImagemVerificacao string `toml:"imagem_verificacao"`
	// TempoLimiteVerificacao é o teto de cada execução, em segundos.
	TempoLimiteVerificacao int `toml:"tempo_limite_verificacao"`
	Paralelismo            int `toml:"paralelismo"`
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

// PrefixoGrupo é a parte fixa do nome dos grupos da turma, antes do GRR.
//
// Serve para pedir ao GitLab só os grupos desta turma, em vez da listagem
// inteira de quem dá aula há vários semestres.
func (c Config) PrefixoGrupo() string {
	antes, _, ok := strings.Cut(c.PadraoGrupo, "{grr}")
	if !ok {
		antes = c.PadraoGrupo
	}
	r := strings.NewReplacer(
		"{codigo}", strings.ToLower(c.Codigo),
		"{ano}", c.Ano(),
		"{periodo}", c.Periodo(),
		"{semestre}", c.Periodo(),
		"{turno}", strings.ToLower(c.Turno),
	)
	return strings.Trim(r.Replace(antes), "-/_")
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
	if c.ImagemVerificacao == "" {
		c.ImagemVerificacao = "docker.io/library/alpine:3.20"
	}
	if c.TempoLimiteVerificacao <= 0 {
		c.TempoLimiteVerificacao = 120
	}
	if c.Paralelismo <= 0 {
		c.Paralelismo = 8
	}
}

// Turma agrega a configuração e todos os registros.
type Turma struct {
	Config       Config
	Alunos       []Aluno
	Exercicios   []Exercicio
	Entregas     []Entrega
	Notas        []Nota
	Verificacoes []Verificacao
	Vinculos     []Vinculo
}

// Dono devolve o GRR de quem tem o fork usado por este aluno no exercício.
// Sem vínculo, o dono é o próprio aluno.
func (t *Turma) Dono(exercicio, grr string) string {
	grr = NormalizarGRR(grr)
	for _, v := range t.Vinculos {
		if v.Exercicio == exercicio && v.GRR == grr {
			return v.Dono
		}
	}
	return grr
}

// Compartilhada informa se a entrega do aluno é de uma equipe, com dois ou
// mais integrantes.
func (t *Turma) Compartilhada(exercicio, grr string) bool {
	return len(t.Equipe(exercicio, grr)) > 1
}

// Equipe devolve todos os GRRs que entregam no mesmo fork, incluindo o dono,
// em ordem. Aluno sem vínculo devolve só ele mesmo.
func (t *Turma) Equipe(exercicio, grr string) []string {
	dono := t.Dono(exercicio, grr)
	equipe := []string{dono}
	for _, v := range t.Vinculos {
		if v.Exercicio == exercicio && v.Dono == dono {
			equipe = append(equipe, v.GRR)
		}
	}
	sort.Strings(equipe)
	return equipe
}

// VinculosDoExercicio devolve os vínculos de um exercício, por GRR do
// integrante.
func (t *Turma) VinculosDoExercicio(exercicio string) map[string]Vinculo {
	out := map[string]Vinculo{}
	for _, v := range t.Vinculos {
		if v.Exercicio == exercicio {
			out[v.GRR] = v
		}
	}
	return out
}

// RegistrarVinculo insere ou substitui o vínculo de um integrante.
func (t *Turma) RegistrarVinculo(v Vinculo) {
	v.GRR, v.Dono = NormalizarGRR(v.GRR), NormalizarGRR(v.Dono)
	for i := range t.Vinculos {
		if t.Vinculos[i].Exercicio == v.Exercicio && t.Vinculos[i].GRR == v.GRR {
			t.Vinculos[i] = v
			return
		}
	}
	t.Vinculos = append(t.Vinculos, v)
	ordenarVinculos(t.Vinculos)
}

// RemoverVinculo desfaz o vínculo de um integrante.
func (t *Turma) RemoverVinculo(exercicio, grr string) bool {
	grr = NormalizarGRR(grr)
	for i := range t.Vinculos {
		if t.Vinculos[i].Exercicio == exercicio && t.Vinculos[i].GRR == grr {
			t.Vinculos = append(t.Vinculos[:i], t.Vinculos[i+1:]...)
			return true
		}
	}
	return false
}

// SubstituirVinculosDescobertos troca o que a coleta apurou em um exercício,
// preservando o que foi cadastrado à mão.
//
// Mesma separação das notas: o que a API diz é regravado a cada coleta, o que
// o professor afirmou permanece. Um vínculo manual também vence o descoberto
// para o mesmo aluno, porque foi uma decisão consciente.
func (t *Turma) SubstituirVinculosDescobertos(exercicio string, novos []Vinculo) {
	manuais := map[string]bool{}
	mantidos := t.Vinculos[:0:0]
	for _, v := range t.Vinculos {
		if v.Exercicio != exercicio || v.Origem == VinculoManual {
			mantidos = append(mantidos, v)
			if v.Exercicio == exercicio {
				manuais[v.GRR] = true
			}
		}
	}
	for _, v := range novos {
		if !manuais[NormalizarGRR(v.GRR)] {
			mantidos = append(mantidos, v)
		}
	}
	t.Vinculos = mantidos
	ordenarVinculos(t.Vinculos)
}

// Verificacao devolve o último resultado da suíte para um aluno.
func (t *Turma) Verificacao(exercicio, grr string) (*Verificacao, bool) {
	grr = NormalizarGRR(grr)
	for i := range t.Verificacoes {
		if t.Verificacoes[i].Exercicio == exercicio && t.Verificacoes[i].GRR == grr {
			return &t.Verificacoes[i], true
		}
	}
	return nil, false
}

// VerificacoesDoExercicio devolve os resultados de um exercício, por GRR.
func (t *Turma) VerificacoesDoExercicio(exercicio string) map[string]Verificacao {
	out := map[string]Verificacao{}
	for _, v := range t.Verificacoes {
		if v.Exercicio == exercicio {
			out[v.GRR] = v
		}
	}
	return out
}

// RegistrarVerificacao insere ou substitui o resultado de um aluno.
func (t *Turma) RegistrarVerificacao(v Verificacao) {
	v.GRR = NormalizarGRR(v.GRR)
	if p, ok := t.Verificacao(v.Exercicio, v.GRR); ok {
		*p = v
		return
	}
	t.Verificacoes = append(t.Verificacoes, v)
	ordenarVerificacoes(t.Verificacoes)
	ordenarVinculos(t.Vinculos)
}

func ordenarVinculos(vs []Vinculo) {
	sort.SliceStable(vs, func(i, j int) bool {
		if vs[i].Exercicio != vs[j].Exercicio {
			return vs[i].Exercicio < vs[j].Exercicio
		}
		return vs[i].GRR < vs[j].GRR
	})
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
	ordenarVerificacoes(t.Verificacoes)
}

func ordenarVerificacoes(vs []Verificacao) {
	sort.SliceStable(vs, func(i, j int) bool {
		if vs[i].Exercicio != vs[j].Exercicio {
			return vs[i].Exercicio < vs[j].Exercicio
		}
		return vs[i].GRR < vs[j].GRR
	})
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

// RegistrarNota lança ou substitui a nota de um aluno em um exercício.
func (t *Turma) RegistrarNota(n Nota) *Nota {
	n.GRR = NormalizarGRR(n.GRR)
	if p, ok := t.Nota(n.Exercicio, n.GRR); ok {
		*p = n
		return p
	}
	t.Notas = append(t.Notas, n)
	ordenarNotas(t.Notas)
	p, _ := t.Nota(n.Exercicio, n.GRR)
	return p
}

// RemoverNota apaga a nota de um aluno em um exercício.
func (t *Turma) RemoverNota(exercicio, grr string) bool {
	grr = NormalizarGRR(grr)
	for i := range t.Notas {
		if t.Notas[i].Exercicio == exercicio && t.Notas[i].GRR == grr {
			t.Notas = append(t.Notas[:i], t.Notas[i+1:]...)
			return true
		}
	}
	return false
}

// Resultado reúne as notas de um aluno e a média ponderada.
type Resultado struct {
	Aluno Aluno
	// Notas traz o valor lançado por exercício. Exercício sem nota não
	// aparece no mapa, o que é diferente de nota zero.
	Notas map[string]float64
	// Media é a média ponderada pelos pesos dos exercícios considerados.
	Media float64
	// Considerados são os exercícios que entraram no denominador.
	Considerados []string
	// Lancadas conta quantos desses já têm nota.
	Lancadas int
}

// Resultados calcula a média de cada aluno sobre os exercícios informados.
//
// Um exercício entra no cálculo quando o prazo já venceu, com ou sem nota
// lançada: quem não entregou tira zero, e ignorar esse caso inflaria a média
// de quem faltou. Exercício com prazo em aberto fica de fora até vencer.
//
// Com somenteLancadas, o denominador tem só os exercícios já corrigidos, que
// é a leitura útil no meio do semestre.
func (t *Turma) Resultados(exercicios []Exercicio, somenteLancadas bool, hoje Data) []Resultado {
	alunos := t.Ativos()
	out := make([]Resultado, 0, len(alunos))

	for _, a := range alunos {
		r := Resultado{Aluno: a, Notas: map[string]float64{}}
		soma, pesos := 0.0, 0.0

		for _, e := range exercicios {
			n, temNota := t.Nota(e.ID, a.GRR)
			if temNota {
				r.Notas[e.ID] = n.Valor
			}
			vencido := !hoje.IsZero() && !e.Prazo.Depois(hoje)
			if somenteLancadas && !temNota {
				continue
			}
			if !somenteLancadas && !temNota && !vencido {
				continue
			}
			peso := e.Peso
			if peso <= 0 {
				peso = 1
			}
			valor := 0.0
			if temNota {
				valor = n.Valor
				r.Lancadas++
			}
			soma += valor * peso
			pesos += peso
			r.Considerados = append(r.Considerados, e.ID)
		}

		if pesos > 0 {
			r.Media = soma / pesos
		}
		out = append(out, r)
	}
	return out
}
