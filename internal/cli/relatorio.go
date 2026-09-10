package cli

import (
	"bytes"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/alexkutzke/gitlab-classroom/internal/acoes"
	"github.com/alexkutzke/gitlab-classroom/internal/export"
	"github.com/alexkutzke/gitlab-classroom/internal/turma"
)

func cmdRelatorio() *cobra.Command {
	var ids []string
	var saida, identificacao, categoria string
	var detalhado bool

	c := &cobra.Command{
		Use:   "relatorio",
		Short: "Gera a tabela de entregas em markdown",
		Long: "Uma linha por aluno, uma coluna por exercício, no formato que os scripts\n" +
			"antigos publicavam no material da disciplina. Sem -o, escreve na saída\n" +
			"padrão.\n\n" +
			"A tabela leva o nome dos alunos. Se o destino for material publicado,\n" +
			"use --identificacao grr.",
		Example: "  classroom relatorio\n" +
			"  classroom relatorio -o ../ds122-alexkutzke/src/entregas.md\n" +
			"  classroom relatorio --exercicio html --identificacao grr\n" +
			"  classroom relatorio --categoria trabalho",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, t, err := abrir()
			if err != nil {
				return err
			}
			var exercicios []turma.Exercicio
			if len(ids) > 0 {
				if exercicios, err = acoes.EscolherExercicios(t, ids); err != nil {
					return err
				}
			}
			if categoria != "" && len(ids) == 0 {
				// Publicar a tabela do trabalho em separado é o caso de uso:
				// as partes têm prazos próprios e vão para outra página do
				// material.
				if exercicios = t.ExerciciosDaCategoria(categoria); len(exercicios) == 0 {
					return fmt.Errorf("nenhum exercício ativo na categoria %q", categoria)
				}
			}

			if detalhado {
				return fmt.Errorf("--detalhado ainda não existe aqui; use `classroom coletar --detalhado`")
			}

			var buf bytes.Buffer
			opts := export.Opcoes{
				Identificacao: export.Identificacao(identificacao),
				Exercicios:    exercicios,
			}
			if err := export.Markdown(&buf, t, opts); err != nil {
				return err
			}
			if saida == "" {
				_, err := os.Stdout.Write(buf.Bytes())
				return err
			}
			if err := os.WriteFile(saida, buf.Bytes(), 0o644); err != nil {
				return err
			}
			fmt.Printf("Relatório gravado em %s\n", saida)
			return nil
		},
	}
	c.Flags().StringArrayVar(&ids, "exercicio", nil, "exercício a incluir (repetível; padrão: todos os ativos)")
	c.Flags().StringVarP(&saida, "saida", "o", "", "arquivo de destino (padrão: saída padrão)")
	c.Flags().StringVar(&identificacao, "identificacao", "nome", "como identificar o aluno na tabela: nome ou grr")
	c.Flags().StringVar(&categoria, "categoria", "", "restringir a uma categoria de exercício, como exercicio ou trabalho")
	c.Flags().BoolVar(&detalhado, "detalhado", false, "reservado")
	return c
}
