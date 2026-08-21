// Package verificacao executa a suíte automatizada de um exercício sobre os
// clones dos alunos.
//
// Rodar código de aluno é o ponto de risco desta ferramenta. O padrão é
// executar dentro de um contêiner sem rede, com o clone montado somente para
// leitura e com teto de memória, de processos e de tempo. A execução direta
// na máquina existe, mas é opção explícita.
package verificacao

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alexkutzke/gitlab-classroom/internal/turma"
)

// Alvo é o clone de um aluno a verificar.
type Alvo struct {
	GRR    string
	Nome   string
	Dir    string
	Commit string
}

// Opcoes descreve como rodar a suíte.
type Opcoes struct {
	Exercicio turma.Exercicio
	// Imagem sobrepõe a do exercício e a padrão da configuração.
	Imagem      string
	TempoLimite time.Duration
	// SemSandbox roda o comando direto na máquina, sem contêiner.
	SemSandbox bool
	// Escrita monta o clone com permissão de escrita, para a suíte que
	// precisa gerar arquivo dentro do repositório.
	Escrita bool
	// Runtime é o programa de contêiner; vazio procura podman e depois docker.
	Runtime string
	// PastaLogs recebe a saída completa de cada execução.
	PastaLogs string
	// Memoria e Processos são os tetos do contêiner.
	Memoria   string
	Processos int
}

func (o *Opcoes) padroes() {
	if o.TempoLimite <= 0 {
		o.TempoLimite = 2 * time.Minute
	}
	if o.Memoria == "" {
		o.Memoria = "512m"
	}
	if o.Processos <= 0 {
		o.Processos = 256
	}
}

// Runtime devolve o programa de contêiner disponível.
func Runtime(preferido string) (string, error) {
	candidatos := []string{preferido, "podman", "docker"}
	for _, c := range candidatos {
		if c == "" {
			continue
		}
		if caminho, err := exec.LookPath(c); err == nil {
			return caminho, nil
		}
	}
	return "", fmt.Errorf("nenhum runtime de contêiner encontrado (podman ou docker); use --sem-sandbox por sua conta e risco")
}

// resultado casa a linha que a suíte pode imprimir para informar quantos
// casos passaram. Sem essa linha, vale só o código de saída.
var reResultado = regexp.MustCompile(`(?i)RESULTADO:\s*(\d+)\s*/\s*(\d+)`)

// Executar roda a suíte sobre todos os alvos, em paralelo.
func Executar(ctx context.Context, alvos []Alvo, o Opcoes, paralelismo int, progresso func(feito, total int, a Alvo)) ([]turma.Verificacao, error) {
	o.padroes()
	if !o.Exercicio.TemSuite() {
		var out []turma.Verificacao
		agora := time.Now()
		for _, a := range alvos {
			out = append(out, turma.Verificacao{
				Exercicio: o.Exercicio.ID, GRR: a.GRR, Situacao: turma.SemSuite,
				Commit: a.Commit, ExecutadoEm: agora,
			})
		}
		return out, nil
	}

	runtime := ""
	if !o.SemSandbox {
		var err error
		if runtime, err = Runtime(o.Runtime); err != nil {
			return nil, err
		}
	}

	if o.PastaLogs != "" {
		if err := os.MkdirAll(o.PastaLogs, 0o755); err != nil {
			return nil, err
		}
	}

	if paralelismo <= 0 {
		paralelismo = 4
	}
	if paralelismo > len(alvos) {
		paralelismo = len(alvos)
	}

	out := make([]turma.Verificacao, len(alvos))
	indices := make(chan int)
	var wg sync.WaitGroup
	var mu sync.Mutex
	feito := 0

	for w := 0; w < paralelismo; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range indices {
				a := alvos[i]
				if ctx.Err() != nil {
					continue
				}
				v := executarUm(ctx, a, o, runtime)

				mu.Lock()
				out[i] = v
				feito++
				if progresso != nil {
					progresso(feito, len(alvos), a)
				}
				mu.Unlock()
			}
		}()
	}
	for i := range alvos {
		indices <- i
	}
	close(indices)
	wg.Wait()

	return out, nil
}

func executarUm(pai context.Context, a Alvo, o Opcoes, runtime string) turma.Verificacao {
	v := turma.Verificacao{
		Exercicio: o.Exercicio.ID, GRR: a.GRR, Commit: a.Commit,
		ExecutadoEm: time.Now(),
	}
	if a.Dir == "" {
		v.Situacao = turma.SemClone
		return v
	}
	if info, err := os.Stat(a.Dir); err != nil || !info.IsDir() {
		v.Situacao = turma.SemClone
		return v
	}

	ctx, cancelar := context.WithTimeout(pai, o.TempoLimite)
	defer cancelar()

	var cmd *exec.Cmd
	if o.SemSandbox {
		cmd = exec.CommandContext(ctx, "sh", "-c", o.Exercicio.Verificacao)
		cmd.Dir = a.Dir
	} else {
		cmd = exec.CommandContext(ctx, runtime, argsContainer(a, o)...)
	}
	// A suíte não conversa com quem a chamou: entrada fechada evita que um
	// script à espera de resposta consuma o tempo limite inteiro.
	cmd.Stdin = nil

	inicio := time.Now()
	saida, err := cmd.CombinedOutput()
	v.Duracao = time.Since(inicio)

	registrarLog(o, a, saida)

	if ctx.Err() == context.DeadlineExceeded {
		v.Situacao = turma.ErroVerificacao
		v.Detalhe = fmt.Sprintf("tempo esgotado depois de %s", o.TempoLimite)
		return v
	}

	codigo := 0
	if err != nil {
		var saiu *exec.ExitError
		if errors.As(err, &saiu) {
			codigo = saiu.ExitCode()
		} else {
			v.Situacao = turma.ErroVerificacao
			v.Detalhe = err.Error()
			return v
		}
	}

	if aprovados, total, ok := parseResultado(string(saida)); ok {
		v.Aprovados, v.Total = aprovados, total
	}

	switch {
	case codigo == 0:
		v.Situacao = turma.Aprovado
		if v.Total > 0 && v.Aprovados < v.Total {
			// A suíte disse que faltou caso, mesmo saindo com zero. O que ela
			// relata vale mais que o código de saída.
			v.Situacao = turma.Reprovado
		}
	case !o.SemSandbox && (codigo == 125 || codigo == 126 || codigo == 127):
		// Códigos que o runtime usa para dizer que nem chegou a rodar o
		// comando: imagem ausente, binário inexistente, contêiner que não
		// subiu. Isso é problema da suíte, não do aluno.
		v.Situacao = turma.ErroVerificacao
		v.Detalhe = fmt.Sprintf("o contêiner não executou a suíte (código %d)", codigo)
	default:
		v.Situacao = turma.Reprovado
		v.Detalhe = fmt.Sprintf("código de saída %d", codigo)
	}
	return v
}

// argsContainer monta a linha de comando do runtime.
func argsContainer(a Alvo, o Opcoes) []string {
	montagem := a.Dir + ":/repo:ro,z"
	if o.Escrita {
		montagem = a.Dir + ":/repo:rw,z"
	}
	imagem := o.Imagem
	if imagem == "" {
		imagem = o.Exercicio.Imagem
	}
	return []string{
		"run", "--rm",
		"--network=none",
		"--security-opt=no-new-privileges",
		"--memory=" + o.Memoria,
		"--pids-limit=" + strconv.Itoa(o.Processos),
		"--cpus=1",
		"--tmpfs=/tmp:rw,size=64m",
		"--volume=" + montagem,
		"--workdir=/repo",
		imagem,
		"sh", "-c", o.Exercicio.Verificacao,
	}
}

func registrarLog(o Opcoes, a Alvo, saida []byte) {
	if o.PastaLogs == "" {
		return
	}
	nome := strings.ToLower(a.GRR) + ".log"
	cabecalho := fmt.Sprintf("# %s  %s\n# exercício %s  commit %s\n# %s\n\n",
		a.GRR, a.Nome, o.Exercicio.ID, a.Commit, time.Now().Format(time.RFC3339))
	_ = os.WriteFile(filepath.Join(o.PastaLogs, nome), append([]byte(cabecalho), saida...), 0o644)
}

func parseResultado(saida string) (aprovados, total int, ok bool) {
	casos := reResultado.FindAllStringSubmatch(saida, -1)
	if len(casos) == 0 {
		return 0, 0, false
	}
	// A última ocorrência é a que vale: a suíte pode imprimir parciais.
	ultimo := casos[len(casos)-1]
	a, err1 := strconv.Atoi(ultimo[1])
	t, err2 := strconv.Atoi(ultimo[2])
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return a, t, true
}
