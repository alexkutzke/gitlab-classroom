package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/alexkutzke/gitlab-classroom/internal/acoes"
	"github.com/alexkutzke/gitlab-classroom/internal/turma"
)

// confirmacao é a pergunta que precede uma ação difícil de desfazer.
type confirmacao struct {
	pergunta string
	acao     func() tea.Cmd
}

// pedirConfirmacao guarda a pergunta; a resposta chega por responderConfirmacao.
func (a *App) pedirConfirmacao(pergunta string, acao func() tea.Cmd) {
	a.confirmar = &confirmacao{pergunta: pergunta, acao: acao}
}

func (a *App) responderConfirmacao(msg tea.KeyMsg) tea.Cmd {
	c := a.confirmar
	switch msg.String() {
	case "s", "S", "enter":
		a.confirmar = nil
		return c.acao()
	case "n", "N", "esc", "q":
		a.confirmar = nil
		a.avisar("cancelado")
	}
	return nil
}

// campoExercicio identifica o campo em edição no formulário.
type campoExercicio int

const (
	campoNenhum campoExercicio = iota
	campoTitulo
	campoPrazo
	campoPeso
	campoVerificacao
	campoImagem
	campoCategoria
	campoRepo
	campoID
)

func (c campoExercicio) String() string {
	switch c {
	case campoTitulo:
		return "título"
	case campoPrazo:
		return "prazo (AAAA-MM-DD)"
	case campoPeso:
		return "peso"
	case campoVerificacao:
		return "comando da suíte"
	case campoImagem:
		return "imagem do contêiner"
	case campoCategoria:
		return "categoria (exercicio, trabalho)"
	case campoRepo:
		return "repositório-modelo"
	case campoID:
		return "id"
	}
	return ""
}

// telaExercicios lista e edita os exercícios da disciplina.
type telaExercicios struct {
	cursor int
	topo   int
	// arquivados mostra também o que saiu do cronograma.
	arquivados bool

	campo  campoExercicio
	buffer string
	// novo guarda o exercício sendo cadastrado, antes de ele existir.
	novo *turma.Exercicio
}

func (tx *telaExercicios) lista(a *App) []turma.Exercicio {
	if tx.arquivados {
		return a.turma.Exercicios
	}
	return a.turma.ExerciciosAtivos()
}

func (tx *telaExercicios) atual(a *App) (turma.Exercicio, bool) {
	lista := tx.lista(a)
	if tx.cursor < 0 || tx.cursor >= len(lista) {
		return turma.Exercicio{}, false
	}
	return lista[tx.cursor], true
}

func (tx *telaExercicios) atualizar(a *App, msg tea.KeyMsg) (tea.Cmd, bool) {
	if tx.campo != campoNenhum {
		return tx.teclaCampo(a, msg), true
	}

	lista := tx.lista(a)
	switch msg.String() {
	case "up", "k":
		tx.cursor = max(0, tx.cursor-1)
	case "down", "j":
		tx.cursor = min(len(lista)-1, tx.cursor+1)
	case "home", "g":
		tx.cursor = 0
	case "end", "G":
		tx.cursor = max(0, len(lista)-1)
	case "enter":
		e, ok := tx.atual(a)
		if !ok {
			return nil, true
		}
		a.exercicio = e.ID
		a.entregas.reiniciar()
		a.ir(idExercicio)
	case "T":
		tx.editar(a, campoTitulo)
	case "D":
		tx.editar(a, campoPrazo)
	case "P":
		tx.editar(a, campoPeso)
	case "V":
		tx.editar(a, campoVerificacao)
	case "I":
		tx.editar(a, campoImagem)
	case "K":
		// A categoria não fica em "C": essa tecla coleta todos os exercícios
		// em qualquer tela, e a lista de exercícios não pode ser a exceção.
		tx.editar(a, campoCategoria)
	case "n":
		tx.novo = &turma.Exercicio{Peso: 1, Situacao: turma.ExercicioAtivo}
		tx.campo, tx.buffer = campoRepo, ""
		a.avisar("novo exercício: informe o repositório-modelo")
	case "A":
		e, ok := tx.atual(a)
		if !ok {
			return nil, true
		}
		tx.alternarArquivo(a, e)
	case "z":
		tx.arquivados = !tx.arquivados
		tx.cursor = 0
	default:
		return nil, false
	}
	return nil, true
}

// editar abre o campo com o valor corrente carregado.
func (tx *telaExercicios) editar(a *App, campo campoExercicio) {
	e, ok := tx.atual(a)
	if !ok {
		a.erro = "nenhum exercício selecionado"
		return
	}
	tx.campo = campo
	switch campo {
	case campoTitulo:
		tx.buffer = e.Titulo
	case campoPrazo:
		tx.buffer = e.Prazo.String()
	case campoPeso:
		tx.buffer = strconv.FormatFloat(e.Peso, 'f', -1, 64)
	case campoVerificacao:
		tx.buffer = e.Verificacao
	case campoImagem:
		tx.buffer = e.Imagem
	case campoCategoria:
		tx.buffer = e.CategoriaDe()
	}
}

func (tx *telaExercicios) teclaCampo(a *App, msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		tx.campo, tx.buffer, tx.novo = campoNenhum, "", nil
		a.avisar("edição cancelada")
	case "enter":
		tx.aplicar(a)
	case "backspace":
		if tx.buffer != "" {
			tx.buffer = tx.buffer[:len(tx.buffer)-1]
		}
	case "space":
		tx.buffer += " "
	default:
		if t := msg.String(); len(t) == 1 {
			tx.buffer += t
		}
	}
	return nil
}

// aplicar grava o campo digitado, seja num exercício existente ou no novo.
func (tx *telaExercicios) aplicar(a *App) {
	valor := strings.TrimSpace(tx.buffer)

	if tx.novo != nil {
		tx.avancarNovo(a, valor)
		return
	}

	e, ok := a.turma.Exercicio(mustID(tx.atual(a)))
	if !ok {
		tx.campo, tx.buffer = campoNenhum, ""
		return
	}
	switch tx.campo {
	case campoTitulo:
		e.Titulo = valor
	case campoPrazo:
		d, err := turma.ParseData(valor)
		if err != nil {
			a.erro = err.Error()
			return
		}
		e.Prazo = d
		a.tarefas.registrar("prazo de %s virou %s; recolete para reclassificar", e.ID, d.String())
	case campoPeso:
		p, err := strconv.ParseFloat(strings.Replace(valor, ",", ".", 1), 64)
		if err != nil || p <= 0 {
			a.erro = "peso inválido: " + valor
			return
		}
		e.Peso = p
	case campoVerificacao:
		e.Verificacao = valor
	case campoImagem:
		e.Imagem = valor
	case campoCategoria:
		anterior := e.Categoria
		e.Categoria = valor
		if err := e.Validar(); err != nil {
			e.Categoria = anterior
			a.erro = err.Error()
			return
		}
	}

	tx.campo, tx.buffer = campoNenhum, ""
	// Gravar reordena os exercícios por prazo, e o ponteiro guarda a posição:
	// ler e.ID depois da gravação traria o exercício que tomou o lugar.
	editado := e.ID
	if a.gravar() {
		a.avisar("%s atualizado", editado)
	}
}

// avancarNovo percorre os campos obrigatórios do cadastro, um por vez.
func (tx *telaExercicios) avancarNovo(a *App, valor string) {
	switch tx.campo {
	case campoRepo:
		if valor == "" {
			a.erro = "o repositório-modelo é obrigatório"
			return
		}
		tx.novo.Repo = valor
		tx.novo.ID = idDoRepo(valor, a.turma.Config.Codigo)
		tx.campo, tx.buffer = campoPrazo, ""
		a.avisar("prazo do exercício, em AAAA-MM-DD")
	case campoPrazo:
		d, err := turma.ParseData(valor)
		if err != nil {
			a.erro = err.Error()
			return
		}
		tx.novo.Prazo = d
		tx.campo, tx.buffer = campoTitulo, ""
		a.avisar("título do exercício")
	case campoTitulo:
		tx.novo.Titulo = valor
		if _, existe := a.turma.Exercicio(tx.novo.ID); existe {
			a.erro = "já existe exercício com id " + tx.novo.ID
			tx.campo, tx.buffer, tx.novo = campoNenhum, "", nil
			return
		}
		if err := tx.novo.Validar(); err != nil {
			a.erro = err.Error()
			return
		}
		a.turma.RegistrarExercicio(*tx.novo)
		id := tx.novo.ID
		tx.campo, tx.buffer, tx.novo = campoNenhum, "", nil
		if a.gravar() {
			a.avisar("exercício %s cadastrado; use c para coletar", id)
		}
	}
}

func (tx *telaExercicios) alternarArquivo(a *App, e turma.Exercicio) {
	if e.Situacao == turma.ExercicioArquivado {
		p, _ := a.turma.Exercicio(e.ID)
		p.Situacao = turma.ExercicioAtivo
		if a.gravar() {
			a.avisar("%s voltou para o cronograma", e.ID)
		}
		return
	}
	a.pedirConfirmacao("arquivar "+e.ID+"? as entregas continuam gravadas", func() tea.Cmd {
		p, ok := a.turma.Exercicio(e.ID)
		if !ok {
			return nil
		}
		p.Situacao = turma.ExercicioArquivado
		if a.gravar() {
			a.avisar("%s arquivado", e.ID)
		}
		return nil
	})
}

func (tx *telaExercicios) atalhos() string {
	return "n novo · T título · D prazo · P peso · V suíte · I imagem · K categoria · A arquiva · z mostra arquivados"
}

func (tx *telaExercicios) desenhar(a *App) string {
	var b strings.Builder
	lista := tx.lista(a)

	if tx.campo != campoNenhum {
		b.WriteString(estDestaque.Render(tx.campo.String()+": ") + tx.buffer + "_\n\n")
	} else {
		b.WriteString(estFraco.Render(fmt.Sprintf("%d exercício(s)", len(lista))) + "\n\n")
	}
	if len(lista) == 0 {
		b.WriteString(estFraco.Render("  nenhum exercício; n cadastra o primeiro") + "\n")
		return b.String()
	}

	b.WriteString(estFraco.Render("  ID          PRAZO       PESO  CATEGORIA   SUÍTE                     TÍTULO") + "\n")

	altura := a.linhasDisponiveis() - 3
	if tx.cursor >= len(lista) {
		tx.cursor = len(lista) - 1
	}
	tx.topo = janela(tx.topo, tx.cursor, altura, len(lista))
	fim := min(tx.topo+altura, len(lista))

	for i := tx.topo; i < fim; i++ {
		e := lista[i]
		cursor := "  "
		id := e.ID
		if i == tx.cursor {
			cursor = estCursor.Render("> ")
			id = estCursor.Render(id)
		}
		suite := estFraco.Render("sem suíte")
		if e.TemSuite() {
			suite = truncar(e.Verificacao, 24)
		}
		situacao := ""
		if e.Situacao == turma.ExercicioArquivado {
			situacao = estFraco.Render(" (arquivado)")
		}
		b.WriteString(cursor + preencher(id, 12) + preencher(e.Prazo.String(), 12) +
			preencher(fmt.Sprintf("%g", e.Peso), 6) + preencher(truncar(e.CategoriaDe(), 10), 12) +
			preencher(suite, 26) + truncar(e.Titulo, 30) + situacao + "\n")
	}
	if len(lista) > altura {
		b.WriteString(rolagem(tx.topo, fim, len(lista)) + "\n")
	}
	return b.String()
}

// mustID devolve o id do exercício selecionado, ou vazio.
func mustID(e turma.Exercicio, ok bool) string {
	if !ok {
		return ""
	}
	return e.ID
}

// idDoRepo deriva o apelido curto do nome do repositório, do mesmo modo que o
// subcomando exercicios add.
func idDoRepo(repo, codigo string) string {
	id := strings.ToLower(strings.TrimSpace(repo))
	id = strings.TrimPrefix(id, strings.ToLower(codigo)+"-")
	id = strings.TrimSuffix(id, "-assignment")
	return id
}

// --- vínculos de equipe pela interface ---

// vincular registra que o aluno sob o cursor entregou no fork de outro.
func (a *App) vincular(integrante, dono turma.Aluno) {
	if integrante.GRR == dono.GRR {
		a.erro = "o integrante e o dono do fork são o mesmo aluno"
		return
	}
	if d := a.turma.Dono(a.exercicio, dono.GRR); d != dono.GRR {
		a.erro = dono.Nome + " já entrega no fork de outro aluno"
		return
	}
	a.turma.RegistrarVinculo(turma.Vinculo{
		Exercicio: a.exercicio, GRR: integrante.GRR, Dono: dono.GRR,
		Origem: turma.VinculoManual, AtualizadoEm: time.Now(),
	})
	if a.gravar() {
		a.avisar("%s entrega no fork de %s; recolete para atualizar a situação",
			integrante.Nome, dono.Nome)
		a.tarefas.registrar("vínculo manual em %s: %s no fork de %s",
			a.exercicio, integrante.GRR, dono.GRR)
	}
}

// desvincular desfaz o vínculo, com confirmação.
func (a *App) desvincular(exercicio string, v turma.Vinculo, nome string) {
	a.pedirConfirmacao("desfazer o vínculo de "+nome+" em "+exercicio+"?", func() tea.Cmd {
		if !a.turma.RemoverVinculo(exercicio, v.GRR) {
			return nil
		}
		if a.gravar() {
			a.avisar("vínculo desfeito; a próxima coleta pode recriá-lo se ele for membro do fork")
		}
		return nil
	})
}

// exportar grava o relatório em markdown e a planilha de notas.
func (a *App) exportar() tea.Cmd {
	md, xlsx, err := acoes.Exportar(a.turma, a.store.Pasta())
	if err != nil {
		a.erro = err.Error()
		return nil
	}
	a.avisar("gravados %s e %s", md, xlsx)
	a.tarefas.registrar("exportação: %s", md)
	a.tarefas.registrar("exportação: %s", xlsx)
	return nil
}
