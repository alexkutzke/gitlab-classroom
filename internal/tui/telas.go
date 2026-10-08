package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/alexkutzke/gitlab-classroom/internal/moldura"
	"github.com/alexkutzke/gitlab-classroom/internal/turma"
)

// erroMsg leva um erro de uma tarefa de volta ao laço de eventos.
type erroMsg struct{ err error }

// --- alunos ---

type telaAlunos struct {
	cursor int

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

func (ta *telaAlunos) digitando() bool { return ta.modoFiltro }

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
		ta.cursor = max(0, min(total-1, ta.cursor+1))
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

func (ta *telaAlunos) entrada(a *App) string {
	if !ta.modoFiltro {
		return ""
	}
	return estAcento.Render("filtro: ") + ta.buffer + "_   " +
		estTecla.Render("enter") + " " + estFraco.Render("aplica") + "   " +
		estTecla.Render("esc") + " " + estFraco.Render("limpa")
}

func (ta *telaAlunos) teclas(a *App) []moldura.Tecla {
	if a.focoAtual() == focoDetalhe {
		return append(teclasDoDetalhe(), a.teclasComuns()...)
	}
	pend := "só pendentes"
	if ta.soPendentes {
		pend = "todos"
	}
	r := []moldura.Tecla{{K: "tab", Rotulo: "painel"}, {K: "j/k", Rotulo: "move"},
		{K: "S", Rotulo: "sincroniza"}, {K: "P", Rotulo: pend}, {K: "/", Rotulo: "filtra"}}
	return append(r, a.teclasComuns()...)
}

// textoDaConta é o rótulo da situação da conta, o mesmo em lista e detalhe.
func textoDaConta(al turma.Aluno) (string, lipgloss.Style) {
	switch al.SituacaoConta {
	case "":
		return "· não verificada", estFraco
	case turma.ContaOK:
		return "✓ " + string(al.SituacaoConta), estOK
	}
	return "▲ " + string(al.SituacaoConta), estAtencao
}

func (ta *telaAlunos) painelLista(a *App) (string, moldura.Conteudo) {
	alunos := ta.lista(a)
	titulo := fmt.Sprintf("Alunos (%d de %d)", len(alunos), a.panorama.Ativos)
	if ta.soPendentes {
		titulo += " · só pendentes"
	}
	if ta.filtro != "" {
		titulo += fmt.Sprintf(" · filtro %q", ta.filtro)
	}
	if len(alunos) == 0 {
		return titulo, moldura.Conteudo{Sel: -1, Linhas: []string{estFraco.Render(" nenhum aluno")}}
	}
	cab := []moldura.Celula{moldura.Cel("grr", estFraco), moldura.Cel("nome", estFraco),
		moldura.Cel("conta", estFraco)}
	var linhas [][]moldura.Celula
	for _, al := range alunos {
		conta, est := textoDaConta(al)
		linhas = append(linhas, []moldura.Celula{moldura.Cel(al.GRR, estFraco),
			moldura.Cel(truncar(al.Nome, 32), estNormal), moldura.Cel(conta, est)})
	}
	ta.cursor = max(0, min(ta.cursor, len(alunos)-1))
	return titulo, moldura.Conteudo{Linhas: moldura.Tabela(cab, linhas), Sel: 1 + ta.cursor,
		Info: fmt.Sprintf("%d de %d", ta.cursor+1, len(alunos))}
}

// detalhe mostra a conta no GitLab e a entrega do aluno em cada exercício.
func (ta *telaAlunos) detalhe(a *App, largura int) (string, moldura.Conteudo) {
	alunos := ta.lista(a)
	if ta.cursor < 0 || ta.cursor >= len(alunos) {
		return "Detalhe", moldura.Conteudo{Sel: -1, Linhas: []string{estFraco.Render(" nada selecionado")}}
	}
	al := alunos[ta.cursor]
	d := novoDetalhe(largura)
	d.campo("GRR", al.GRR)
	d.campo("e-mail", al.Email)
	usuario := al.UsuarioEsperado()
	if al.Usuario == "" {
		usuario += estFraco.Render(" (pela convenção)")
	}
	d.campo("usuário", usuario)
	d.campo("grupo esperado", a.turma.Config.CaminhoGrupo(al.GRR))
	grupo := al.Grupo
	if grupo == "" {
		grupo = estFraco.Render("-")
	}
	d.campo("grupo achado", grupo)
	conta, est := textoDaConta(al)
	d.campo("conta", est.Render(conta))
	if !al.VerificadoEm.IsZero() {
		d.campo("verificada em", instante(al.VerificadoEm))
	}
	if al.Observacao != "" {
		d.campo("observação", al.Observacao)
	}

	d.secao("ENTREGAS")
	exs := a.turma.ExerciciosAtivos()
	if len(exs) == 0 {
		d.linha(estFraco.Render(" · nenhum exercício ativo"))
	}
	largID := 0
	for _, e := range exs {
		largID = max(largID, len([]rune(e.ID)))
	}
	for _, e := range exs {
		id := e.ID + strings.Repeat(" ", largID-len([]rune(e.ID)))
		en, ok := a.turma.Entrega(e.ID, al.GRR)
		sit := estFraco.Render("· sem coleta")
		if ok && en.Situacao != "" {
			sit = corDaSituacao(en.Situacao).Render(simboloDaSituacao(en.Situacao) + " " + en.Descricao())
		}
		nota := ""
		if n, ok := a.turma.Nota(e.ID, al.GRR); ok {
			nota = "  " + estAcento.Render(fmt.Sprintf("nota %g", n.Valor))
		}
		d.linha(" " + estFraco.Render(id) + "  " + sit + nota)
	}
	return al.Nome, d.conteudo()
}

// --- equipes ---

type telaEquipes struct {
	cursor int
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

func (tq *telaEquipes) digitando() bool { return false }

func (tq *telaEquipes) atualizar(a *App, msg tea.KeyMsg) (tea.Cmd, bool) {
	total := len(tq.lista(a))
	switch msg.String() {
	case "up", "k":
		tq.cursor = max(0, tq.cursor-1)
	case "down", "j":
		tq.cursor = max(0, min(total-1, tq.cursor+1))
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

func (tq *telaEquipes) teclas(a *App) []moldura.Tecla {
	if a.focoAtual() == focoDetalhe {
		return append(teclasDoDetalhe(), a.teclasComuns()...)
	}
	r := []moldura.Tecla{{K: "tab", Rotulo: "painel"}, {K: "j/k", Rotulo: "move"},
		{K: "d", Rotulo: "desfaz o vínculo"}, {K: "u", Rotulo: "procura membro sem cadastro"}}
	return append(r, a.teclasComuns()...)
}

func (tq *telaEquipes) painelLista(a *App) (string, moldura.Conteudo) {
	lista := tq.lista(a)
	titulo := fmt.Sprintf("Equipes (%d)", len(lista))
	if len(lista) == 0 {
		return titulo, moldura.Conteudo{Sel: -1,
			Linhas: []string{estFraco.Render(" nenhuma entrega compartilhada registrada")}}
	}
	cab := []moldura.Celula{moldura.Cel("exercício", estFraco), moldura.Cel("integrante", estFraco),
		moldura.Cel("entregou no fork de", estFraco), moldura.Cel("origem", estFraco)}
	var linhas [][]moldura.Celula
	for _, v := range lista {
		origem := moldura.Cel(string(v.Vinculo.Origem), estFraco)
		if v.Vinculo.Origem == turma.VinculoManual {
			origem = moldura.Cel("manual", estAcento)
		}
		linhas = append(linhas, []moldura.Celula{moldura.Cel(v.Vinculo.Exercicio, estNormal),
			moldura.Cel(truncar(v.Integrante, 30), estNormal), moldura.Cel(truncar(v.Dono, 30), estNormal),
			origem})
	}
	tq.cursor = max(0, min(tq.cursor, len(lista)-1))
	return titulo, moldura.Conteudo{Linhas: moldura.Tabela(cab, linhas), Sel: 1 + tq.cursor,
		Info: fmt.Sprintf("%d de %d", tq.cursor+1, len(lista))}
}

func (tq *telaEquipes) detalhe(a *App, largura int) (string, moldura.Conteudo) {
	lista := tq.lista(a)
	d := novoDetalhe(largura)
	if tq.cursor < 0 || tq.cursor >= len(lista) {
		d.texto("A coleta descobre sozinha quem é membro do fork de outro aluno. Para a dupla "+
			"que não adicionou o colega ao projeto, use `classroom equipes vincular`.", estFraco)
		d.linha("")
		d.texto("u procura membro de fork cujo login não está no cadastro, que é o motivo mais "+
			"comum de uma dupla passar despercebida.", estFraco)
		return "Equipes", d.conteudo()
	}
	v := lista[tq.cursor]
	d.campo("exercício", v.Vinculo.Exercicio)
	d.campo("dono do fork", v.Dono+estFraco.Render(" "+v.Vinculo.Dono))
	if en, ok := a.turma.Entrega(v.Vinculo.Exercicio, v.Vinculo.Dono); ok && en.Projeto != "" {
		d.campo("fork", en.Projeto)
	}
	origem := string(v.Vinculo.Origem)
	if v.Vinculo.Origem == turma.VinculoManual {
		origem = estAcento.Render("manual") + estFraco.Render(", a coleta não toca")
	} else {
		origem += estFraco.Render(", descoberto pelos membros do fork")
	}
	d.campo("origem", origem)
	if !v.Vinculo.AtualizadoEm.IsZero() {
		d.campo("atualizado em", instante(v.Vinculo.AtualizadoEm))
	}
	d.secao("INTEGRANTES")
	for _, grr := range a.turma.Equipe(v.Vinculo.Exercicio, v.Vinculo.Dono) {
		nome := grr
		if al, ok := a.turma.AlunoPorGRR(grr); ok {
			nome = al.Nome
		}
		papel := ""
		if grr == v.Vinculo.Dono {
			papel = estFraco.Render("  dono do fork")
		}
		d.linha(" " + estFraco.Render("○") + " " + nome + estFraco.Render(" "+grr) + papel)
	}
	return v.Integrante, d.conteudo()
}

// --- tarefas ---

// estadoRegistro é como uma entrada da tela de tarefas terminou.
type estadoRegistro int

const (
	regEmCurso estadoRegistro = iota
	regConcluido
	regErro
	regCancelado
	// regAnotacao é o que a sessão fez sem operação longa, como a correção
	// gravada ou o vínculo manual.
	regAnotacao
)

func (e estadoRegistro) String() string {
	switch e {
	case regEmCurso:
		return "em curso"
	case regErro:
		return "erro"
	case regCancelado:
		return "cancelada"
	case regAnotacao:
		return "feito"
	}
	return "concluída"
}

func (e estadoRegistro) simbolo() string {
	switch e {
	case regEmCurso:
		return estAtencao.Render("●")
	case regErro:
		return estErro.Render("✖")
	case regCancelado:
		return estAtencao.Render("▲")
	}
	return estOK.Render("✓")
}

// registro é uma entrada da tela de tarefas: uma operação longa, com o resumo
// e os detalhes dela, ou uma anotação avulsa.
type registro struct {
	nome     string
	estado   estadoRegistro
	operacao bool
	inicio   time.Time
	fim      time.Time
	linhas   []string
}

// telaTarefas guarda a saída das operações longas, que de outro modo sumiria
// junto com a barra de progresso.
type telaTarefas struct {
	// linhas é o histórico corrido da sessão, na ordem em que aconteceu.
	linhas    []string
	registros []*registro
	cursor    int
}

// registrar acrescenta uma anotação avulsa ao histórico da sessão.
func (tt *telaTarefas) registrar(formato string, args ...any) {
	texto := fmt.Sprintf(formato, args...)
	tt.linhas = append(tt.linhas, texto)
	agora := relogio()
	tt.registros = append(tt.registros, &registro{nome: texto, estado: regAnotacao,
		inicio: agora, fim: agora, linhas: []string{texto}})
}

// abrir cria a entrada de uma operação longa que começa agora.
func (tt *telaTarefas) abrir(nome string) *registro {
	r := &registro{nome: nome, estado: regEmCurso, operacao: true, inicio: relogio()}
	tt.registros = append(tt.registros, r)
	return r
}

// encerrar fecha a operação com o estado e a linha de resumo.
func (tt *telaTarefas) encerrar(r *registro, estado estadoRegistro, resumo string) {
	r.estado, r.fim = estado, relogio()
	r.linhas = append(r.linhas, resumo)
	tt.linhas = append(tt.linhas, r.nome+": "+resumo)
}

// acrescentar junta uma linha de detalhe à operação.
func (tt *telaTarefas) acrescentar(r *registro, linha string) {
	r.linhas = append(r.linhas, linha)
	tt.linhas = append(tt.linhas, "  "+linha)
}

// ultimaOperacao devolve a operação longa mais recente já encerrada.
func (tt *telaTarefas) ultimaOperacao() *registro {
	for i := len(tt.registros) - 1; i >= 0; i-- {
		if r := tt.registros[i]; r.operacao && r.estado != regEmCurso {
			return r
		}
	}
	return nil
}

// emOrdem devolve as entradas da mais recente para a mais antiga, que é a
// ordem da lista: o que acabou de acontecer fica no topo.
func (tt *telaTarefas) emOrdem() []*registro {
	out := make([]*registro, len(tt.registros))
	for i, r := range tt.registros {
		out[len(out)-1-i] = r
	}
	return out
}

func (tt *telaTarefas) digitando() bool { return false }

func (tt *telaTarefas) atualizar(a *App, msg tea.KeyMsg) (tea.Cmd, bool) {
	total := len(tt.registros)
	switch msg.String() {
	case "up", "k":
		tt.cursor = max(0, tt.cursor-1)
	case "down", "j":
		tt.cursor = max(0, min(total-1, tt.cursor+1))
	case "home", "g":
		tt.cursor = 0
	case "end", "G":
		tt.cursor = max(0, total-1)
	case "c":
		tt.linhas, tt.registros, tt.cursor = nil, nil, 0
		a.avisar("histórico limpo")
	default:
		return nil, false
	}
	return nil, true
}

func (tt *telaTarefas) teclas(a *App) []moldura.Tecla {
	if a.focoAtual() == focoDetalhe {
		return append(teclasDoDetalhe(), a.teclasComuns()...)
	}
	r := []moldura.Tecla{{K: "tab", Rotulo: "painel"}, {K: "j/k", Rotulo: "move"},
		{K: "c", Rotulo: "limpa o histórico"}}
	return append(r, a.teclasComuns()...)
}

func (tt *telaTarefas) painelLista(a *App) (string, moldura.Conteudo) {
	regs := tt.emOrdem()
	titulo := fmt.Sprintf("Tarefas (%d)", len(regs))
	if len(regs) == 0 {
		return titulo, moldura.Conteudo{Sel: -1,
			Linhas: []string{estFraco.Render(" nenhuma operação nesta sessão")}}
	}
	var linhas []string
	for _, r := range regs {
		linhas = append(linhas, " "+r.estado.simbolo()+" "+estFraco.Render(r.inicio.Format("15:04"))+
			"  "+moldura.Limpo(truncar(r.nome, 60)))
	}
	tt.cursor = max(0, min(tt.cursor, len(regs)-1))
	return titulo, moldura.Conteudo{Linhas: linhas, Sel: tt.cursor,
		Info: fmt.Sprintf("%d de %d", tt.cursor+1, len(regs))}
}

func (tt *telaTarefas) detalhe(a *App, largura int) (string, moldura.Conteudo) {
	regs := tt.emOrdem()
	if tt.cursor < 0 || tt.cursor >= len(regs) {
		return "Registro", moldura.Conteudo{Sel: -1, Linhas: []string{estFraco.Render(" nada selecionado")}}
	}
	r := regs[tt.cursor]
	d := novoDetalhe(largura)
	d.campo("situação", r.estado.simbolo()+" "+r.estado.String())
	d.campo("início", r.inicio.Format("15:04:05"))
	if !r.fim.IsZero() && r.operacao {
		d.campo("fim", r.fim.Format("15:04:05")+estFraco.Render(", "+r.fim.Sub(r.inicio).Round(time.Second).String()))
	}
	if a.emCurso() && a.tarefa.registro == r {
		d.campo("andamento", a.textoProgresso())
	}
	d.secao("REGISTRO")
	for _, l := range r.linhas {
		d.texto(l, estNormal)
	}
	return r.nome, d.conteudo()
}

// linhasDoRegistro é o que o painel da tarefa mostra na tela inicial.
func linhasDoRegistro(r *registro) []string {
	out := []string{" " + r.estado.simbolo() + " " + moldura.Limpo(r.nome)}
	for _, l := range r.linhas {
		out = append(out, "   "+moldura.Limpo(l))
	}
	return out
}
