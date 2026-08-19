package store

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/alexkutzke/gitlab-classroom/internal/turma"
)

// tabela é um CSV cujas colunas são localizadas pelo nome no cabeçalho, e não
// pela posição.
//
// Ler por nome permite acrescentar colunas sem invalidar arquivos já gravados
// e tolera que elas sejam reordenadas na edição manual. Coluna ausente devolve
// valor vazio, de modo que o chamador aplica o padrão que fizer sentido.
type tabela struct {
	arquivo string
	col     map[string]int
	linhas  [][]string
	numeros []int // linha correspondente no arquivo, para mensagens de erro
}

// lerTabela carrega um CSV. `padrao` é a ordem posicional usada como reserva
// quando o cabeçalho não é reconhecível, e `obrigatoria` é a coluna cuja
// presença define se o cabeçalho foi entendido.
//
// Arquivo ausente é tratado como vazio, para o init poder criar os arquivos
// aos poucos.
func lerTabela(caminho string, padrao []string, obrigatoria string) (*tabela, error) {
	nome := filepath.Base(caminho)

	f, err := os.Open(caminho)
	if err != nil {
		if os.IsNotExist(err) {
			return &tabela{arquivo: nome, col: map[string]int{}}, nil
		}
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.Comma = ';'
	r.FieldsPerRecord = -1 // tolera linhas curtas editadas à mão
	r.TrimLeadingSpace = true
	registros, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("lendo %s: %w", nome, err)
	}
	if len(registros) == 0 {
		return &tabela{arquivo: nome, col: map[string]int{}}, nil
	}

	t := &tabela{arquivo: nome, col: map[string]int{}}
	for i, c := range registros[0] {
		if n := normalizarColuna(c); n != "" {
			t.col[n] = i
		}
	}
	if _, ok := t.col[obrigatoria]; !ok {
		// Cabeçalho irreconhecível: assume a ordem posicional conhecida.
		t.col = map[string]int{}
		for i, n := range padrao {
			t.col[n] = i
		}
	}

	for i, l := range registros[1:] {
		if len(l) == 0 || (len(l) == 1 && strings.TrimSpace(l[0]) == "") {
			continue // ignora linhas em branco deixadas por edição manual
		}
		t.linhas = append(t.linhas, l)
		t.numeros = append(t.numeros, i+2)
	}
	return t, nil
}

// numero devolve a linha do arquivo correspondente ao índice, para erros.
func (t *tabela) numero(i int) int {
	if i < len(t.numeros) {
		return t.numeros[i]
	}
	return i + 2
}

// str devolve o campo já sem espaços em volta. Coluna ausente devolve "".
func (t *tabela) str(i int, coluna string) string {
	c, ok := t.col[coluna]
	if !ok || c >= len(t.linhas[i]) {
		return ""
	}
	return strings.TrimSpace(t.linhas[i][c])
}

// inteiro lê um campo numérico, aplicando padrao quando estiver vazio.
func (t *tabela) inteiro(i int, coluna string, padrao int) (int, error) {
	v := t.str(i, coluna)
	if v == "" {
		return padrao, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s linha %d: %s inválido %q", t.arquivo, t.numero(i), coluna, v)
	}
	return n, nil
}

// decimal lê um campo com vírgula ou ponto como separador, porque planilha em
// português grava com vírgula.
func (t *tabela) decimal(i int, coluna string, padrao float64) (float64, error) {
	v := t.str(i, coluna)
	if v == "" {
		return padrao, nil
	}
	n, err := strconv.ParseFloat(strings.Replace(v, ",", ".", 1), 64)
	if err != nil {
		return 0, fmt.Errorf("%s linha %d: %s inválido %q", t.arquivo, t.numero(i), coluna, v)
	}
	return n, nil
}

func (t *tabela) data(i int, coluna string) (turma.Data, error) {
	d, err := turma.ParseData(t.str(i, coluna))
	if err != nil {
		return turma.Data{}, fmt.Errorf("%s linha %d: %w", t.arquivo, t.numero(i), err)
	}
	return d, nil
}

func (t *tabela) instante(i int, coluna string) (time.Time, error) {
	v, err := turma.ParseInstante(t.str(i, coluna))
	if err != nil {
		return time.Time{}, fmt.Errorf("%s linha %d: %w", t.arquivo, t.numero(i), err)
	}
	return v, nil
}

// normalizarColuna descarta o BOM que planilhas costumam deixar na primeira
// célula do arquivo, além de espaços e caixa.
func normalizarColuna(s string) string {
	return strings.ToLower(strings.TrimSpace(strings.TrimPrefix(s, "\ufeff")))
}

func formatarDecimal(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}
