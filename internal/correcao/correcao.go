// Package correcao implementa a interface interativa de lançamento de notas.
package correcao

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/alexkutzke/gitlab-classroom/internal/turma"
)

var (
	estTitulo   = lipgloss.NewStyle().Bold(true)
	estFraco    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	estEntregue = lipgloss.NewStyle().Foreground(lipgloss.Color("35"))
	estAtraso   = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	estFalta    = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	estNota     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	estCursor   = lipgloss.NewStyle().Bold(true)
	estFiltro   = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	estAviso    = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
)

// Item é uma linha da correção: o aluno, o que a coleta apurou e a nota que
// já existe, se existir.
type Item struct {
	Aluno   turma.Aluno
	Entrega turma.Entrega
	// Verificacao é o resultado da suíte automatizada, quando o exercício
	// tem uma. Situação vazia significa que não foi verificado.
	Verificacao turma.Verificacao
	// Dir é o clone local. Vazio ou inexistente desabilita a abertura no
	// editor.
	Dir string

	nota       float64
	temNota    bool
	comentario string
	// alterado marca o que mudou nesta sessão, para gravar só isso.
	alterado bool
	removido bool
}

// Opcoes descreve a correção a ser feita.
type Opcoes struct {
	Exercicio  turma.Exercicio
	NotaMaxima float64
	Itens      []Item
}

// Resultado é o que a correção devolve ao comando.
type Resultado struct {
	Salvar    bool
	Notas     []turma.Nota
	Removidas []string
}

type modo int

const (
	navegando modo = iota
	digitandoNota
	digitandoComentario
	digitandoFiltro
)

type modelo struct {
	exercicio  turma.Exercicio
	notaMaxima float64

	itens   []Item
	visivel []int

	cursor  int
	topo    int
	altura  int
	largura int

	modo   modo
	buffer string
	filtro string

	// ultimaNota e ultimoComentario alimentam a tecla de repetição, que
	// encurta a correção de uma turma inteira com o mesmo veredito.
	ultimaNota       float64
	temUltimaNota    bool
	ultimoComentario string

	aviso  string
	salvar bool
}

// Executar abre a interface e devolve as notas lançadas.
func Executar(o Opcoes) (Resultado, error) {
	if len(o.Itens) == 0 {
		return Resultado{}, fmt.Errorf("nenhum aluno a corrigir em %s", o.Exercicio.ID)
	}
	if o.NotaMaxima <= 0 {
		o.NotaMaxima = 100
	}

	m := modelo{
		exercicio:  o.Exercicio,
		notaMaxima: o.NotaMaxima,
		itens:      o.Itens,
		altura:     20,
		largura:    100,
	}
	m.filtrar()

	p := tea.NewProgram(&m)
	saida, err := p.Run()
	if err != nil {
		return Resultado{}, err
	}
	final := saida.(*modelo)
	if !final.salvar {
		return Resultado{}, nil
	}

	res := Resultado{Salvar: true}
	agora := time.Now()
	for _, i := range final.itens {
		switch {
		case i.removido:
			res.Removidas = append(res.Removidas, i.Aluno.GRR)
		case i.alterado && i.temNota:
			res.Notas = append(res.Notas, turma.Nota{
				Exercicio: final.exercicio.ID, GRR: i.Aluno.GRR,
				Valor: i.nota, Comentario: i.comentario, CorrigidoEm: agora,
			})
		}
	}
	return res, nil
}

// Preencher carrega a nota já lançada em um item.
func (i *Item) Preencher(n turma.Nota) {
	i.nota, i.temNota, i.comentario = n.Valor, true, n.Comentario
}

func (m *modelo) Init() tea.Cmd { return nil }

func (m *modelo) filtrar() {
	m.visivel = m.visivel[:0]
	termo := turma.ChaveNome(m.filtro)
	for i, it := range m.itens {
		if termo == "" {
			m.visivel = append(m.visivel, i)
			continue
		}
		alvo := turma.ChaveNome(it.Aluno.Nome + " " + it.Aluno.GRR)
		if strings.Contains(alvo, termo) {
			m.visivel = append(m.visivel, i)
		}
	}
	if m.cursor >= len(m.visivel) {
		m.cursor = max(0, len(m.visivel)-1)
	}
}

func (m *modelo) atual() *Item {
	if len(m.visivel) == 0 {
		return nil
	}
	return &m.itens[m.visivel[m.cursor]]
}

func (m *modelo) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.largura = msg.Width
		m.altura = max(5, msg.Height-9)
		return m, nil
	case erroAbertura:
		if msg.err != nil {
			m.aviso = msg.err.Error()
		}
		return m, nil
	case tea.KeyMsg:
		switch m.modo {
		case digitandoNota:
			return m, m.teclaNota(msg)
		case digitandoComentario:
			return m, m.teclaComentario(msg)
		case digitandoFiltro:
			return m, m.teclaFiltro(msg)
		}
		return m, m.teclaNavegacao(msg)
	}
	return m, nil
}

func (m *modelo) teclaNavegacao(msg tea.KeyMsg) tea.Cmd {
	m.aviso = ""
	switch msg.String() {
	case "q", "esc", "ctrl+c":
		m.salvar = false
		return tea.Quit
	case "enter":
		m.salvar = true
		return tea.Quit
	case "up", "k":
		m.mover(-1)
	case "down", "j":
		m.mover(1)
	case "pgup":
		m.mover(-m.altura)
	case "pgdown":
		m.mover(m.altura)
	case "home", "g":
		m.cursor = 0
		m.ajustarJanela()
	case "end", "G":
		m.cursor = max(0, len(m.visivel)-1)
		m.ajustarJanela()
	case "/":
		m.modo = digitandoFiltro
		m.buffer = m.filtro
	case "n":
		if it := m.atual(); it != nil {
			m.modo = digitandoNota
			m.buffer = ""
			if it.temNota {
				m.buffer = formatarNota(it.nota)
			}
		}
	case "c":
		if it := m.atual(); it != nil {
			m.modo = digitandoComentario
			m.buffer = it.comentario
		}
	case "x":
		if it := m.atual(); it != nil && it.temNota {
			it.temNota, it.nota, it.comentario = false, 0, ""
			it.removido, it.alterado = true, true
			m.mover(1)
		}
	case "r":
		m.repetir()
	case "o":
		return m.abrir()
	default:
		// Dígito começa a digitar a nota direto, que é o caminho mais curto
		// para quem já sabe o valor.
		if len(msg.String()) == 1 && msg.String() >= "0" && msg.String() <= "9" {
			if it := m.atual(); it != nil {
				m.modo = digitandoNota
				m.buffer = msg.String()
				_ = it
			}
		}
	}
	return nil
}

func (m *modelo) teclaNota(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		m.modo, m.buffer = navegando, ""
	case "enter":
		it := m.atual()
		if it == nil {
			m.modo = navegando
			return nil
		}
		texto := strings.TrimSpace(m.buffer)
		if texto == "" {
			m.modo, m.buffer = navegando, ""
			return nil
		}
		valor, err := strconv.ParseFloat(strings.Replace(texto, ",", ".", 1), 64)
		if err != nil {
			m.aviso = fmt.Sprintf("nota inválida: %q", texto)
			return nil
		}
		if valor < 0 || valor > m.notaMaxima {
			m.aviso = fmt.Sprintf("nota fora da escala 0 a %s", formatarNota(m.notaMaxima))
			return nil
		}
		it.nota, it.temNota, it.alterado, it.removido = valor, true, true, false
		m.ultimaNota, m.temUltimaNota = valor, true
		m.ultimoComentario = it.comentario
		m.modo, m.buffer = navegando, ""
		m.mover(1)
	case "backspace":
		if m.buffer != "" {
			m.buffer = m.buffer[:len(m.buffer)-1]
		}
	default:
		if t := msg.String(); len(t) == 1 && (t == "." || t == "," || (t >= "0" && t <= "9")) {
			m.buffer += t
		}
	}
	return nil
}

func (m *modelo) teclaComentario(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		m.modo, m.buffer = navegando, ""
	case "enter":
		if it := m.atual(); it != nil {
			it.comentario = strings.TrimSpace(m.buffer)
			it.alterado = true
			if !it.temNota {
				m.aviso = "comentário guardado; lance a nota com n para ele ser gravado"
			}
			m.ultimoComentario = it.comentario
		}
		m.modo, m.buffer = navegando, ""
	case "backspace":
		if m.buffer != "" {
			m.buffer = m.buffer[:len(m.buffer)-1]
		}
	case "space":
		m.buffer += " "
	default:
		if t := msg.String(); len(t) == 1 {
			m.buffer += t
		}
	}
	return nil
}

func (m *modelo) teclaFiltro(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		m.modo, m.buffer, m.filtro = navegando, "", ""
		m.filtrar()
	case "enter":
		m.filtro = m.buffer
		m.modo, m.buffer = navegando, ""
		m.cursor = 0
		m.filtrar()
	case "backspace":
		if m.buffer != "" {
			m.buffer = m.buffer[:len(m.buffer)-1]
		}
	case "space":
		m.buffer += " "
	default:
		if t := msg.String(); len(t) == 1 {
			m.buffer += t
		}
	}
	return nil
}

// repetir aplica ao aluno atual a última nota lançada, com o comentário que a
// acompanhou.
func (m *modelo) repetir() {
	it := m.atual()
	if it == nil {
		return
	}
	if !m.temUltimaNota {
		m.aviso = "nenhuma nota lançada ainda nesta sessão"
		return
	}
	it.nota, it.temNota, it.alterado, it.removido = m.ultimaNota, true, true, false
	if it.comentario == "" {
		it.comentario = m.ultimoComentario
	}
	m.mover(1)
}

type erroAbertura struct{ err error }

// abrir entrega o clone ao $EDITOR, devolvendo o terminal à interface quando
// o editor fecha.
func (m *modelo) abrir() tea.Cmd {
	it := m.atual()
	if it == nil {
		return nil
	}
	if it.Dir == "" {
		m.aviso = "sem clone local: rode `classroom clonar --exercicio " + m.exercicio.ID + "`"
		return nil
	}
	if _, err := os.Stat(it.Dir); err != nil {
		m.aviso = "clone ausente em " + it.Dir
		return nil
	}

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor == "" {
		if _, err := exec.LookPath("xdg-open"); err != nil {
			m.aviso = "defina $EDITOR para abrir o repositório"
			return nil
		}
		if err := exec.Command("xdg-open", it.Dir).Start(); err != nil {
			m.aviso = err.Error()
		}
		return nil
	}
	partes := strings.Fields(editor)
	cmd := exec.Command(partes[0], append(partes[1:], it.Dir)...)
	return tea.ExecProcess(cmd, func(err error) tea.Msg { return erroAbertura{err} })
}

func (m *modelo) mover(delta int) {
	if len(m.visivel) == 0 {
		return
	}
	m.cursor = min(max(0, m.cursor+delta), len(m.visivel)-1)
	m.ajustarJanela()
}

func (m *modelo) ajustarJanela() {
	if m.cursor < m.topo {
		m.topo = m.cursor
	}
	if m.cursor >= m.topo+m.altura {
		m.topo = m.cursor - m.altura + 1
	}
	if m.topo < 0 {
		m.topo = 0
	}
}

func formatarNota(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
