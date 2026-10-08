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

	"github.com/alexkutzke/gitlab-classroom/internal/moldura"
	"github.com/alexkutzke/gitlab-classroom/internal/turma"
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
	// editor. Na entrega em dupla, aponta para o clone do dono do fork.
	Dir string
	// Equipe traz os demais integrantes da entrega, quando há.
	Equipe []string
	// Devolutiva é a issue já publicada para esta entrega, se houver. O
	// detalhe a compara com o comentário em edição para dizer se ela ficou
	// desatualizada.
	Devolutiva *turma.Devolutiva

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
	// Propagar repete a nota nos demais integrantes de uma entrega em dupla,
	// que é o que se quer quase sempre. A tecla D desliga durante a sessão.
	Propagar bool
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
	altura  int
	largura int

	// foco é o painel em foco: 0 a lista, 1 o detalhe. topoDet é a rolagem
	// do detalhe, que volta ao topo quando o cursor muda de aluno.
	foco    int
	topoDet int
	ajuda   bool
	// anuncio vai à direita da barra de título. A interface usa para mostrar
	// a tarefa em curso, que fica à vista em qualquer tela.
	anuncio string

	modo   modo
	buffer string
	filtro string

	// ultimaNota e ultimoComentario alimentam a tecla de repetição, que
	// encurta a correção de uma turma inteira com o mesmo veredito.
	ultimaNota       float64
	temUltimaNota    bool
	ultimoComentario string

	// propagar repete a nota nos demais integrantes da entrega em dupla.
	propagar bool

	// autonomo distingue a tela aberta como programa próprio, pelo
	// subcomando, da mesma tela embutida na interface: no segundo caso sair
	// devolve o controle à aplicação, em vez de encerrá-la.
	autonomo bool

	aviso     string
	salvar    bool
	encerrada bool
}

// Executar abre a correção como programa próprio, que é como o subcomando a
// usa.
func Executar(o Opcoes) (Resultado, error) {
	s, err := Nova(o)
	if err != nil {
		return Resultado{}, err
	}
	s.m.autonomo = true

	saida, err := tea.NewProgram(s.m, tea.WithAltScreen()).Run()
	if err != nil {
		return Resultado{}, err
	}
	return (&Sessao{m: saida.(*modelo)}).Resultado(), nil
}

// Sessao é a correção embutida em outra aplicação. A interface interativa a
// usa como subtela, sem abrir um programa Bubble Tea próprio.
type Sessao struct{ m *modelo }

// Nova prepara a correção.
func Nova(o Opcoes) (*Sessao, error) {
	if len(o.Itens) == 0 {
		return nil, fmt.Errorf("nenhum aluno a corrigir em %s", o.Exercicio.ID)
	}
	if o.NotaMaxima <= 0 {
		o.NotaMaxima = 100
	}
	m := &modelo{
		exercicio:  o.Exercicio,
		notaMaxima: o.NotaMaxima,
		itens:      o.Itens,
		propagar:   o.Propagar,
		altura:     moldura.AlturaPadrao,
		largura:    moldura.LarguraPadrao,
	}
	m.filtrar()
	return &Sessao{m: m}, nil
}

// Atualizar entrega uma mensagem à correção.
func (s *Sessao) Atualizar(msg tea.Msg) tea.Cmd {
	_, cmd := s.m.Update(msg)
	return cmd
}

// Dimensionar informa o tamanho da tela. A correção ocupa o terminal
// inteiro, com as próprias barras de título e de teclas.
func (s *Sessao) Dimensionar(largura, altura int) {
	s.m.largura, s.m.altura = largura, altura
}

// Anunciar põe um texto à direita da barra de título, como o andamento de
// uma tarefa da interface que hospeda a correção.
func (s *Sessao) Anunciar(texto string) { s.m.anuncio = texto }

// View desenha a correção.
func (s *Sessao) View() string { return s.m.View() }

// Encerrada informa se o professor saiu da correção.
func (s *Sessao) Encerrada() bool { return s.m.encerrada }

// Resultado traz as notas lançadas. Vem vazio quando a saída foi sem gravar.
func (s *Sessao) Resultado() Resultado {
	if !s.m.salvar {
		return Resultado{}
	}
	res := Resultado{Salvar: true}
	agora := time.Now()
	for _, i := range s.m.itens {
		switch {
		case i.removido:
			res.Removidas = append(res.Removidas, i.Aluno.GRR)
		case i.alterado && i.temNota:
			res.Notas = append(res.Notas, turma.Nota{
				Exercicio: s.m.exercicio.ID, GRR: i.Aluno.GRR,
				Valor: i.nota, Comentario: i.comentario, CorrigidoEm: agora,
			})
		}
	}
	return res
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
		m.largura, m.altura = msg.Width, msg.Height
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
	// A tecla que fecha a ajuda não age por baixo dela.
	if m.ajuda {
		m.ajuda = false
		return nil
	}
	if m.foco == 1 && m.rolarDetalhe(msg.String()) {
		return nil
	}
	switch msg.String() {
	case "?":
		m.ajuda = true
	case "tab", "shift+tab":
		m.foco = 1 - m.foco
	case "esc":
		// Com o foco no detalhe, esc devolve o foco à lista; na lista, sai
		// sem gravar, como sempre.
		if m.foco == 1 {
			m.foco = 0
			return nil
		}
		return m.encerrar(false)
	case "q", "ctrl+c":
		return m.encerrar(false)
	case "enter":
		return m.encerrar(true)
	case "up", "k":
		m.mover(-1)
	case "down", "j":
		m.mover(1)
	case "pgup":
		m.mover(-m.linhasVisiveis())
	case "pgdown":
		m.mover(m.linhasVisiveis())
	case "home", "g":
		m.mover(-len(m.visivel))
	case "end", "G":
		m.mover(len(m.visivel))
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
			m.propagarDe(*it)
			m.mover(1)
		}
	case "D":
		m.propagar = !m.propagar
		if m.propagar {
			m.aviso = "nota da entrega em dupla vai para os dois integrantes"
		} else {
			m.aviso = "nota lançada só para o aluno sob o cursor"
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

// encerrar fecha a correção, salvando ou não.
func (m *modelo) encerrar(salvar bool) tea.Cmd {
	m.salvar, m.encerrada = salvar, true
	if m.autonomo {
		return tea.Quit
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
		m.propagarDe(*it)
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
	m.propagarDe(*it)
	m.mover(1)
}

// propagarDe repete nota, comentário e remoção nos demais integrantes da
// mesma entrega.
//
// Uma entrega em dupla é um trabalho só, e lançar duas notas diferentes seria
// acidente quase sempre. A tecla D desliga isso quando a intenção for mesmo
// avaliar um integrante à parte.
func (m *modelo) propagarDe(origem Item) {
	if !m.propagar || len(origem.Equipe) == 0 {
		return
	}
	iguais := 0
	for i := range m.itens {
		outro := &m.itens[i]
		if outro.Aluno.GRR == origem.Aluno.GRR || !mesmaEntrega(origem, *outro) {
			continue
		}
		outro.nota, outro.temNota = origem.nota, origem.temNota
		outro.comentario, outro.alterado = origem.comentario, true
		outro.removido = origem.removido
		iguais++
	}
	if iguais > 0 {
		m.aviso = fmt.Sprintf("mesma nota aplicada a %s", strings.Join(origem.Equipe, ", "))
	}
}

// mesmaEntrega informa se os dois alunos entregaram no mesmo fork.
func mesmaEntrega(a, b Item) bool {
	return a.Entrega.Projeto != "" && a.Entrega.Projeto == b.Entrega.Projeto
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
	// O detalhe segue o cursor, e o aluno novo começa do topo.
	m.topoDet = 0
}

// rolarDetalhe move o detalhe em foco; devolve false para as teclas que não
// são de rolagem, como as de nota, que continuam valendo daqui.
func (m *modelo) rolarDetalhe(tecla string) bool {
	w, h := m.retanguloDetalhe()
	visiveis := max(1, h-2)
	limite := max(0, len(m.linhasDetalhe(w))-visiveis)
	switch tecla {
	case "down", "j":
		m.topoDet++
	case "up", "k":
		m.topoDet--
	case "pgdown":
		m.topoDet += visiveis
	case "pgup":
		m.topoDet -= visiveis
	case "home", "g":
		m.topoDet = 0
	case "end", "G":
		m.topoDet = limite
	default:
		return false
	}
	m.topoDet = max(0, min(m.topoDet, limite))
	return true
}

func formatarNota(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
