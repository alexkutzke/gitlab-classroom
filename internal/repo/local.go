package repo

import (
	"path/filepath"
	"strings"

	"github.com/alexkutzke/gitlab-classroom/internal/turma"
)

// Base devolve a pasta onde ficam os clones de um exercício.
func Base(pastaTurma, pastaEntregas, exercicio string) string {
	if pastaEntregas == "" {
		pastaEntregas = "entregas"
	}
	if !filepath.IsAbs(pastaEntregas) {
		pastaEntregas = filepath.Join(pastaTurma, pastaEntregas)
	}
	return filepath.Join(pastaEntregas, exercicio)
}

// Caminho devolve a pasta do clone de um aluno em um exercício.
//
// O nome entra junto do GRR porque a pasta é aberta à mão durante a correção,
// e um diretório só com o GRR obrigaria a consultar o cadastro a cada vez.
func Caminho(pastaTurma, pastaEntregas, exercicio string, a turma.Aluno) string {
	nome := Slug(a.Nome)
	pasta := strings.ToLower(a.GRR)
	if nome != "" {
		pasta += "-" + nome
	}
	return filepath.Join(Base(pastaTurma, pastaEntregas, exercicio), pasta)
}

// Slug reduz o nome do aluno a letras, números e hifens.
func Slug(nome string) string {
	chave := turma.ChaveNome(nome)
	var b strings.Builder
	for _, r := range chave {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			if b.Len() > 0 && !strings.HasSuffix(b.String(), "-") {
				b.WriteByte('-')
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// NomeRamo é o ramo local criado no clone para apontar o commit avaliado.
func NomeRamo(exercicio string) string { return "entrega/" + exercicio }
