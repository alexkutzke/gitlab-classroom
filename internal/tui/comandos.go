package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/alexkutzke/gitlab-classroom/internal/acoes"
	"github.com/alexkutzke/gitlab-classroom/internal/turma"
)

// Aqui ficam as ações que a interface dispara. Nenhuma delas tem lógica
// própria: todas chamam internal/acoes, as mesmas funções dos subcomandos.

// coletar apura as entregas dos exercícios informados; lista vazia coleta
// todos os ativos.
func (a *App) coletar(exercicios []turma.Exercicio) tea.Cmd {
	if len(exercicios) == 0 {
		exercicios = a.turma.ExerciciosAtivos()
	}
	if len(exercicios) == 0 {
		a.erro = "nenhum exercício cadastrado"
		return nil
	}
	cli, err := a.conectar()
	if err != nil {
		a.erro = err.Error()
		return nil
	}

	nome := "coleta de " + listarIDs(exercicios)
	return a.iniciar(nome, func(ctx context.Context, prog acoes.AvisoProgresso) fimMsg {
		res, err := acoes.Coletar(ctx, a.turma, cli, exercicios, prog)
		if err != nil {
			return fimMsg{err: err}
		}
		var partes, detalhes []string
		for _, e := range exercicios {
			partes = append(partes, fmt.Sprintf("%s: %s", e.ID, resumirSituacoes(res.Situacoes[e.ID])))
			if n := res.Compartilhadas[e.ID]; n > 0 {
				detalhes = append(detalhes, fmt.Sprintf("%s: %d entrega(s) compartilhada(s)", e.ID, n))
			}
		}
		return fimMsg{resumo: strings.Join(partes, "; "), detalhes: detalhes, gravar: true}
	})
}

// clonar baixa os forks do exercício aberto.
func (a *App) clonar(e turma.Exercicio) tea.Cmd {
	return a.iniciar("clone de "+e.ID, func(ctx context.Context, prog acoes.AvisoProgresso) fimMsg {
		res, err := acoes.Clonar(ctx, a.turma, a.store.Pasta(), []turma.Exercicio{e},
			acoes.OpcoesClone{}, prog)
		if err != nil {
			return fimMsg{err: err}
		}
		var detalhes []string
		for _, f := range res.Falhas {
			detalhes = append(detalhes, fmt.Sprintf("%s: %v", f.GRR, f.Erro))
		}
		resumo := fmt.Sprintf("%d clonado(s), %d atualizado(s), %d com falha",
			res.Novos, res.Atualizados, len(res.Falhas))
		return fimMsg{resumo: resumo, detalhes: detalhes}
	})
}

// verificar roda a suíte automatizada sobre os clones do exercício aberto.
func (a *App) verificar(e turma.Exercicio) tea.Cmd {
	if !e.TemSuite() {
		a.erro = "o exercício " + e.ID + " não tem suíte cadastrada"
		return nil
	}
	return a.iniciar("verificação de "+e.ID, func(ctx context.Context, prog acoes.AvisoProgresso) fimMsg {
		res, err := acoes.Verificar(ctx, a.turma, a.store.Pasta(), e, acoes.OpcoesVerificacao{}, prog)
		if err != nil {
			return fimMsg{err: err}
		}
		var detalhes []string
		for _, v := range res.Problemas {
			nome := v.GRR
			if al, ok := a.turma.AlunoPorGRR(v.GRR); ok {
				nome = al.Nome
			}
			detalhes = append(detalhes, fmt.Sprintf("%s: %s", nome, v.Resumo()))
		}
		resumo := fmt.Sprintf("%d aprovado(s), %d reprovado(s), %d sem clone, %d com erro",
			res.Contagem[turma.Aprovado], res.Contagem[turma.Reprovado],
			res.Contagem[turma.SemClone], res.Contagem[turma.ErroVerificacao])
		return fimMsg{resumo: resumo, detalhes: detalhes, gravar: true}
	})
}

// desconhecidos varre os forks atrás de membros que não casam com o cadastro.
//
// O resultado vai para a tela de tarefas: são poucas linhas, e o professor
// precisa delas na mão para decidir se cadastra o login.
func (a *App) desconhecidos() tea.Cmd {
	exercicios := a.turma.ExerciciosAtivos()
	if len(exercicios) == 0 {
		a.erro = "nenhum exercício cadastrado"
		return nil
	}
	cli, err := a.conectar()
	if err != nil {
		a.erro = err.Error()
		return nil
	}
	return a.iniciar("busca de membros não reconhecidos",
		func(ctx context.Context, prog acoes.AvisoProgresso) fimMsg {
			achados, err := acoes.MembrosDesconhecidos(ctx, a.turma, cli, exercicios, prog)
			if err != nil {
				return fimMsg{err: err}
			}
			if len(achados) == 0 {
				return fimMsg{resumo: "todo membro de fork corresponde a um aluno"}
			}
			var detalhes []string
			for _, m := range achados {
				palpite := "sem palpite"
				if m.TemSugestao {
					palpite = "talvez " + m.Sugestao.GRR + " " + m.Sugestao.Nome
				}
				detalhes = append(detalhes, fmt.Sprintf("%s: %s (%s) no fork de %s, %s",
					m.Exercicio, m.Usuario, m.Nome, m.DonoNome, palpite))
			}
			return fimMsg{
				resumo:   fmt.Sprintf("%d membro(s) sem correspondência, veja a tela de tarefas", len(achados)),
				detalhes: detalhes,
			}
		})
}

// sincronizar reimporta o cadastro do diario e reconcilia as contas.
func (a *App) sincronizar() tea.Cmd {
	cli, err := a.conectar()
	if err != nil {
		a.erro = err.Error()
		return nil
	}
	return a.iniciar("sincronização do cadastro", func(ctx context.Context, prog acoes.AvisoProgresso) fimMsg {
		res, err := acoes.Sincronizar(ctx, a.turma, a.store.Pasta(), cli, acoes.OpcoesSync{}, prog)
		if err != nil {
			return fimMsg{err: err}
		}
		var detalhes []string
		if res.SemCadastro != "" {
			detalhes = append(detalhes, res.SemCadastro)
		}
		for _, grr := range res.Importacao.Novos {
			detalhes = append(detalhes, "novo: "+grr)
		}
		for _, grr := range res.Importacao.Cancelados {
			detalhes = append(detalhes, "cancelado: "+grr)
		}
		pendentes := 0
		for sit, n := range res.Contas {
			if sit != turma.ContaOK {
				pendentes += n
			}
		}
		resumo := fmt.Sprintf("%d conta(s) em ordem, %d com pendência",
			res.Contas[turma.ContaOK], pendentes)
		return fimMsg{resumo: resumo, detalhes: detalhes, gravar: true}
	})
}

// resumirSituacoes descreve a contagem por situação, da mais comum para a
// menos comum.
func resumirSituacoes(contagem map[turma.SituacaoEntrega]int) string {
	if len(contagem) == 0 {
		return "nada encontrado"
	}
	var sits []turma.SituacaoEntrega
	for s := range contagem {
		sits = append(sits, s)
	}
	sort.Slice(sits, func(i, j int) bool {
		if contagem[sits[i]] != contagem[sits[j]] {
			return contagem[sits[i]] > contagem[sits[j]]
		}
		return sits[i] < sits[j]
	})
	var partes []string
	for _, s := range sits {
		partes = append(partes, fmt.Sprintf("%d %s", contagem[s], s.Rotulo()))
	}
	return strings.Join(partes, ", ")
}

func listarIDs(es []turma.Exercicio) string {
	var ids []string
	for _, e := range es {
		ids = append(ids, e.ID)
	}
	return strings.Join(ids, ", ")
}
