package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alexkutzke/gitlab-classroom/internal/turma"
)

func turmaExemplo() *turma.Turma {
	momento := time.Date(2026, 8, 19, 21, 0, 0, 0, time.UTC)
	t := &turma.Turma{
		Config: turma.Config{
			Codigo: "DS122", Disciplina: "DESENVOLVIMENTO WEB I", Turma: "TADSN2A",
			Semestre: "2026-02", Turno: "n", NamespaceModelos: "ds122-alexkutzke",
			PadraoGrupo: "{codigo}-{ano}-{periodo}-{turno}-{grr}",
		},
		Alunos: []turma.Aluno{{
			GRR: "GRR20259001", Nome: "Ana Souza", Email: "ana@ufpr.br",
			Usuario: "grr20259001", Grupo: "ds122-2026-2-n-grr20259001",
			Situacao: turma.Ativo, SituacaoConta: turma.ContaOK, VerificadoEm: momento,
		}},
		Exercicios: []turma.Exercicio{{
			ID: "html", Repo: "ds122-html-assignment", Titulo: "HTML",
			Prazo: turma.NovaData(2026, time.September, 5), Peso: 1.5, Ordem: 2,
			Situacao: turma.ExercicioAtivo,
		}},
		Entregas: []turma.Entrega{{
			Exercicio: "html", GRR: "GRR20259001", Situacao: turma.Entregue,
			Projeto: "ds122-2026-2-n-grr20259001/ds122-html-assignment",
			Commit:  "abc123", DataCommit: momento, Commits: 4,
			UltimoCommit: "def456", DataUltimo: momento, AtrasoDias: 0, ColetadoEm: momento,
		}},
		Notas: []turma.Nota{{
			Exercicio: "html", GRR: "GRR20259001", Valor: 85,
			Comentario: "faltou o rodapé", CorrigidoEm: momento,
		}},
		Vinculos: []turma.Vinculo{{
			Exercicio: "html", GRR: "GRR20259002", Dono: "GRR20259001",
			Origem: turma.VinculoDescoberto, AtualizadoEm: momento,
		}},
	}
	t.Config.Padroes()
	return t
}

func TestGravarECarregarPreservaTudo(t *testing.T) {
	dir := t.TempDir()
	s, err := Criar(dir)
	if err != nil {
		t.Fatal(err)
	}
	original := turmaExemplo()
	if err := s.Gravar(original); err != nil {
		t.Fatal(err)
	}

	lida, err := s.Carregar()
	if err != nil {
		t.Fatal(err)
	}
	if len(lida.Alunos) != 1 || lida.Alunos[0].Grupo != original.Alunos[0].Grupo {
		t.Errorf("aluno não sobreviveu ao ciclo: %+v", lida.Alunos)
	}
	if len(lida.Exercicios) != 1 || lida.Exercicios[0].Peso != 1.5 {
		t.Errorf("exercício não sobreviveu ao ciclo: %+v", lida.Exercicios)
	}
	if lida.Exercicios[0].Ordem != 2 {
		t.Errorf("ordem não sobreviveu ao ciclo: %+v", lida.Exercicios[0])
	}
	if len(lida.Entregas) != 1 || lida.Entregas[0].Commits != 4 {
		t.Errorf("entrega não sobreviveu ao ciclo: %+v", lida.Entregas)
	}
	if len(lida.Notas) != 1 || lida.Notas[0].Valor != 85 {
		t.Errorf("nota não sobreviveu ao ciclo: %+v", lida.Notas)
	}
	if len(lida.Vinculos) != 1 || lida.Vinculos[0].Dono != "GRR20259001" {
		t.Errorf("vínculo de equipe não sobreviveu ao ciclo: %+v", lida.Vinculos)
	}
	if lida.Vinculos[0].Origem != turma.VinculoDescoberto {
		t.Errorf("origem do vínculo = %v", lida.Vinculos[0].Origem)
	}
	if !lida.Entregas[0].DataCommit.Equal(original.Entregas[0].DataCommit) {
		t.Errorf("data do commit mudou: %v", lida.Entregas[0].DataCommit)
	}
}

func TestLerExerciciosSemColunaOrdemUsaZero(t *testing.T) {
	dir := t.TempDir()
	s, err := Criar(dir)
	if err != nil {
		t.Fatal(err)
	}
	conteudo := "id;repo;titulo;prazo;peso;verificacao;imagem;situacao\n" +
		"html;ds122-html-assignment;HTML;2026-09-05;1;;;ativo\n" +
		"prepare;ds122-prepare-assignment;;2026-08-15;1;;;ativo\n"
	if err := os.WriteFile(s.caminho("exercicios.csv"), []byte(conteudo), 0o644); err != nil {
		t.Fatal(err)
	}
	exs, err := s.lerExercicios()
	if err != nil {
		t.Fatal(err)
	}
	if len(exs) != 2 {
		t.Fatalf("esperava 2 exercícios, veio %d", len(exs))
	}
	for _, e := range exs {
		if e.Ordem != 0 {
			t.Errorf("exercício %s: Ordem = %d, queria 0 (CSV sem a coluna)", e.ID, e.Ordem)
		}
	}
}

func TestGravarDuasVezesProduzOMesmoArquivo(t *testing.T) {
	dir := t.TempDir()
	s, err := Criar(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Gravar(turmaExemplo()); err != nil {
		t.Fatal(err)
	}
	primeiro, err := os.ReadFile(filepath.Join(s.Raiz, arqEntregas))
	if err != nil {
		t.Fatal(err)
	}
	lida, err := s.Carregar()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Gravar(lida); err != nil {
		t.Fatal(err)
	}
	segundo, err := os.ReadFile(filepath.Join(s.Raiz, arqEntregas))
	if err != nil {
		t.Fatal(err)
	}
	if string(primeiro) != string(segundo) {
		t.Errorf("gravação não é estável:\n%s\n---\n%s", primeiro, segundo)
	}
}

func TestColunasSaoLidasPeloNomeENaoPelaPosicao(t *testing.T) {
	dir := t.TempDir()
	s, err := Criar(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Colunas fora de ordem e uma coluna a menos, como fica depois de uma
	// edição à mão do arquivo.
	conteudo := "nome;grr;situacao\nAna Souza;GRR20259001;ativo\n"
	if err := os.WriteFile(filepath.Join(s.Raiz, arqAlunos), []byte(conteudo), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := s.GravarConfig(turmaExemplo().Config); err != nil {
		t.Fatal(err)
	}
	lida, err := s.Carregar()
	if err != nil {
		t.Fatal(err)
	}
	if len(lida.Alunos) != 1 || lida.Alunos[0].GRR != "GRR20259001" || lida.Alunos[0].Nome != "Ana Souza" {
		t.Errorf("leitura por nome de coluna falhou: %+v", lida.Alunos)
	}
}

func TestDescobrirSobeAArvore(t *testing.T) {
	dir := t.TempDir()
	if _, err := Criar(dir); err != nil {
		t.Fatal(err)
	}
	fundo := filepath.Join(dir, "a", "b")
	if err := os.MkdirAll(fundo, 0o755); err != nil {
		t.Fatal(err)
	}
	s, err := Descobrir(fundo)
	if err != nil {
		t.Fatal(err)
	}
	if s.Pasta() != dir {
		t.Errorf("Pasta = %q, queria %q", s.Pasta(), dir)
	}
}

func TestVinculoSemColunaDeOrigemEhTratadoComoManual(t *testing.T) {
	dir := t.TempDir()
	s, err := Criar(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.GravarConfig(turmaExemplo().Config); err != nil {
		t.Fatal(err)
	}
	// Linha escrita à mão, sem a coluna origem: tratá-la como manual é o
	// seguro, porque a coleta não apaga o que o professor afirmou.
	conteudo := "exercicio;grr;dono\nhtml;GRR20259002;GRR20259001\n"
	if err := os.WriteFile(filepath.Join(s.Raiz, arqEquipes), []byte(conteudo), 0o644); err != nil {
		t.Fatal(err)
	}

	lida, err := s.Carregar()
	if err != nil {
		t.Fatal(err)
	}
	if len(lida.Vinculos) != 1 || lida.Vinculos[0].Origem != turma.VinculoManual {
		t.Errorf("vínculo lido = %+v", lida.Vinculos)
	}
}
