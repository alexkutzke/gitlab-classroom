package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/alexkutzke/gitlab-classroom/internal/turma"
)

func TestCorrecaoEmbutidaGravaAsNotas(t *testing.T) {
	a := appExemplo(t)
	teclar(a, "enter") // abre o prepare
	teclar(a, "n")     // abre a correção

	if a.tela != idCorrecao || a.correcao == nil {
		t.Fatalf("tela = %v, correção = %v", a.tela, a.correcao)
	}

	// A primeira linha é a Ana, que já tem 95; troca para 80 e grava.
	teclar(a, "8", "0", "enter")
	teclar(a, "enter") // enter fora do modo nota fecha gravando

	if a.tela != idPainel && a.tela != idExercicio {
		t.Errorf("depois de gravar, a correção deveria devolver o controle: tela = %v", a.tela)
	}
	if a.correcao != nil {
		t.Error("a sessão de correção deveria ter sido encerrada")
	}

	n, ok := a.turma.Nota("prepare", "GRR20259001")
	if !ok || n.Valor != 80 {
		t.Fatalf("nota = %+v, queria 80", n)
	}
	relida, err := a.store.Carregar()
	if err != nil {
		t.Fatal(err)
	}
	if n, ok := relida.Nota("prepare", "GRR20259001"); !ok || n.Valor != 80 {
		t.Errorf("a nota não foi gravada no disco: %+v", n)
	}
}

func TestCorrecaoFechadaComQNaoGrava(t *testing.T) {
	a := appExemplo(t)
	teclar(a, "enter", "n")
	teclar(a, "5", "0", "enter") // lança
	teclar(a, "q")               // sai sem gravar

	if n, _ := a.turma.Nota("prepare", "GRR20259001"); n != nil && n.Valor == 50 {
		t.Error("sair com q não pode gravar a nota digitada")
	}
	if !strings.Contains(a.status, "sem gravar") {
		t.Errorf("status = %q", a.status)
	}
}

func TestVincularEntregaEmDuplaPelaTela(t *testing.T) {
	a := appExemplo(t)
	teclar(a, "enter") // prepare
	// Cursor na Ana; marca ela como integrante e escolhe o Bruno como dono.
	teclar(a, "V")
	if a.entregas.vinculando == nil {
		t.Fatal("V deveria entrar no modo de vínculo")
	}
	teclar(a, "j", "enter")

	if got := a.turma.Dono("prepare", "GRR20259001"); got != "GRR20259002" {
		t.Errorf("dono = %q, queria o Bruno", got)
	}
	v, ok := a.turma.VinculosDoExercicio("prepare")["GRR20259001"]
	if !ok || v.Origem != turma.VinculoManual {
		t.Errorf("vínculo = %+v, queria manual", v)
	}
	if a.entregas.vinculando != nil {
		t.Error("o modo de vínculo deveria terminar depois da escolha")
	}
}

func TestEscCancelaOModoDeVinculo(t *testing.T) {
	a := appExemplo(t)
	teclar(a, "enter", "V", "esc")

	if a.entregas.vinculando != nil {
		t.Error("esc deveria sair do modo de vínculo")
	}
	if len(a.turma.Vinculos) != 0 {
		t.Errorf("nenhum vínculo deveria ter sido criado: %+v", a.turma.Vinculos)
	}
}

func TestDesvincularPedeConfirmacao(t *testing.T) {
	a := appExemplo(t)
	a.turma.RegistrarVinculo(turma.Vinculo{
		Exercicio: "prepare", GRR: "GRR20259002", Dono: "GRR20259001",
		Origem: turma.VinculoManual,
	})

	teclar(a, "e") // tela de equipes
	teclar(a, "d")

	if a.confirmar == nil {
		t.Fatal("desvincular deveria pedir confirmação")
	}
	teclar(a, "n") // recusa
	if len(a.turma.Vinculos) != 1 {
		t.Error("recusar a confirmação não podia apagar o vínculo")
	}

	teclar(a, "d", "s") // pede de novo e confirma
	if len(a.turma.Vinculos) != 0 {
		t.Errorf("o vínculo deveria ter sido desfeito: %+v", a.turma.Vinculos)
	}
}

func TestCadastrarExercicioPelaTela(t *testing.T) {
	a := appExemplo(t)
	teclar(a, "x") // tela de exercícios
	teclar(a, "n") // novo

	for _, r := range "ds122-js-assignment" {
		teclar(a, string(r))
	}
	teclar(a, "enter")
	for _, r := range "2026-10-09" {
		teclar(a, string(r))
	}
	teclar(a, "enter")
	for _, r := range "JavaScript" {
		teclar(a, string(r))
	}
	teclar(a, "enter")

	e, ok := a.turma.Exercicio("js")
	if !ok {
		t.Fatalf("exercício não cadastrado; erro: %q", a.erro)
	}
	if e.Repo != "ds122-js-assignment" || e.Titulo != "JavaScript" {
		t.Errorf("exercício = %+v", e)
	}
	if e.Prazo.String() != "2026-10-09" {
		t.Errorf("prazo = %q", e.Prazo.String())
	}
}

func TestPrazoInvalidoNaoEhAceito(t *testing.T) {
	a := appExemplo(t)
	teclar(a, "x", "D") // edita o prazo do primeiro exercício
	for _, r := range "09/10/2026" {
		teclar(a, string(r))
	}
	teclar(a, "enter")

	if a.erro == "" {
		t.Error("data em outro formato deveria ser recusada com aviso")
	}
	e, _ := a.turma.Exercicio("prepare")
	if e.Prazo.String() != "2026-08-15" {
		t.Errorf("o prazo não podia mudar: %q", e.Prazo.String())
	}
}

func TestArquivarExercicioPedeConfirmacao(t *testing.T) {
	a := appExemplo(t)
	teclar(a, "x", "A")

	if a.confirmar == nil {
		t.Fatal("arquivar deveria pedir confirmação")
	}
	teclar(a, "s")

	e, _ := a.turma.Exercicio("prepare")
	if e.Situacao != turma.ExercicioArquivado {
		t.Errorf("situação = %v, queria arquivado", e.Situacao)
	}
	// Some da lista corrente, mas continua no arquivo.
	if len(a.turma.ExerciciosAtivos()) != 1 {
		t.Errorf("ativos = %d, queria 1", len(a.turma.ExerciciosAtivos()))
	}
	if len(a.turma.Exercicios) != 2 {
		t.Error("arquivar não pode apagar o exercício")
	}
}

func TestExportarGravaOsDoisArquivos(t *testing.T) {
	a := appExemplo(t)
	teclar(a, "E")

	if a.erro != "" {
		t.Fatalf("erro: %s", a.erro)
	}
	if !strings.Contains(a.status, "entregas.md") || !strings.Contains(a.status, ".xlsx") {
		t.Errorf("status = %q, queria os dois caminhos", a.status)
	}
	if len(a.tarefas.linhas) != 2 {
		t.Errorf("a exportação deveria ficar registrada: %v", a.tarefas.linhas)
	}
}

func TestTeclaDesconhecidaNaoQuebra(t *testing.T) {
	a := appExemplo(t)
	for _, tecla := range []string{"z", "Z", "1", "!", "ç"} {
		a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tecla)})
	}
	if a.View() == "" {
		t.Error("a interface deveria continuar desenhando")
	}
}
