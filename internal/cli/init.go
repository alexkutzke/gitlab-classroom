package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alexkutzke/gitlab-classroom/internal/diario"
	"github.com/alexkutzke/gitlab-classroom/internal/store"
	"github.com/alexkutzke/gitlab-classroom/internal/turma"
)

func cmdInit() *cobra.Command {
	var cfg turma.Config
	var pastaDiario string
	var semCadastro bool

	c := &cobra.Command{
		Use:   "init",
		Short: "Cria o .classroom/ na pasta da turma",
		Long: "Prepara o diretório de dados e importa o cadastro de alunos do .diario/\n" +
			"da mesma pasta, quando ele existe. Disciplina, turma e semestre também\n" +
			"são lidos de lá; as opções só entram para corrigir ou completar.\n\n" +
			"O turno é deduzido do nome da pasta (sufixo _n ou _t).",
		Example: "  classroom init\n" +
			"  classroom init --turno n --namespace ds122-alexkutzke",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			destino, err := pastaDestino()
			if err != nil {
				return err
			}
			absoluto, err := filepath.Abs(destino)
			if err != nil {
				return err
			}

			if pastaDiario == "" {
				pastaDiario = ".diario"
			}
			doDiario, err := diario.LerConfig(filepath.Join(absoluto, pastaDiario))
			if err != nil {
				return err
			}
			preencher(&cfg.Codigo, doDiario.Codigo)
			preencher(&cfg.Disciplina, doDiario.Disciplina)
			preencher(&cfg.Turma, doDiario.Turma)
			preencher(&cfg.Semestre, doDiario.Semestre)

			if cfg.Codigo == "" {
				return fmt.Errorf("código da disciplina não informado: use --codigo, ou rode dentro da pasta que tem o .diario/")
			}
			if cfg.Semestre == "" {
				return fmt.Errorf("semestre não informado: use --semestre AAAA-NN")
			}
			if cfg.Turno == "" {
				cfg.Turno = turnoDaPasta(absoluto)
			}
			if cfg.Turno == "" {
				return fmt.Errorf("turno não deduzido do nome da pasta %q: use --turno n ou --turno t",
					filepath.Base(absoluto))
			}
			if cfg.NamespaceModelos == "" {
				cfg.NamespaceModelos = strings.ToLower(cfg.Codigo) + "-alexkutzke"
			}
			if cfg.PadraoGrupo == "" {
				cfg.PadraoGrupo = "{codigo}-{ano}-{periodo}-{turno}-{grr}"
			}
			cfg.PastaDiario = pastaDiario
			cfg.Padroes()

			s, err := store.Criar(absoluto)
			if err != nil {
				return err
			}
			t := &turma.Turma{Config: cfg}

			if !semCadastro {
				alunos, err := diario.Ler(filepath.Join(absoluto, pastaDiario))
				switch e := err.(type) {
				case nil:
					res := diario.Importar(t, alunos)
					fmt.Printf("Cadastro importado do diario: %d aluno(s).\n", len(res.Novos))
				case *diario.ErrSemCadastro:
					avisar("Aviso: %s. Rode `classroom sync --diario <pasta>` quando tiver o cadastro.", e)
				default:
					return err
				}
			}

			if err := s.Gravar(t); err != nil {
				return err
			}
			fmt.Printf("Turma %s criada em %s\n", cfg.Descricao(), s.Raiz)
			fmt.Printf("Grupo esperado de cada aluno: %s\n", cfg.CaminhoGrupo("grr20259999"))
			fmt.Println("Próximo passo: `classroom exercicios add --repo <repositório> --prazo AAAA-MM-DD`.")
			return nil
		},
	}
	c.Flags().StringVar(&cfg.Codigo, "codigo", "", "código da disciplina (padrão: o do .diario/)")
	c.Flags().StringVar(&cfg.Disciplina, "disciplina", "", "nome da disciplina")
	c.Flags().StringVar(&cfg.Turma, "turma", "", "identificação da turma")
	c.Flags().StringVar(&cfg.Semestre, "semestre", "", "semestre no formato AAAA-NN")
	c.Flags().StringVar(&cfg.Turno, "turno", "", "turno da turma, n ou t (padrão: sufixo da pasta)")
	c.Flags().StringVar(&cfg.NamespaceModelos, "namespace", "", "grupo dos repositórios-modelo no GitLab")
	c.Flags().StringVar(&cfg.PadraoGrupo, "padrao-grupo", "", "padrão do nome do grupo do aluno")
	c.Flags().StringVar(&cfg.Host, "host", "", "endereço do GitLab (padrão: https://gitlab.com)")
	c.Flags().Float64Var(&cfg.NotaMaxima, "nota-maxima", 0, "nota máxima dos exercícios (padrão: 100)")
	c.Flags().StringVar(&pastaDiario, "diario", "", "pasta do diario, relativa à da turma (padrão: .diario)")
	c.Flags().BoolVar(&semCadastro, "sem-cadastro", false, "não importar o cadastro de alunos")
	return c
}

func preencher(destino *string, valor string) {
	if *destino == "" {
		*destino = valor
	}
}

// turnoDaPasta lê o sufixo _n ou _t do nome da pasta da turma, que é a
// convenção já usada nas pastas das disciplinas.
func turnoDaPasta(caminho string) string {
	base := strings.ToLower(filepath.Base(caminho))
	switch {
	case strings.HasSuffix(base, "_n"):
		return "n"
	case strings.HasSuffix(base, "_t"):
		return "t"
	}
	return ""
}
