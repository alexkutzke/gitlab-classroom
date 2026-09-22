package acoes

import (
	"context"
	"fmt"
	"strings"
	"time"

	gl "github.com/alexkutzke/gitlab-classroom/internal/gitlab"
	"github.com/alexkutzke/gitlab-classroom/internal/turma"
)

// OpcoesDevolutiva controla a publicação do comentário da correção como issue
// no fork do aluno.
type OpcoesDevolutiva struct {
	// GRRs restringe a rodada a alguns alunos. Vazio vale a turma ativa.
	GRRs []string
	// Aplicar autoriza a escrita no GitLab. Sem ela, nada é enviado: o
	// ensaio é o padrão porque issue publicada não se desfaz em silêncio.
	Aplicar bool
	// Refazer republica quem já tem issue e cujo comentário mudou depois,
	// como comentário novo na issue existente.
	Refazer bool
	// Prazo é a data até quando o aluno pode comentar, escrita no corpo.
	Prazo turma.Data
	// Confirmar, quando definida, pergunta aluno a aluno antes de cada
	// publicação. Só vale com Aplicar: revisar sem publicar é o ensaio.
	Confirmar func(ItemDevolutiva) (DecisaoDevolutiva, error)
	// Editar abre o comentário da correção para edição e devolve o texto
	// novo. Recebe e devolve apenas o comentário, sem a menção nem o
	// rodapé, que são montados na hora da publicação.
	Editar func(comentario string) (string, error)
	// Gravar persiste a turma. É chamada a cada publicação, e não ao final
	// da rodada: interrupção no meio não pode perder o registro do que já
	// foi para o GitLab, senão a rodada seguinte republica a issue.
	Gravar func() error
}

// DecisaoDevolutiva é a resposta da revisão texto por texto.
type DecisaoDevolutiva string

const (
	// DecisaoPublicar publica o aluno atual e segue para o próximo.
	DecisaoPublicar DecisaoDevolutiva = "publicar"
	// DecisaoPular deixa o aluno de fora, sem publicar nem registrar.
	DecisaoPular DecisaoDevolutiva = "pular"
	// DecisaoEditar abre o comentário para edição e volta a perguntar.
	DecisaoEditar DecisaoDevolutiva = "editar"
	// DecisaoTodas publica o atual e os restantes sem perguntar de novo.
	DecisaoTodas DecisaoDevolutiva = "todas"
	// DecisaoSair encerra a rodada, preservando o que já foi publicado.
	DecisaoSair DecisaoDevolutiva = "sair"
)

// AcaoDevolutiva é o que a rodada faz com um aluno.
type AcaoDevolutiva string

const (
	// DevolutivaCriar abre a issue no fork.
	DevolutivaCriar AcaoDevolutiva = "criar"
	// DevolutivaComentar acrescenta o texto revisto à issue já aberta.
	DevolutivaComentar AcaoDevolutiva = "comentar"
	// DevolutivaReconhecer adota a issue de mesmo título já existente no
	// projeto, sem publicar nada. É a defesa para o devolutivas.csv perdido
	// e para a issue aberta à mão.
	DevolutivaReconhecer AcaoDevolutiva = "reconhecer"
	// DevolutivaPular é o aluno que fica de fora, com o motivo em Motivo.
	DevolutivaPular AcaoDevolutiva = "pular"
)

// Motivos de exclusão da rodada, mostrados no resumo do final.
const (
	MotivoSemNota       = "sem nota"
	MotivoSemComentario = "nota sem comentário"
	MotivoSemFork       = "sem fork"
	MotivoJaPublicada   = "já publicada"
	MotivoDesatualizada = "devolutiva desatualizada"
	MotivoPulada        = "pulada na revisão"
	MotivoNaoRevisada   = "rodada encerrada antes da revisão"
)

// ItemDevolutiva é o que a rodada apurou sobre um aluno.
type ItemDevolutiva struct {
	Exercicio string
	GRR       string
	Nome      string
	Acao      AcaoDevolutiva
	// Motivo explica a exclusão quando a ação é DevolutivaPular.
	Motivo  string
	Projeto string
	Titulo  string
	// Comentario é o texto da correção que vai ao aluno, e Corpo é a issue
	// inteira, com a menção e o commit avaliado em volta dele.
	Comentario string
	Corpo      string
	Hash       string
	// Issue e URL vêm preenchidas quando já existe issue, seja do arquivo,
	// seja da consulta ao projeto.
	Issue int64
	URL   string
	// Equipe são os demais integrantes que recebem a mesma issue. Na entrega
	// em dupla há um fork só, então há uma issue só.
	Equipe []string
	Erro   string
}

// ResumoDevolutiva descreve a rodada inteira.
type ResumoDevolutiva struct {
	Itens []ItemDevolutiva
	// Publicadas, Comentadas e Reconhecidas contam issues, e não alunos: a
	// entrega em dupla soma uma.
	Publicadas   int
	Comentadas   int
	Reconhecidas int
	// Fora conta os alunos excluídos, por motivo.
	Fora map[string]int
	// Erros são as falhas de publicação, uma por issue tentada.
	Erros []string
}

// Devolutivas publica, ou ensaia publicar, o comentário da correção de cada
// aluno como issue no fork onde a entrega está.
//
// Sem OpcoesDevolutiva.Aplicar nada é enviado ao GitLab, e a turma não é
// alterada. Com ela, o registro de cada publicação entra em t.Devolutivas, e
// quem chamou grava. Erro em um aluno não interrompe os demais: o que já foi
// publicado precisa chegar ao arquivo, senão a rodada seguinte duplica a
// issue.
func Devolutivas(ctx context.Context, t *turma.Turma, cli gl.Cliente, exercicios []turma.Exercicio, o OpcoesDevolutiva, prog AvisoProgresso) (ResumoDevolutiva, error) {
	res := ResumoDevolutiva{Fora: map[string]int{}}
	if cli == nil {
		return res, fmt.Errorf("sem conexão com o GitLab")
	}
	// O cliente pode vir de uma operação anterior, com as issues de cada
	// projeto memorizadas. Descartar aqui é o que faz a rodada enxergar a
	// issue aberta desde a última.
	cli.Renovar()

	alunos, err := alunosDaRodada(t, o.GRRs)
	if err != nil {
		return res, err
	}

	var planos []ItemDevolutiva
	for _, e := range exercicios {
		itens, err := planejarDevolutivas(t, cli, e, alunos, o)
		if err != nil {
			return res, err
		}
		planos = append(planos, itens...)
	}

	total := 0
	for _, it := range planos {
		if it.Acao != DevolutivaPular {
			total++
		}
	}

	porID := map[string]turma.Exercicio{}
	for _, e := range exercicios {
		porID[e.ID] = e
	}

	feito := 0
	revisar := o.Aplicar && o.Confirmar != nil
	for i, it := range planos {
		if it.Acao == DevolutivaPular {
			res.Fora[it.Motivo]++
			res.Itens = append(res.Itens, it)
			continue
		}
		if err := ctx.Err(); err != nil {
			res.Itens = append(res.Itens, it)
			return res, err
		}

		if revisar {
			dec, err := revisarDevolutiva(t, porID[it.Exercicio], &it, o)
			if err != nil {
				res.Itens = append(res.Itens, it)
				return res, err
			}
			switch dec {
			case DecisaoSair:
				// O que já foi publicado está gravado. O resto entra no
				// resumo como não revisado, para a contagem final não
				// sugerir que a rodada cobriu a turma inteira.
				encerrarRevisao(&res, planos[i:])
				return res, nil
			case DecisaoPular:
				it.Acao, it.Motivo = DevolutivaPular, MotivoPulada
				res.Fora[it.Motivo]++
				res.Itens = append(res.Itens, it)
				continue
			case DecisaoTodas:
				revisar = false
			}
		}

		feito++
		prog.avisar(feito, total, it.Nome)

		if o.Aplicar {
			if err := aplicarDevolutiva(t, cli, it); err != nil {
				it.Erro = err.Error()
				res.Erros = append(res.Erros, fmt.Sprintf("%s: %v", it.Nome, err))
				res.Itens = append(res.Itens, it)
				continue
			}
			// Gravar aqui, e não no fim: queda de rede ou Ctrl+C depois
			// desta issue não pode apagar o registro dela.
			if o.Gravar != nil {
				if err := o.Gravar(); err != nil {
					return res, err
				}
			}
		}
		switch it.Acao {
		case DevolutivaCriar:
			res.Publicadas++
		case DevolutivaComentar:
			res.Comentadas++
		case DevolutivaReconhecer:
			res.Reconhecidas++
		}
		res.Itens = append(res.Itens, it)
	}
	return res, nil
}

// revisarDevolutiva pergunta o que fazer com um aluno, repetindo a pergunta
// depois de cada edição do comentário.
//
// A edição grava o texto novo em notas.csv: o comentário publicado é o
// comentário da correção, senão o hash deixaria de detectar a devolutiva
// desatualizada. O corrigido_em não muda, porque a nota não mudou.
func revisarDevolutiva(t *turma.Turma, e turma.Exercicio, it *ItemDevolutiva, o OpcoesDevolutiva) (DecisaoDevolutiva, error) {
	for {
		dec, err := o.Confirmar(*it)
		if err != nil {
			return "", err
		}
		if dec != DecisaoEditar {
			return dec, nil
		}
		if o.Editar == nil {
			continue
		}
		novo, err := o.Editar(it.Comentario)
		if err != nil {
			return "", err
		}
		novo = strings.TrimSpace(novo)
		if novo == "" || novo == strings.TrimSpace(it.Comentario) {
			continue
		}
		if n, ok := t.Nota(it.Exercicio, it.GRR); ok {
			n.Comentario = novo
		}
		it.Comentario = novo
		it.Hash = turma.HashComentario(novo)
		it.Corpo = corpoDevolutiva(t, e, *it, o.Prazo)
		if o.Gravar != nil {
			if err := o.Gravar(); err != nil {
				return "", err
			}
		}
	}
}

// encerrarRevisao registra no resumo os alunos que a saída antecipada deixou
// sem revisar.
func encerrarRevisao(res *ResumoDevolutiva, restantes []ItemDevolutiva) {
	for _, it := range restantes {
		if it.Acao != DevolutivaPular {
			it.Acao, it.Motivo = DevolutivaPular, MotivoNaoRevisada
		}
		res.Fora[it.Motivo]++
		res.Itens = append(res.Itens, it)
	}
}

// alunosDaRodada resolve os GRRs informados, ou devolve a turma ativa.
func alunosDaRodada(t *turma.Turma, grrs []string) ([]turma.Aluno, error) {
	if len(grrs) == 0 {
		alunos := t.Ativos()
		if len(alunos) == 0 {
			return nil, fmt.Errorf("nenhum aluno ativo: rode `classroom sync` para importar o cadastro")
		}
		return alunos, nil
	}
	var out []turma.Aluno
	for _, grr := range grrs {
		a, ok := t.AlunoPorGRR(grr)
		if !ok {
			return nil, fmt.Errorf("aluno %q não encontrado", grr)
		}
		out = append(out, *a)
	}
	return out, nil
}

// planejarDevolutivas decide o que fazer com cada aluno de um exercício.
//
// A decisão é por fork, e não por aluno: na entrega em dupla os dois leem a
// mesma issue, e abrir uma para cada seria mandar a devolutiva duas vezes
// para o mesmo repositório.
func planejarDevolutivas(t *turma.Turma, cli gl.Cliente, e turma.Exercicio, alunos []turma.Aluno, o OpcoesDevolutiva) ([]ItemDevolutiva, error) {
	var itens []ItemDevolutiva
	// projetos guarda a posição, em itens, do aluno que representa cada fork
	// já visto.
	projetos := map[string]int{}

	for _, a := range alunos {
		it := ItemDevolutiva{Exercicio: e.ID, GRR: a.GRR, Nome: a.Nome, Acao: DevolutivaPular}

		nota, ok := t.Nota(e.ID, a.GRR)
		if !ok {
			it.Motivo = MotivoSemNota
			itens = append(itens, it)
			continue
		}
		if strings.TrimSpace(nota.Comentario) == "" {
			it.Motivo = MotivoSemComentario
			itens = append(itens, it)
			continue
		}
		projeto, ok := projetoDaEntrega(t, e.ID, a.GRR)
		if !ok {
			it.Motivo = MotivoSemFork
			itens = append(itens, it)
			continue
		}
		it.Projeto = projeto

		// Segundo integrante da mesma entrega: entra na equipe do primeiro,
		// que é quem carrega a issue. A devolutiva publicada é a do primeiro,
		// e o colega só se acrescenta à menção.
		if i, visto := projetos[projeto]; visto {
			itens[i].Equipe = append(itens[i].Equipe, a.GRR)
			itens[i].Corpo = corpoDevolutiva(t, e, itens[i], o.Prazo)
			it.Motivo = "entrega compartilhada com " + itens[i].Nome
			itens = append(itens, it)
			continue
		}

		it.Titulo = tituloDevolutiva(e)
		it.Comentario = nota.Comentario
		it.Hash = turma.HashComentario(nota.Comentario)
		it.Corpo = corpoDevolutiva(t, e, it, o.Prazo)

		if d, ok := devolutivaDoFork(t, e.ID, a.GRR, projeto); ok {
			it.Issue, it.URL = d.Issue, d.URL
			switch {
			case !d.Desatualizada(it.Comentario):
				it.Motivo = MotivoJaPublicada
			case !o.Refazer:
				it.Motivo = MotivoDesatualizada
			default:
				it.Acao = DevolutivaComentar
			}
			projetos[projeto] = len(itens)
			itens = append(itens, it)
			continue
		}

		// Sem linha no arquivo: a issue pode existir assim mesmo, criada à
		// mão ou por uma rodada cujo arquivo se perdeu. Publicar de novo
		// mandaria a mesma devolutiva duas vezes.
		issues, err := cli.IssuesDoProjeto(projeto)
		if err != nil {
			return nil, err
		}
		if existente, achou := acharIssue(issues, it.Titulo); achou {
			it.Acao = DevolutivaReconhecer
			it.Issue, it.URL = existente.IID, existente.URL
		} else {
			it.Acao = DevolutivaCriar
		}
		projetos[projeto] = len(itens)
		itens = append(itens, it)
	}
	return itens, nil
}

// aplicarDevolutiva executa a ação no GitLab e registra o resultado na turma.
func aplicarDevolutiva(t *turma.Turma, cli gl.Cliente, it ItemDevolutiva) error {
	switch it.Acao {
	case DevolutivaCriar:
		issue, err := cli.CriarIssue(it.Projeto, it.Titulo, it.Corpo)
		if err != nil {
			return err
		}
		it.Issue, it.URL = issue.IID, issue.URL
	case DevolutivaComentar:
		if err := cli.ComentarIssue(it.Projeto, it.Issue, it.Corpo); err != nil {
			return err
		}
	}

	agora := time.Now()
	for _, grr := range append([]string{it.GRR}, it.Equipe...) {
		t.RegistrarDevolutiva(turma.Devolutiva{
			Exercicio: it.Exercicio, GRR: grr, Projeto: it.Projeto,
			Issue: it.Issue, URL: it.URL, PublicadoEm: agora, Hash: it.Hash,
		})
	}
	return nil
}

// projetoDaEntrega devolve o fork onde a entrega do aluno está.
//
// A coleta grava o caminho do grupo no lugar do projeto quando o aluno nem
// chegou a ter grupo visível, e grupo não recebe issue. O que distingue os
// dois é a barra: o caminho de um projeto sempre traz o namespace.
func projetoDaEntrega(t *turma.Turma, exercicio, grr string) (string, bool) {
	en, ok := t.Entrega(exercicio, grr)
	if !ok || !strings.Contains(en.Projeto, "/") {
		return "", false
	}
	return en.Projeto, true
}

// devolutivaDoFork procura o registro de publicação daquele fork.
//
// A busca é pelo aluno e, na falta dele, pelo projeto: na entrega em dupla a
// linha pode ter ficado no nome do colega, e a issue é a mesma.
func devolutivaDoFork(t *turma.Turma, exercicio, grr, projeto string) (turma.Devolutiva, bool) {
	if d, ok := t.Devolutiva(exercicio, grr); ok {
		return *d, true
	}
	for _, d := range t.Devolutivas {
		if d.Exercicio == exercicio && d.Projeto == projeto {
			return d, true
		}
	}
	return turma.Devolutiva{}, false
}

func acharIssue(issues []gl.Issue, titulo string) (gl.Issue, bool) {
	for _, i := range issues {
		if strings.EqualFold(strings.TrimSpace(i.Titulo), titulo) {
			return i, true
		}
	}
	return gl.Issue{}, false
}

// tituloDevolutiva é o que identifica a issue na rodada seguinte, então não
// pode variar com o aluno nem com a data.
func tituloDevolutiva(e turma.Exercicio) string {
	if titulo := strings.TrimSpace(e.Titulo); titulo != "" {
		return "Devolutiva: " + titulo
	}
	return "Devolutiva: " + e.ID
}

// corpoDevolutiva monta o texto da issue.
//
// A menção ao usuário é o que notifica o aluno: a issue não é atribuída nem
// etiquetada, porque atribuir exige o id numérico da conta e etiqueta teria
// de ser criada em cada projeto. A nota não entra, nem aqui nem no título: o
// lugar dela é o diario e o UFPR Virtual.
func corpoDevolutiva(t *turma.Turma, e turma.Exercicio, it ItemDevolutiva, prazo turma.Data) string {
	var b strings.Builder
	if m := mencoes(t, append([]string{it.GRR}, it.Equipe...)); m != "" {
		fmt.Fprintf(&b, "%s, segue a devolutiva da sua entrega.\n\n", m)
	} else {
		// Aluno sem login cadastrado fica sem menção, e sem notificação. O
		// texto ainda serve a quem abrir o repositório.
		b.WriteString("Segue a devolutiva da sua entrega.\n\n")
	}
	b.WriteString(strings.TrimSpace(it.Comentario))
	b.WriteString("\n\n")

	if commit := commitAvaliado(t, e.ID, it.GRR); commit != "" {
		fmt.Fprintf(&b, "Commit avaliado: %s\n", commit)
	}
	if !prazo.IsZero() {
		fmt.Fprintf(&b, "Comentários até %s. ", prazo.Format("02/01/2006"))
	}
	b.WriteString("Responda aqui mesmo se discordar de algum ponto ou quiser " +
		"entender melhor a correção.\n")
	return b.String()
}

// mencoes monta a chamada dos alunos que têm login cadastrado.
func mencoes(t *turma.Turma, grrs []string) string {
	var out []string
	for _, grr := range grrs {
		a, ok := t.AlunoPorGRR(grr)
		if !ok || a.Usuario == "" {
			continue
		}
		out = append(out, "@"+a.Usuario)
	}
	return strings.Join(out, ", ")
}

// commitAvaliado devolve o SHA curto do commit que a correção olhou.
func commitAvaliado(t *turma.Turma, exercicio, grr string) string {
	en, ok := t.Entrega(exercicio, grr)
	if !ok || en.Commit == "" {
		return ""
	}
	if len(en.Commit) > 8 {
		return en.Commit[:8]
	}
	return en.Commit
}
