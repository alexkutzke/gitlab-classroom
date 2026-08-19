package turma

import (
	"strings"
	"unicode"
)

// ChaveNome normaliza um texto para comparação: sem acentuação, sem caixa e
// com os espaços colapsados.
//
// É o que permite cruzar o nome vindo do SIGA com o nome digitado na busca
// sem depender de o professor acertar os acentos.
func ChaveNome(s string) string {
	var b strings.Builder
	espaco := false
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		r = semAcento(r)
		if unicode.IsSpace(r) {
			espaco = true
			continue
		}
		if espaco && b.Len() > 0 {
			b.WriteRune(' ')
		}
		espaco = false
		b.WriteRune(r)
	}
	return b.String()
}

// semAcento troca as letras acentuadas do português pela versão simples.
// Uma tabela explícita evita depender de golang.org/x/text só para isto.
var acentos = map[rune]rune{
	'á': 'a', 'à': 'a', 'ã': 'a', 'â': 'a', 'ä': 'a',
	'é': 'e', 'è': 'e', 'ê': 'e', 'ë': 'e',
	'í': 'i', 'ì': 'i', 'î': 'i', 'ï': 'i',
	'ó': 'o', 'ò': 'o', 'õ': 'o', 'ô': 'o', 'ö': 'o',
	'ú': 'u', 'ù': 'u', 'û': 'u', 'ü': 'u',
	'ç': 'c', 'ñ': 'n',
}

func semAcento(r rune) rune {
	if s, ok := acentos[r]; ok {
		return s
	}
	return r
}

// NormalizarGRR devolve o GRR em maiúsculas, sem espaços, como o SIGA grava.
func NormalizarGRR(s string) string {
	return strings.ToUpper(strings.TrimSpace(s))
}

// UsuarioGitLab devolve o nome de usuário esperado no gitlab.com, que é o GRR
// em minúsculas, conforme as instruções de submissão dadas à turma.
func UsuarioGitLab(grr string) string {
	return strings.ToLower(strings.TrimSpace(grr))
}
