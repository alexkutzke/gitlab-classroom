// Package store cuida da descoberta do diretório .classroom/ e da leitura e
// escrita dos arquivos texto que guardam o estado da turma.
package store

import (
	"encoding/csv"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/alexkutzke/gitlab-classroom/internal/turma"
)

// Dir é o nome do diretório de dados criado na pasta da turma.
const Dir = ".classroom"

const (
	arqConfig       = "config.toml"
	arqAlunos       = "alunos.csv"
	arqExercicios   = "exercicios.csv"
	arqEntregas     = "entregas.csv"
	arqNotas        = "notas.csv"
	arqVerificacoes = "verificacoes.csv"
)

// ErrNaoEncontrado indica que nenhum .classroom/ foi achado subindo a árvore.
var ErrNaoEncontrado = errors.New("nenhuma turma encontrada")

// Store aponta para um diretório .classroom/ concreto.
type Store struct {
	Raiz string // caminho absoluto do .classroom/
}

// Descobrir procura um .classroom/ a partir de inicio, subindo até a raiz do
// sistema de arquivos, mesma lógica que o git usa com .git. É isso que faz o
// comando inferir a turma a partir da pasta onde foi chamado.
func Descobrir(inicio string) (*Store, error) {
	dir, err := filepath.Abs(inicio)
	if err != nil {
		return nil, err
	}
	for {
		alvo := filepath.Join(dir, Dir)
		if info, err := os.Stat(alvo); err == nil && info.IsDir() {
			return &Store{Raiz: alvo}, nil
		}
		pai := filepath.Dir(dir)
		if pai == dir {
			return nil, fmt.Errorf("%w a partir de %s: rode `classroom init` na pasta da turma",
				ErrNaoEncontrado, inicio)
		}
		dir = pai
	}
}

// Criar prepara um .classroom/ novo dentro de dir.
func Criar(dir string) (*Store, error) {
	raiz := filepath.Join(dir, Dir)
	if _, err := os.Stat(raiz); err == nil {
		return nil, fmt.Errorf("%s já existe: use `classroom sync` para atualizar a turma", raiz)
	}
	if err := os.MkdirAll(raiz, 0o755); err != nil {
		return nil, err
	}
	return &Store{Raiz: raiz}, nil
}

// Pasta devolve o diretório da turma (o pai de .classroom/).
func (s *Store) Pasta() string { return filepath.Dir(s.Raiz) }

func (s *Store) caminho(nome string) string { return filepath.Join(s.Raiz, nome) }

// Carregar lê todos os arquivos de dados.
func (s *Store) Carregar() (*turma.Turma, error) {
	t := &turma.Turma{}
	if _, err := toml.DecodeFile(s.caminho(arqConfig), &t.Config); err != nil {
		return nil, fmt.Errorf("lendo %s: %w", arqConfig, err)
	}
	t.Config.Padroes()
	var err error
	if t.Alunos, err = s.lerAlunos(); err != nil {
		return nil, err
	}
	if t.Exercicios, err = s.lerExercicios(); err != nil {
		return nil, err
	}
	if t.Entregas, err = s.lerEntregas(); err != nil {
		return nil, err
	}
	if t.Notas, err = s.lerNotas(); err != nil {
		return nil, err
	}
	if t.Verificacoes, err = s.lerVerificacoes(); err != nil {
		return nil, err
	}
	t.Ordenar()
	return t, nil
}

// Gravar escreve todos os arquivos, sempre em ordem estável.
func (s *Store) Gravar(t *turma.Turma) error {
	t.Ordenar()
	if err := s.GravarConfig(t.Config); err != nil {
		return err
	}
	if err := s.gravarAlunos(t.Alunos); err != nil {
		return err
	}
	if err := s.gravarExercicios(t.Exercicios); err != nil {
		return err
	}
	if err := s.gravarEntregas(t.Entregas); err != nil {
		return err
	}
	if err := s.gravarNotas(t.Notas); err != nil {
		return err
	}
	return s.gravarVerificacoes(t.Verificacoes)
}

// GravarConfig escreve apenas o config.toml.
func (s *Store) GravarConfig(c turma.Config) error {
	var b strings.Builder
	b.WriteString("# Configuração da turma. Editável à mão.\n")
	b.WriteString("# O token de acesso ao GitLab não fica aqui: veja `classroom token`.\n")
	if err := toml.NewEncoder(&b).Encode(c); err != nil {
		return err
	}
	return escreverAtomico(s.caminho(arqConfig), []byte(b.String()))
}

// --- alunos ---

var cabecalhoAlunos = []string{
	"grr", "nome", "email", "usuario", "grupo", "situacao", "situacao_conta",
	"verificado_em", "observacao",
}

func (s *Store) lerAlunos() ([]turma.Aluno, error) {
	t, err := lerTabela(s.caminho(arqAlunos), cabecalhoAlunos, "grr")
	if err != nil {
		return nil, err
	}
	var out []turma.Aluno
	for i := range t.linhas {
		verificado, err := t.instante(i, "verificado_em")
		if err != nil {
			return nil, err
		}
		sit := turma.Situacao(t.str(i, "situacao"))
		if sit == "" {
			sit = turma.Ativo
		}
		out = append(out, turma.Aluno{
			GRR:           turma.NormalizarGRR(t.str(i, "grr")),
			Nome:          t.str(i, "nome"),
			Email:         t.str(i, "email"),
			Usuario:       t.str(i, "usuario"),
			Grupo:         t.str(i, "grupo"),
			Situacao:      sit,
			SituacaoConta: turma.SituacaoConta(t.str(i, "situacao_conta")),
			VerificadoEm:  verificado,
			Observacao:    t.str(i, "observacao"),
		})
	}
	return out, nil
}

func (s *Store) gravarAlunos(as []turma.Aluno) error {
	linhas := [][]string{cabecalhoAlunos}
	for _, a := range as {
		linhas = append(linhas, []string{
			a.GRR, a.Nome, a.Email, a.Usuario, a.Grupo,
			string(a.Situacao), string(a.SituacaoConta),
			turma.FormatarInstante(a.VerificadoEm), a.Observacao,
		})
	}
	return gravarCSV(s.caminho(arqAlunos), linhas)
}

// --- exercícios ---

var cabecalhoExercicios = []string{"id", "repo", "titulo", "prazo", "peso", "verificacao", "imagem", "situacao"}

func (s *Store) lerExercicios() ([]turma.Exercicio, error) {
	t, err := lerTabela(s.caminho(arqExercicios), cabecalhoExercicios, "id")
	if err != nil {
		return nil, err
	}
	var out []turma.Exercicio
	for i := range t.linhas {
		prazo, err := t.data(i, "prazo")
		if err != nil {
			return nil, err
		}
		peso, err := t.decimal(i, "peso", 1)
		if err != nil {
			return nil, err
		}
		sit := turma.SituacaoExercicio(t.str(i, "situacao"))
		if sit == "" {
			sit = turma.ExercicioAtivo
		}
		e := turma.Exercicio{
			ID:          t.str(i, "id"),
			Repo:        t.str(i, "repo"),
			Titulo:      t.str(i, "titulo"),
			Prazo:       prazo,
			Peso:        peso,
			Verificacao: t.str(i, "verificacao"),
			Imagem:      t.str(i, "imagem"),
			Situacao:    sit,
		}
		if err := e.Validar(); err != nil {
			return nil, fmt.Errorf("%s linha %d: %w", t.arquivo, t.numero(i), err)
		}
		out = append(out, e)
	}
	return out, nil
}

func (s *Store) gravarExercicios(es []turma.Exercicio) error {
	linhas := [][]string{cabecalhoExercicios}
	for _, e := range es {
		linhas = append(linhas, []string{
			e.ID, e.Repo, e.Titulo, e.Prazo.String(), formatarDecimal(e.Peso),
			e.Verificacao, e.Imagem, string(e.Situacao),
		})
	}
	return gravarCSV(s.caminho(arqExercicios), linhas)
}

// --- entregas ---

var cabecalhoEntregas = []string{
	"exercicio", "grr", "situacao", "projeto", "commit", "data_commit",
	"commits", "ultimo_commit", "data_ultimo", "atraso_dias", "coletado_em", "detalhe",
}

func (s *Store) lerEntregas() ([]turma.Entrega, error) {
	t, err := lerTabela(s.caminho(arqEntregas), cabecalhoEntregas, "exercicio")
	if err != nil {
		return nil, err
	}
	var out []turma.Entrega
	for i := range t.linhas {
		dataCommit, err := t.instante(i, "data_commit")
		if err != nil {
			return nil, err
		}
		dataUltimo, err := t.instante(i, "data_ultimo")
		if err != nil {
			return nil, err
		}
		coletado, err := t.instante(i, "coletado_em")
		if err != nil {
			return nil, err
		}
		commits, err := t.inteiro(i, "commits", 0)
		if err != nil {
			return nil, err
		}
		atraso, err := t.inteiro(i, "atraso_dias", 0)
		if err != nil {
			return nil, err
		}
		out = append(out, turma.Entrega{
			Exercicio:    t.str(i, "exercicio"),
			GRR:          turma.NormalizarGRR(t.str(i, "grr")),
			Situacao:     turma.SituacaoEntrega(t.str(i, "situacao")),
			Projeto:      t.str(i, "projeto"),
			Commit:       t.str(i, "commit"),
			DataCommit:   dataCommit,
			Commits:      commits,
			UltimoCommit: t.str(i, "ultimo_commit"),
			DataUltimo:   dataUltimo,
			AtrasoDias:   atraso,
			ColetadoEm:   coletado,
			Detalhe:      t.str(i, "detalhe"),
		})
	}
	return out, nil
}

func (s *Store) gravarEntregas(es []turma.Entrega) error {
	linhas := [][]string{cabecalhoEntregas}
	for _, e := range es {
		linhas = append(linhas, []string{
			e.Exercicio, e.GRR, string(e.Situacao), e.Projeto,
			e.Commit, turma.FormatarInstante(e.DataCommit),
			strconv.Itoa(e.Commits),
			e.UltimoCommit, turma.FormatarInstante(e.DataUltimo),
			strconv.Itoa(e.AtrasoDias), turma.FormatarInstante(e.ColetadoEm), e.Detalhe,
		})
	}
	return gravarCSV(s.caminho(arqEntregas), linhas)
}

// --- notas ---

var cabecalhoNotas = []string{"exercicio", "grr", "nota", "comentario", "corrigido_em"}

func (s *Store) lerNotas() ([]turma.Nota, error) {
	t, err := lerTabela(s.caminho(arqNotas), cabecalhoNotas, "exercicio")
	if err != nil {
		return nil, err
	}
	var out []turma.Nota
	for i := range t.linhas {
		valor, err := t.decimal(i, "nota", 0)
		if err != nil {
			return nil, err
		}
		corrigido, err := t.instante(i, "corrigido_em")
		if err != nil {
			return nil, err
		}
		out = append(out, turma.Nota{
			Exercicio:   t.str(i, "exercicio"),
			GRR:         turma.NormalizarGRR(t.str(i, "grr")),
			Valor:       valor,
			Comentario:  t.str(i, "comentario"),
			CorrigidoEm: corrigido,
		})
	}
	return out, nil
}

func (s *Store) gravarNotas(ns []turma.Nota) error {
	linhas := [][]string{cabecalhoNotas}
	for _, n := range ns {
		linhas = append(linhas, []string{
			n.Exercicio, n.GRR, formatarDecimal(n.Valor), n.Comentario,
			turma.FormatarInstante(n.CorrigidoEm),
		})
	}
	return gravarCSV(s.caminho(arqNotas), linhas)
}

// --- verificações ---

var cabecalhoVerificacoes = []string{
	"exercicio", "grr", "situacao", "aprovados", "total", "commit",
	"duracao_ms", "executado_em", "detalhe",
}

func (s *Store) lerVerificacoes() ([]turma.Verificacao, error) {
	t, err := lerTabela(s.caminho(arqVerificacoes), cabecalhoVerificacoes, "exercicio")
	if err != nil {
		return nil, err
	}
	var out []turma.Verificacao
	for i := range t.linhas {
		aprovados, err := t.inteiro(i, "aprovados", 0)
		if err != nil {
			return nil, err
		}
		total, err := t.inteiro(i, "total", 0)
		if err != nil {
			return nil, err
		}
		ms, err := t.inteiro(i, "duracao_ms", 0)
		if err != nil {
			return nil, err
		}
		quando, err := t.instante(i, "executado_em")
		if err != nil {
			return nil, err
		}
		out = append(out, turma.Verificacao{
			Exercicio:   t.str(i, "exercicio"),
			GRR:         turma.NormalizarGRR(t.str(i, "grr")),
			Situacao:    turma.SituacaoVerificacao(t.str(i, "situacao")),
			Aprovados:   aprovados,
			Total:       total,
			Commit:      t.str(i, "commit"),
			Duracao:     time.Duration(ms) * time.Millisecond,
			ExecutadoEm: quando,
			Detalhe:     t.str(i, "detalhe"),
		})
	}
	return out, nil
}

func (s *Store) gravarVerificacoes(vs []turma.Verificacao) error {
	linhas := [][]string{cabecalhoVerificacoes}
	for _, v := range vs {
		linhas = append(linhas, []string{
			v.Exercicio, v.GRR, string(v.Situacao),
			strconv.Itoa(v.Aprovados), strconv.Itoa(v.Total), v.Commit,
			strconv.FormatInt(v.Duracao.Milliseconds(), 10),
			turma.FormatarInstante(v.ExecutadoEm), v.Detalhe,
		})
	}
	return gravarCSV(s.caminho(arqVerificacoes), linhas)
}

// --- utilidades de CSV ---

func gravarCSV(caminho string, linhas [][]string) error {
	var b strings.Builder
	w := csv.NewWriter(&b)
	w.Comma = ';'
	if err := w.WriteAll(linhas); err != nil {
		return err
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return err
	}
	return escreverAtomico(caminho, []byte(b.String()))
}

// escreverAtomico grava via arquivo temporário e rename, para que uma
// interrupção no meio da escrita não deixe dado truncado numa pasta que o
// Nextcloud está sincronizando.
func escreverAtomico(caminho string, dados []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(caminho), ".tmp-*")
	if err != nil {
		return err
	}
	nome := tmp.Name()
	defer os.Remove(nome)

	if _, err := tmp.Write(dados); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(nome, 0o644); err != nil {
		return err
	}
	return os.Rename(nome, caminho)
}
