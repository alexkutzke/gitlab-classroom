package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/alexkutzke/gitlab-classroom/internal/turma"
)

// erroMsg leva um erro de uma tarefa de volta ao laço de eventos.
type erroMsg struct{ err error }

// --- alunos ---

type telaAlunos struct {
	cursor int
	topo   int

	filtro     string
	modoFiltro bool
	buffer     string
	// soPendentes esconde quem já está com a conta em ordem, que é a maioria.
	soPendentes bool
}

func (ta *telaAlunos) lista(a *App) []turma.Aluno {
	alunos := turma.Busca(a.turma.Ativos(), ta.filtro)
	if !ta.soPendentes {
		return alunos
	}
	var out []turma.Aluno
	for _, al := range alunos {
		if al.SituacaoConta != turma.ContaOK {
			out = append(out, al)
		}
	}
	return out
}

func (ta *telaAlunos) atualizar(a *App, msg tea.KeyMsg) (tea.Cmd, bool) {
	if ta.modoFiltro {
		switch msg.String() {
		case "esc":
			ta.modoFiltro, ta.buffer, ta.filtro = false, "", ""
		case "enter":
			ta.filtro, ta.modoFiltro, ta.buffer = ta.buffer, false, ""
			ta.cursor = 0
		case "backspace":
			if ta.buffer != "" {
				ta.buffer = ta.buffer[:len(ta.buffer)-1]
			}
		case "space":
			ta.buffer += " "
		default:
			if t := msg.String(); len(t) == 1 {
				ta.buffer += t
			}
		}
		return nil, true
	}

	total := len(ta.lista(a))
	switch msg.String() {
	case "up", "k":
		ta.cursor = max(0, ta.cursor-1)
	case "down", "j":
		ta.cursor = min(total-1, ta.cursor+1)
	case "home", "g":
		ta.cursor = 0
	case "end", "G":
		ta.cursor = max(0, total-1)
	case "/":
		ta.modoFiltro, ta.buffer = true, ta.filtro
	case "P":
		ta.soPendentes = !ta.soPendentes
		ta.cursor = 0
		if ta.soPendentes {
			a.avisar("mostrando só quem tem pendência de cadastro")
		}
	default:
		return nil, false
	}
	return nil, true
}

func (ta *telaAlunos) atalhos() string { return "S sincroniza · P só pendentes · / filtra" }

func (ta *telaAlunos) desenhar(a *App) string {
	var b strings.Builder
	alunos := ta.lista(a)

	if ta.modoFiltro {
		b.WriteString(estDestaque.Render("filtro: ") + ta.buffer + "_\n\n")
	} else {
		b.WriteString(estFraco.Render(fmt.Sprintf("%d de %d aluno(s) ativo(s)",
			len(alunos), a.panorama.Ativos)) + "\n\n")
	}
	if len(alunos) == 0 {
		b.WriteString(estFraco.Render("  nenhum aluno") + "\n")
		return b.String()
	}

	altura := a.linhasDisponiveis() - 2
	if ta.cursor >= len(alunos) {
		ta.cursor = len(alunos) - 1
	}
	ta.topo = janela(ta.topo, ta.cursor, altura, len(alunos))
	fim := min(ta.topo+altura, len(alunos))

	for i := ta.topo; i < fim; i++ {
		al := alunos[i]
		cursor := "  "
		nome := truncar(al.Nome, 32)
		if i == ta.cursor {
			cursor = estCursor.Render("> ")
			nome = estCursor.Render(nome)
		}
		conta := estFraco.Render("não verificada")
		if al.SituacaoConta != "" {
			estilo := estAtencao
			if al.SituacaoConta == turma.ContaOK {
				estilo = estOK
			}
			conta = estilo.Render(string(al.SituacaoConta))
		}
		grupo := al.Grupo
		if grupo == "" {
			grupo = estFraco.Render("esperado " + a.turma.Config.CaminhoGrupo(al.GRR))
		}
		b.WriteString(cursor + preencher(al.GRR, 13) + preencher(nome, 33) +
			preencher(conta, 18) + truncar(grupo, max(10, a.largura-66)) + "\n")
	}
	if len(alunos) > altura {
		b.WriteString(rolagem(ta.topo, fim, len(alunos)) + "\n")
	}
	return b.String()
}

// --- equipes ---

type telaEquipes struct {
	cursor int
	topo   int
}

// vinculoNaTela junta o vínculo com os nomes, para a lista não consultar o
// cadastro a cada quadro.
type vinculoNaTela struct {
	Vinculo    turma.Vinculo
	Integrante string
	Dono       string
}

// lista monta as linhas da tela.
//
// A ordem precisa ser estável: o desenho e o tratamento de tecla chamam esta
// função separadamente, e duas ordens diferentes fariam o `d` desfazer o
// vínculo de outro aluno. VinculosDoExercicio devolve um map, e percorrer map
// em Go dá uma ordem nova a cada vez.
func (tq *telaEquipes) lista(a *App) []vinculoNaTela {
	nome := func(grr string) string {
		if al, ok := a.turma.AlunoPorGRR(grr); ok {
			return al.Nome
		}
		return grr
	}
	var out []vinculoNaTela
	for _, e := range a.turma.ExerciciosAtivos() {
		// A ordenação é por exercício, e não da lista toda, para os
		// exercícios ficarem na ordem do semestre.
		inicio := len(out)
		for _, v := range a.turma.VinculosDoExercicio(e.ID) {
			out = append(out, vinculoNaTela{
				Vinculo: v, Integrante: nome(v.GRR), Dono: nome(v.Dono),
			})
		}
		bloco := out[inicio:]
		sort.Slice(bloco, func(i, j int) bool {
			if bloco[i].Integrante != bloco[j].Integrante {
				return bloco[i].Integrante < bloco[j].Integrante
			}
			return bloco[i].Vinculo.GRR < bloco[j].Vinculo.GRR
		})
	}
	return out
}

func (tq *telaEquipes) atualizar(a *App, msg tea.KeyMsg) (tea.Cmd, bool) {
	total := len(tq.lista(a))
	switch msg.String() {
	case "up", "k":
		tq.cursor = max(0, tq.cursor-1)
	case "down", "j":
		tq.cursor = min(total-1, tq.cursor+1)
	case "home", "g":
		tq.cursor = 0
	case "end", "G":
		tq.cursor = max(0, total-1)
	case "d":
		lista := tq.lista(a)
		if tq.cursor < 0 || tq.cursor >= len(lista) {
			return nil, true
		}
		v := lista[tq.cursor]
		a.desvincular(v.Vinculo.Exercicio, v.Vinculo, v.Integrante)
	case "u":
		return a.desconhecidos(), true
	default:
		return nil, false
	}
	return nil, true
}

func (tq *telaEquipes) atalhos() string {
	return "d desfaz o vínculo · u procura membro sem cadastro"
}

func (tq *telaEquipes) desenhar(a *App) string {
	var b strings.Builder
	lista := tq.lista(a)
	if len(lista) == 0 {
		b.WriteString(estFraco.Render("  nenhuma entrega compartilhada registrada") + "\n\n")
		b.WriteString(estFraco.Render(
			"  A coleta descobre sozinha quem é membro do fork de outro aluno.\n"+
				"  Para a dupla que não adicionou o colega ao projeto, use\n"+
				"  `classroom equipes vincular`.\n\n"+
				"  u procura membro de fork cujo login não está no cadastro,\n"+
				"  que é o motivo mais comum de uma dupla passar despercebida.") + "\n")
		return b.String()
	}

	b.WriteString(estFraco.Render("EXERCÍCIO   INTEGRANTE                       ENTREGOU NO FORK DE              ORIGEM") + "\n")

	altura := a.linhasDisponiveis() - 2
	if tq.cursor >= len(lista) {
		tq.cursor = len(lista) - 1
	}
	tq.topo = janela(tq.topo, tq.cursor, altura, len(lista))
	fim := min(tq.topo+altura, len(lista))

	for i := tq.topo; i < fim; i++ {
		v := lista[i]
		cursor := "  "
		if i == tq.cursor {
			cursor = estCursor.Render("> ")
		}
		origem := estFraco.Render(string(v.Vinculo.Origem))
		if v.Vinculo.Origem == turma.VinculoManual {
			origem = estDestaque.Render("manual")
		}
		b.WriteString(cursor + preencher(v.Vinculo.Exercicio, 10) +
			preencher(truncar(v.Integrante, 32), 33) +
			preencher(truncar(v.Dono, 32), 33) + origem + "\n")
	}
	if len(lista) > altura {
		b.WriteString(rolagem(tq.topo, fim, len(lista)) + "\n")
	}
	return b.String()
}

// --- tarefas ---

// telaTarefas guarda a saída das operações longas, que de outro modo sumiria
// junto com a barra de progresso.
type telaTarefas struct {
	linhas []string
	topo   int
}

// registrar acrescenta uma linha ao histórico da sessão.
func (tt *telaTarefas) registrar(formato string, args ...any) {
	tt.linhas = append(tt.linhas, fmt.Sprintf(formato, args...))
}

func (tt *telaTarefas) atualizar(a *App, msg tea.KeyMsg) (tea.Cmd, bool) {
	altura := a.linhasDisponiveis()
	switch msg.String() {
	case "up", "k":
		tt.topo = max(0, tt.topo-1)
	case "down", "j":
		tt.topo = min(max(0, len(tt.linhas)-altura), tt.topo+1)
	case "home", "g":
		tt.topo = 0
	case "end", "G":
		tt.topo = max(0, len(tt.linhas)-altura)
	case "c":
		tt.linhas = nil
		tt.topo = 0
		a.avisar("histórico limpo")
	default:
		return nil, false
	}
	return nil, true
}

func (tt *telaTarefas) desenhar(a *App) string {
	if len(tt.linhas) == 0 {
		return estFraco.Render("  nenhuma operação nesta sessão")
	}
	var b strings.Builder
	altura := a.linhasDisponiveis()
	fim := min(tt.topo+altura, len(tt.linhas))
	for i := tt.topo; i < fim; i++ {
		b.WriteString("  " + truncar(tt.linhas[i], a.largura-4) + "\n")
	}
	return b.String()
}

// --- ajuda ---

type ajuda struct{}

func (aj *ajuda) atualizar(a *App, msg tea.KeyMsg) (tea.Cmd, bool) { return nil, false }

func (aj *ajuda) desenhar(a *App) string {
	secoes := []struct {
		titulo  string
		atalhos [][2]string
	}{
		{"Navegação", [][2]string{
			{"j k, setas", "move o cursor"},
			{"g G", "primeiro e último"},
			{"pgup pgdown", "página"},
			{"enter", "abre o item"},
			{"esc q", "volta, e sai no painel"},
			{"r", "recarrega os arquivos do disco"},
		}},
		{"Ações", [][2]string{
			{"C", "coleta todos os exercícios ativos"},
			{"S", "sincroniza o cadastro e as contas"},
			{"c", "coleta o exercício sob o cursor, ou o aberto"},
			{"l", "clona os forks do exercício aberto"},
			{"v", "roda a suíte do exercício aberto"},
			{"esc", "cancela a operação em curso"},
		}},
		{"Telas", [][2]string{
			{"p", "painel"},
			{"a", "alunos"},
			{"e", "equipes"},
			{"t", "tarefas"},
			{"?", "esta ajuda"},
		}},
		{"Exercício", [][2]string{
			{"n N", "corrige, e só quem ainda não tem nota"},
			{"V X", "vincula e desvincula entrega em dupla"},
			{"o", "abre o clone do aluno no $EDITOR"},
			{"w", "abre o projeto no GitLab"},
			{"s", "alterna a ordem: nome, situação, nota"},
			{"/", "filtra por nome ou GRR"},
		}},
		{"Alunos", [][2]string{
			{"P", "mostra só quem tem pendência de cadastro"},
		}},
		{"Equipes", [][2]string{
			{"d", "desfaz o vínculo sob o cursor"},
			{"u", "procura membro de fork sem cadastro"},
		}},
		{"Exercícios (x)", [][2]string{
			{"n", "cadastra um exercício"},
			{"T D P V I", "edita título, prazo, peso, suíte e imagem"},
			{"K", "edita a categoria: exercicio, trabalho"},
			{"A z", "arquiva, e mostra os arquivados"},
		}},
		{"Painel", [][2]string{
			{"E", "exporta o relatório e a planilha de notas"},
		}},
	}

	var b strings.Builder
	for _, s := range secoes {
		b.WriteString(estTitulo.Render(s.titulo) + "\n")
		for _, at := range s.atalhos {
			b.WriteString("  " + preencher(estDestaque.Render(at[0]), 16) + estFraco.Render(at[1]) + "\n")
		}
		b.WriteString("\n")
	}
	b.WriteString(estFraco.Render(
		"A linha de comando continua valendo para tudo, e é o caminho para scripts:\n"+
			"classroom --help lista os subcomandos.") + "\n")
	return b.String()
}
