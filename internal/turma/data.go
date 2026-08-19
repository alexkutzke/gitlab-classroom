package turma

import (
	"fmt"
	"strings"
	"time"
)

// LayoutData é o formato usado em todos os arquivos de dados.
const LayoutData = "2006-01-02"

// Data é uma data de calendário, sem horário nem fuso.
type Data struct {
	time.Time
}

// NovaData constrói uma Data a partir de ano, mês e dia.
func NovaData(ano int, mes time.Month, dia int) Data {
	return Data{time.Date(ano, mes, dia, 0, 0, 0, 0, time.UTC)}
}

// Hoje devolve a data corrente no fuso local.
func Hoje() Data {
	a, m, d := time.Now().Date()
	return NovaData(a, m, d)
}

// ParseData lê uma data no formato AAAA-MM-DD. String vazia devolve a data zero.
func ParseData(s string) (Data, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Data{}, nil
	}
	t, err := time.ParseInLocation(LayoutData, s, time.UTC)
	if err != nil {
		return Data{}, fmt.Errorf("data inválida %q: use o formato AAAA-MM-DD", s)
	}
	return Data{t}, nil
}

// String devolve a data em AAAA-MM-DD, ou "" se for a data zero.
func (d Data) String() string {
	if d.IsZero() {
		return ""
	}
	return d.Format(LayoutData)
}

// Curta devolve a data em dd/mm, para cabeçalhos de tabela.
func (d Data) Curta() string {
	if d.IsZero() {
		return ""
	}
	return d.Format("02/01")
}

// Antes informa se d é anterior a outra data.
func (d Data) Antes(o Data) bool { return d.Time.Before(o.Time) }

// Depois informa se d é posterior a outra data.
func (d Data) Depois(o Data) bool { return d.Time.After(o.Time) }

// FimDoDia devolve o último instante do dia no fuso local.
//
// É o corte real de um prazo: entregar às 23h50 do dia marcado está no prazo.
// O fuso é o local porque é nele que o aluno lê a data no enunciado, ainda
// que o GitLab devolva os commits em UTC.
func (d Data) FimDoDia() time.Time {
	if d.IsZero() {
		return time.Time{}
	}
	a, m, dia := d.Date()
	return time.Date(a, m, dia, 23, 59, 59, int(time.Second-1), time.Local)
}

// InstanteLocal devolve o começo do dia no fuso local.
func (d Data) InstanteLocal() time.Time {
	if d.IsZero() {
		return time.Time{}
	}
	a, m, dia := d.Date()
	return time.Date(a, m, dia, 0, 0, 0, 0, time.Local)
}

// DataDe converte um instante em data de calendário, no fuso local.
func DataDe(t time.Time) Data {
	if t.IsZero() {
		return Data{}
	}
	a, m, d := t.In(time.Local).Date()
	return NovaData(a, m, d)
}

// LayoutInstante é o formato dos carimbos de tempo gravados nos arquivos.
const LayoutInstante = "2006-01-02T15:04:05Z07:00"

// FormatarInstante grava um instante em RFC 3339, ou "" se for zero.
func FormatarInstante(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(LayoutInstante)
}

// ParseInstante lê um carimbo de tempo em RFC 3339. Vazio devolve o zero.
func ParseInstante(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("instante inválido %q: use o formato RFC 3339", s)
	}
	return t, nil
}
