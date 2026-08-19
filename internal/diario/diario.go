// Package diario lê o cadastro de alunos mantido pela aplicação diario.
//
// A leitura é do arquivo, e não do código do diario: as duas aplicações
// compartilham o formato do CSV, não uma biblioteca. O acoplamento fica no
// cabeçalho das colunas, que é lido por nome e tolera ausências.
package diario

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/alexkutzke/gitlab-classroom/internal/turma"
)

// Aluno é o que interessa aqui do cadastro do SIGA. O CPF é ignorado de
// propósito: a coleta de entregas não tem uso para ele.
type Aluno struct {
	GRR      string
	Nome     string
	Email    string
	Situacao turma.Situacao
}

// ErrSemCadastro indica que não há alunos.csv na pasta informada.
type ErrSemCadastro struct{ Caminho string }

func (e *ErrSemCadastro) Error() string {
	return fmt.Sprintf("cadastro do diario não encontrado em %s", e.Caminho)
}

// Ler carrega o cadastro a partir do diretório .diario/ informado.
func Ler(pastaDiario string) ([]Aluno, error) {
	caminho := filepath.Join(pastaDiario, "alunos.csv")
	f, err := os.Open(caminho)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, &ErrSemCadastro{Caminho: caminho}
		}
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.Comma = ';'
	r.FieldsPerRecord = -1
	r.TrimLeadingSpace = true
	registros, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("lendo %s: %w", caminho, err)
	}
	if len(registros) == 0 {
		return nil, nil
	}

	col := map[string]int{}
	for i, c := range registros[0] {
		n := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(c, "\ufeff")))
		if n != "" {
			col[n] = i
		}
	}
	if _, ok := col["grr"]; !ok {
		return nil, fmt.Errorf("%s não parece o cadastro do diario: falta a coluna grr", caminho)
	}

	campo := func(l []string, nome string) string {
		i, ok := col[nome]
		if !ok || i >= len(l) {
			return ""
		}
		return strings.TrimSpace(l[i])
	}

	var out []Aluno
	for _, l := range registros[1:] {
		if len(l) == 0 || (len(l) == 1 && strings.TrimSpace(l[0]) == "") {
			continue
		}
		grr := turma.NormalizarGRR(campo(l, "grr"))
		if grr == "" {
			continue
		}
		sit := turma.Situacao(campo(l, "situacao"))
		if sit == "" {
			sit = turma.Ativo
		}
		out = append(out, Aluno{
			GRR:      grr,
			Nome:     campo(l, "nome"),
			Email:    campo(l, "email"),
			Situacao: sit,
		})
	}
	return out, nil
}

// Resultado resume o que a importação mudou, para o comando poder relatar.
type Resultado struct {
	Novos       []string
	Atualizados []string
	Cancelados  []string
}

// Vazio informa se nada mudou no cadastro.
func (r Resultado) Vazio() bool {
	return len(r.Novos) == 0 && len(r.Atualizados) == 0 && len(r.Cancelados) == 0
}

// Importar reconcilia o cadastro do diario com o da turma.
//
// Nome, e-mail e situação vêm do SIGA e sobrescrevem o que estava aqui, que é
// cópia. Usuário, grupo e situação da conta são apurados contra o GitLab e
// nunca são tocados por esta função. Ninguém é apagado: quem sai da lista do
// SIGA vira cancelado e conserva as entregas já coletadas.
func Importar(t *turma.Turma, cadastro []Aluno) Resultado {
	var res Resultado
	presentes := map[string]bool{}

	for _, c := range cadastro {
		presentes[c.GRR] = true
		a, ok := t.AlunoPorGRR(c.GRR)
		if !ok {
			t.Alunos = append(t.Alunos, turma.Aluno{
				GRR:      c.GRR,
				Nome:     c.Nome,
				Email:    c.Email,
				Usuario:  turma.UsuarioGitLab(c.GRR),
				Situacao: c.Situacao,
			})
			res.Novos = append(res.Novos, c.GRR)
			continue
		}
		if a.Nome != c.Nome || a.Email != c.Email || a.Situacao != c.Situacao {
			a.Nome, a.Email, a.Situacao = c.Nome, c.Email, c.Situacao
			res.Atualizados = append(res.Atualizados, c.GRR)
		}
	}

	for i := range t.Alunos {
		a := &t.Alunos[i]
		if !presentes[a.GRR] && a.Situacao != turma.Cancelado {
			a.Situacao = turma.Cancelado
			res.Cancelados = append(res.Cancelados, a.GRR)
		}
	}

	t.Ordenar()
	return res
}

// Config são os metadados da turma que o diario já guarda e que servem para
// preencher o config.toml sem redigitação.
type Config struct {
	Codigo     string `toml:"codigo"`
	Disciplina string `toml:"disciplina"`
	Turma      string `toml:"turma"`
	Semestre   string `toml:"semestre"`
	Professor  string `toml:"professor"`
}

// LerConfig lê o turma.toml do diario. Ausência do arquivo devolve config
// vazia, sem erro: o cadastro pode existir sem ele.
func LerConfig(pastaDiario string) (Config, error) {
	var c Config
	caminho := filepath.Join(pastaDiario, "turma.toml")
	if _, err := os.Stat(caminho); err != nil {
		return c, nil
	}
	if _, err := toml.DecodeFile(caminho, &c); err != nil {
		return c, fmt.Errorf("lendo %s: %w", caminho, err)
	}
	return c, nil
}
