// Package acoes reúne as operações que mexem no estado da turma: sincronizar
// o cadastro, coletar as entregas, clonar os forks, verificar e corrigir.
//
// Existe para que a interface de terminal e a linha de comando executem o
// mesmo código. Cada função aplica o resultado na turma e devolve um resumo do
// que aconteceu; gravar fica com quem chamou, que é o que torna o --dry-run
// trivial.
package acoes

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alexkutzke/gitlab-classroom/internal/coleta"
	"github.com/alexkutzke/gitlab-classroom/internal/correcao"
	"github.com/alexkutzke/gitlab-classroom/internal/diario"
	"github.com/alexkutzke/gitlab-classroom/internal/export"
	gl "github.com/alexkutzke/gitlab-classroom/internal/gitlab"
	"github.com/alexkutzke/gitlab-classroom/internal/repo"
	"github.com/alexkutzke/gitlab-classroom/internal/turma"
	"github.com/alexkutzke/gitlab-classroom/internal/verificacao"
)

// Progresso descreve o andamento de uma operação longa.
type Progresso struct {
	Feito int
	Total int
	// Rotulo é o item corrente, para a barra de progresso ter o que mostrar.
	Rotulo string
}

// AvisoProgresso recebe o andamento. Pode ser nil.
type AvisoProgresso func(Progresso)

func (p AvisoProgresso) avisar(feito, total int, rotulo string) {
	if p != nil {
		p(Progresso{Feito: feito, Total: total, Rotulo: rotulo})
	}
}

// --- exercícios ---

// EscolherExercicios resolve os ids informados, ou devolve todos os ativos.
func EscolherExercicios(t *turma.Turma, ids []string) ([]turma.Exercicio, error) {
	if len(ids) == 0 {
		es := t.ExerciciosAtivos()
		if len(es) == 0 {
			return nil, fmt.Errorf("nenhum exercício cadastrado: use `classroom exercicios add`")
		}
		return es, nil
	}
	var out []turma.Exercicio
	for _, id := range ids {
		e, ok := t.Exercicio(id)
		if !ok {
			return nil, fmt.Errorf("exercício %q não encontrado", id)
		}
		out = append(out, *e)
	}
	return out, nil
}

// --- entregas em dupla ---

// DonoDaEntrega devolve o aluno cujo clone serve a este aluno no exercício.
//
// Na entrega em dupla há um fork só, então há um clone só: o do dono. O
// colega aponta para a mesma pasta em vez de ganhar uma cópia.
func DonoDaEntrega(t *turma.Turma, exercicio string, a turma.Aluno) turma.Aluno {
	grr := t.Dono(exercicio, a.GRR)
	if grr == a.GRR {
		return a
	}
	if dono, ok := t.AlunoPorGRR(grr); ok {
		return *dono
	}
	return a
}

// DirDaEntrega é a pasta do clone que atende este aluno no exercício.
func DirDaEntrega(t *turma.Turma, pastaTurma, exercicio string, a turma.Aluno) string {
	return repo.Caminho(pastaTurma, t.Config.PastaEntregas, exercicio,
		DonoDaEntrega(t, exercicio, a))
}

// NomesDaEquipe devolve os nomes dos integrantes de uma entrega, menos o
// aluno informado em exceto.
func NomesDaEquipe(t *turma.Turma, equipe []string, exceto string) []string {
	var out []string
	for _, grr := range equipe {
		if grr == exceto {
			continue
		}
		if a, ok := t.AlunoPorGRR(grr); ok {
			out = append(out, a.Nome)
			continue
		}
		out = append(out, grr)
	}
	return out
}

// --- cadastro ---

// OpcoesSync controla a atualização do cadastro.
type OpcoesSync struct {
	// PastaDiario é relativa à pasta da turma; vazia usa a da configuração.
	PastaDiario string
	SemDiario   bool
	SemGitLab   bool
}

// ResumoSync conta o que a sincronização mudou.
type ResumoSync struct {
	Importacao diario.Resultado
	// SemCadastro traz o aviso quando não há .diario/ para importar.
	SemCadastro string
	Alunos      []turma.Aluno
	Contas      map[turma.SituacaoConta]int
}

// Sincronizar reimporta o cadastro do diario e reconcilia as contas com o
// GitLab.
func Sincronizar(ctx context.Context, t *turma.Turma, pastaTurma string, cli gl.Cliente, o OpcoesSync, prog AvisoProgresso) (ResumoSync, error) {
	var res ResumoSync

	if !o.SemDiario {
		pasta := o.PastaDiario
		if pasta == "" {
			pasta = t.Config.PastaDiario
		}
		alunos, err := diario.Ler(filepath.Join(pastaTurma, pasta))
		switch e := err.(type) {
		case nil:
			res.Importacao = diario.Importar(t, alunos)
		case *diario.ErrSemCadastro:
			res.SemCadastro = e.Error()
		default:
			return res, err
		}
	}

	if o.SemGitLab {
		return res, nil
	}
	if cli == nil {
		return res, fmt.Errorf("sem conexão com o GitLab")
	}

	ativos := t.Ativos()
	col := &coleta.Coletor{
		Cliente: cli,
		Config:  t.Config,
		Progresso: func(feito, total int, a turma.Aluno) {
			prog.avisar(feito, total, a.Nome)
		},
	}
	atualizados, err := col.Reconciliar(ctx, ativos)
	if err != nil {
		return res, err
	}
	AplicarAlunos(t, atualizados)

	res.Alunos = atualizados
	res.Contas = map[turma.SituacaoConta]int{}
	for _, a := range atualizados {
		res.Contas[a.SituacaoConta]++
	}
	return res, nil
}

// AplicarAlunos leva de volta ao cadastro o que a reconciliação apurou.
//
// Nome, e-mail e situação não entram: vêm do SIGA e seriam sobrescritos por
// uma cópia velha.
func AplicarAlunos(t *turma.Turma, atualizados []turma.Aluno) {
	for _, a := range atualizados {
		if p, ok := t.AlunoPorGRR(a.GRR); ok {
			p.Grupo, p.SituacaoConta, p.VerificadoEm = a.Grupo, a.SituacaoConta, a.VerificadoEm
			if a.Usuario != "" {
				p.Usuario = a.Usuario
			}
		}
	}
}

// --- coleta ---

// ResumoColeta descreve o que a coleta encontrou.
type ResumoColeta struct {
	// Situacoes conta as entregas por situação, em cada exercício.
	Situacoes map[string]map[turma.SituacaoEntrega]int
	// Compartilhadas conta as entregas em dupla por exercício.
	Compartilhadas map[string]int
}

// Coletar percorre o GitLab e aplica na turma as entregas e os vínculos de
// equipe de cada exercício informado.
func Coletar(ctx context.Context, t *turma.Turma, cli gl.Cliente, exercicios []turma.Exercicio, prog AvisoProgresso) (ResumoColeta, error) {
	res := ResumoColeta{
		Situacoes:      map[string]map[turma.SituacaoEntrega]int{},
		Compartilhadas: map[string]int{},
	}
	if cli == nil {
		return res, fmt.Errorf("sem conexão com o GitLab")
	}
	alunos := t.Ativos()
	if len(alunos) == 0 {
		return res, fmt.Errorf("nenhum aluno ativo: rode `classroom sync` para importar o cadastro")
	}

	col := &coleta.Coletor{
		Cliente: cli,
		Config:  t.Config,
		Progresso: func(feito, total int, a turma.Aluno) {
			prog.avisar(feito, total, a.Nome)
		},
	}
	saida, err := col.Coletar(ctx, alunos, exercicios)
	if err != nil {
		return res, err
	}

	AplicarAlunos(t, saida.Alunos)
	for _, e := range exercicios {
		var entregas []turma.Entrega
		for _, en := range saida.Entregas {
			if en.Exercicio == e.ID {
				entregas = append(entregas, en)
			}
		}
		t.SubstituirEntregas(e.ID, entregas)

		var vinculos []turma.Vinculo
		for _, v := range saida.Vinculos {
			if v.Exercicio == e.ID {
				vinculos = append(vinculos, v)
			}
		}
		t.SubstituirVinculosDescobertos(e.ID, vinculos)

		contagem := map[turma.SituacaoEntrega]int{}
		for _, en := range entregas {
			contagem[en.Situacao]++
		}
		res.Situacoes[e.ID] = contagem
		res.Compartilhadas[e.ID] = len(t.VinculosDoExercicio(e.ID))
	}
	return res, nil
}

// --- clone ---

// OpcoesClone controla o que baixar.
type OpcoesClone struct {
	// SoEntregues ignora quem não entregou no prazo.
	SoEntregues bool
}

// ResumoClone conta o que aconteceu com os clones.
type ResumoClone struct {
	Novos       int
	Atualizados int
	Falhas      []repo.Resultado
	// Base é a pasta onde os clones do primeiro exercício ficaram.
	Base string
}

// Clonar baixa, ou atualiza, o fork de cada entrega e posiciona o clone no
// commit avaliado.
func Clonar(ctx context.Context, t *turma.Turma, pastaTurma string, exercicios []turma.Exercicio, o OpcoesClone, prog AvisoProgresso) (ResumoClone, error) {
	var res ResumoClone

	alvos := AlvosDeClone(t, pastaTurma, exercicios, o)
	if len(alvos) == 0 {
		return res, fmt.Errorf("nenhum fork a clonar: rode `classroom coletar` antes")
	}

	saida := repo.Sincronizar(ctx, alvos, t.Config.Host, t.Config.Paralelismo,
		func(feito, total int, a repo.Alvo) {
			prog.avisar(feito, total, a.Nome)
		})

	for _, r := range saida {
		switch {
		case r.Erro != nil:
			res.Falhas = append(res.Falhas, r)
		case r.Novo:
			res.Novos++
		default:
			res.Atualizados++
		}
	}
	if len(exercicios) > 0 {
		res.Base = repo.Base(pastaTurma, t.Config.PastaEntregas, exercicios[0].ID)
	}
	return res, nil
}

// AlvosDeClone monta a lista de repositórios a baixar, um por entrega.
//
// A entrega em dupla tem um fork só: clonar duas vezes na mesma pasta daria
// conflito, então o integrante que não é dono não gera alvo.
func AlvosDeClone(t *turma.Turma, pastaTurma string, exercicios []turma.Exercicio, o OpcoesClone) []repo.Alvo {
	var alvos []repo.Alvo
	vistos := map[string]bool{}

	for _, e := range exercicios {
		entregas := t.EntregasDoExercicio(e.ID)
		for _, a := range t.Ativos() {
			en, ok := entregas[a.GRR]
			if !ok || en.Projeto == "" || !strings.Contains(en.Projeto, "/") {
				continue // sem fork não há o que clonar
			}
			if o.SoEntregues && en.Situacao != turma.Entregue {
				continue
			}
			dono := DonoDaEntrega(t, e.ID, a)
			dir := repo.Caminho(pastaTurma, t.Config.PastaEntregas, e.ID, dono)
			if vistos[dir] {
				continue
			}
			vistos[dir] = true
			alvos = append(alvos, repo.Alvo{
				Exercicio: e.ID, GRR: dono.GRR, Nome: dono.Nome,
				Projeto: en.Projeto, Commit: en.Commit, Dir: dir,
			})
		}
	}
	return alvos
}

// --- verificação ---

// OpcoesVerificacao controla a execução da suíte.
type OpcoesVerificacao struct {
	// GRR restringe a um aluno só; vazio verifica a turma.
	GRR         string
	Imagem      string
	Runtime     string
	TempoLimite time.Duration
	SemSandbox  bool
	Escrita     bool
}

// ResumoVerificacao conta os vereditos, já por aluno.
type ResumoVerificacao struct {
	Contagem  map[turma.SituacaoVerificacao]int
	Problemas []turma.Verificacao
	PastaLogs string
}

// Verificar roda a suíte do exercício sobre os clones e aplica o resultado na
// turma.
//
// Roda uma vez por fork: a entrega em dupla tem um repositório só, e o
// veredito é gravado para cada integrante, de modo que o relatório continua
// tendo uma linha por aluno.
func Verificar(ctx context.Context, t *turma.Turma, pastaTurma string, e turma.Exercicio, o OpcoesVerificacao, prog AvisoProgresso) (ResumoVerificacao, error) {
	res := ResumoVerificacao{Contagem: map[turma.SituacaoVerificacao]int{}}
	if !e.TemSuite() {
		return res, fmt.Errorf(
			"exercício %s não tem suíte; cadastre com `classroom exercicios editar --id %s --verificacao <comando>`",
			e.ID, e.ID)
	}

	entregas := t.EntregasDoExercicio(e.ID)
	var alvos []verificacao.Alvo
	porDono := map[string][]string{}
	for _, a := range t.Ativos() {
		if o.GRR != "" && !strings.EqualFold(a.GRR, o.GRR) {
			continue
		}
		dono := DonoDaEntrega(t, e.ID, a)
		if _, visto := porDono[dono.GRR]; !visto {
			alvos = append(alvos, verificacao.Alvo{
				GRR:    dono.GRR,
				Nome:   dono.Nome,
				Dir:    repo.Caminho(pastaTurma, t.Config.PastaEntregas, e.ID, dono),
				Commit: entregas[dono.GRR].Commit,
			})
		}
		porDono[dono.GRR] = append(porDono[dono.GRR], a.GRR)
	}
	if len(alvos) == 0 {
		return res, fmt.Errorf("nenhum aluno a verificar")
	}

	opts := verificacao.Opcoes{
		Exercicio:   e,
		Imagem:      o.Imagem,
		Runtime:     o.Runtime,
		TempoLimite: o.TempoLimite,
		SemSandbox:  o.SemSandbox,
		Escrita:     o.Escrita,
		PastaLogs:   filepath.Join(repo.Base(pastaTurma, t.Config.PastaEntregas, e.ID), ".logs"),
	}
	if opts.TempoLimite <= 0 {
		opts.TempoLimite = time.Duration(t.Config.TempoLimiteVerificacao) * time.Second
	}
	if opts.Imagem == "" && e.Imagem == "" {
		opts.Imagem = t.Config.ImagemVerificacao
	}
	res.PastaLogs = opts.PastaLogs

	saida, err := verificacao.Executar(ctx, alvos, opts, t.Config.Paralelismo,
		func(feito, total int, a verificacao.Alvo) {
			prog.avisar(feito, total, a.Nome)
		})
	if err != nil {
		return res, err
	}

	for _, v := range saida {
		if v.Situacao == turma.Reprovado || v.Situacao == turma.ErroVerificacao {
			res.Problemas = append(res.Problemas, v)
		}
		for _, integrante := range porDono[v.GRR] {
			res.Contagem[v.Situacao]++
			copia := v
			copia.GRR = integrante
			t.RegistrarVerificacao(copia)
		}
	}
	return res, nil
}

// --- correção ---

// FiltroCorrecao restringe quem entra na tela de correção.
type FiltroCorrecao struct {
	SoEntregues bool
	SemNota     bool
}

// ItensDeCorrecao monta a lista da tela de correção de um exercício.
func ItensDeCorrecao(t *turma.Turma, pastaTurma string, e turma.Exercicio, f FiltroCorrecao) []correcao.Item {
	entregas := t.EntregasDoExercicio(e.ID)
	verificacoes := t.VerificacoesDoExercicio(e.ID)

	var itens []correcao.Item
	for _, a := range t.Ativos() {
		en := entregas[a.GRR]
		if f.SoEntregues && en.Situacao != turma.Entregue {
			continue
		}
		item := correcao.Item{
			Aluno:       a,
			Entrega:     en,
			Verificacao: verificacoes[a.GRR],
			Dir:         DirDaEntrega(t, pastaTurma, e.ID, a),
		}
		if n, ok := t.Nota(e.ID, a.GRR); ok {
			if f.SemNota {
				continue
			}
			item.Preencher(*n)
		}
		if equipe := t.Equipe(e.ID, a.GRR); len(equipe) > 1 {
			item.Equipe = NomesDaEquipe(t, equipe, a.GRR)
		}
		itens = append(itens, item)
	}
	return itens
}

// AplicarCorrecao grava na turma o que a tela de correção devolveu.
func AplicarCorrecao(t *turma.Turma, exercicio string, r correcao.Resultado) (lancadas, apagadas int) {
	for _, n := range r.Notas {
		t.RegistrarNota(n)
	}
	for _, grr := range r.Removidas {
		if t.RemoverNota(exercicio, grr) {
			apagadas++
		}
	}
	return len(r.Notas), apagadas
}

// LancarNota grava a nota de um aluno e, por padrão, dos demais integrantes
// da mesma entrega.
//
// A entrega em dupla é um trabalho só, e duas notas diferentes para ela seriam
// acidente quase sempre.
func LancarNota(t *turma.Turma, exercicio, grr string, valor float64, comentario string, comentarioMudou, soEste bool) []string {
	alvos := []string{turma.NormalizarGRR(grr)}
	if !soEste {
		alvos = t.Equipe(exercicio, grr)
	}
	agora := time.Now()
	for _, alvo := range alvos {
		n := turma.Nota{Exercicio: exercicio, GRR: alvo, Valor: valor, Comentario: comentario}
		if anterior, ok := t.Nota(exercicio, alvo); ok && !comentarioMudou {
			n.Comentario = anterior.Comentario
		}
		n.CorrigidoEm = agora
		t.RegistrarNota(n)
	}
	return alvos
}

// ApagarNota remove a nota de um aluno e, por padrão, a dos demais
// integrantes da mesma entrega.
func ApagarNota(t *turma.Turma, exercicio, grr string, soEste bool) int {
	alvos := []string{turma.NormalizarGRR(grr)}
	if !soEste {
		alvos = t.Equipe(exercicio, grr)
	}
	apagadas := 0
	for _, alvo := range alvos {
		if t.RemoverNota(exercicio, alvo) {
			apagadas++
		}
	}
	return apagadas
}

// --- exportações ---

// Exportar grava o relatório de entregas em markdown e a planilha de notas na
// pasta da turma, com os nomes padrão. Devolve os dois caminhos.
//
// O relatório sai identificado por GRR, que é a versão publicável no material
// da disciplina.
func Exportar(t *turma.Turma, pastaTurma string) (markdown, planilha string, err error) {
	markdown = filepath.Join(pastaTurma, nomeBase(t.Config)+"_entregas.md")
	var buf bytes.Buffer
	if err := export.Markdown(&buf, t, export.Opcoes{Identificacao: export.PorGRR}); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(markdown, buf.Bytes(), 0o644); err != nil {
		return "", "", err
	}

	planilha = filepath.Join(pastaTurma, nomeBase(t.Config)+"_notas_exercicios.xlsx")
	if err := export.NotasXLSX(planilha, t, OpcoesNotas(t)); err != nil {
		return "", "", err
	}
	return markdown, planilha, nil
}

// OpcoesNotas monta as opções padrão da planilha de notas.
func OpcoesNotas(t *turma.Turma) export.OpcoesNotas {
	return export.OpcoesNotas{Hoje: turma.Hoje()}
}

func nomeBase(c turma.Config) string {
	partes := []string{c.Codigo}
	if c.Turma != "" {
		partes = append(partes, c.Turma)
	}
	return strings.Join(partes, "_")
}
