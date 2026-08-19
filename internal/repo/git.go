// Package repo cuida dos clones locais dos forks dos alunos.
//
// Executa o git do sistema por os/exec, sem biblioteca git em Go: as
// operações usadas são poucas e a autenticação já vem pronta da chave SSH do
// professor.
package repo

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// TempoLimite é o teto de cada operação de git. Clone de repositório de
// exercício leva segundos; o limite existe para uma rede ruim não travar a
// coleta inteira.
const TempoLimite = 5 * time.Minute

// Repo é um clone local.
type Repo struct {
	Dir string
}

// URLSSH monta o endereço de clone por SSH a partir do host configurado.
func URLSSH(host, projeto string) string {
	h := strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(host, "https://"), "http://"), "/")
	if h == "" {
		h = "gitlab.com"
	}
	return fmt.Sprintf("ssh://git@%s/%s.git", h, projeto)
}

// Existe informa se o diretório já contém um clone.
func Existe(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil && info.IsDir()
}

// Preparar clona o repositório ou, se o clone já existir, atualiza as
// referências. Devolve o clone pronto para ser posicionado.
func Preparar(dir, url string) (*Repo, error) {
	if Existe(dir) {
		r := &Repo{Dir: dir}
		// A URL pode ter mudado desde o clone, quando o aluno renomeia o
		// projeto ou o grupo.
		if _, err := r.git("remote", "set-url", "origin", url); err != nil {
			return nil, err
		}
		if _, err := r.git("fetch", "--prune", "--tags", "--quiet", "origin"); err != nil {
			return nil, err
		}
		return r, nil
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return nil, err
	}
	if _, err := rodar("", TempoLimite, "clone", "--quiet", url, dir); err != nil {
		return nil, err
	}
	return &Repo{Dir: dir}, nil
}

// Posicionar deixa o clone no commit avaliado, sob um ramo local de nome
// previsível.
//
// Um ramo, e não um checkout solto, porque HEAD destacado silencioso foi o
// defeito do script antigo: quem abrisse a pasta depois não tinha como saber
// em que ponto do histórico estava olhando.
func (r *Repo) Posicionar(ramo, commit string) (string, error) {
	alvo := commit
	if alvo == "" {
		padrao, err := r.RamoPadrao()
		if err != nil {
			return "", err
		}
		alvo = "origin/" + padrao
	}
	if _, err := r.git("switch", "--quiet", "--force-create", ramo, alvo); err != nil {
		return "", err
	}
	return r.HEAD()
}

// RamoPadrao devolve o ramo padrão do remoto.
func (r *Repo) RamoPadrao() (string, error) {
	saida, err := r.git("symbolic-ref", "--short", "refs/remotes/origin/HEAD")
	if err != nil {
		// Repositório clonado antes de o remoto ter HEAD definido: tenta os
		// nomes usuais antes de desistir.
		for _, nome := range []string{"main", "master"} {
			if _, err := r.git("rev-parse", "--verify", "--quiet", "origin/"+nome); err == nil {
				return nome, nil
			}
		}
		return "", fmt.Errorf("ramo padrão de %s não identificado", r.Dir)
	}
	return strings.TrimPrefix(strings.TrimSpace(saida), "origin/"), nil
}

// HEAD devolve o commit atual do clone.
func (r *Repo) HEAD() (string, error) {
	saida, err := r.git("rev-parse", "HEAD")
	return strings.TrimSpace(saida), err
}

// TemCommit informa se o commit existe no clone. Serve para detectar o
// histórico reescrito depois da coleta.
func (r *Repo) TemCommit(sha string) bool {
	if sha == "" {
		return false
	}
	_, err := r.git("cat-file", "-e", sha+"^{commit}")
	return err == nil
}

func (r *Repo) git(args ...string) (string, error) {
	return rodar(r.Dir, TempoLimite, args...)
}

// rodar executa o git, com o terminal fora do caminho: pedido de senha ou de
// confirmação de host vira erro, em vez de travar a coleta esperando alguém
// digitar.
func rodar(dir string, limite time.Duration, args ...string) (string, error) {
	ctx, cancelar := context.WithTimeout(context.Background(), limite)
	defer cancelar()

	cmd := exec.CommandContext(ctx, "git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GIT_SSH_COMMAND=ssh -o BatchMode=yes -o StrictHostKeyChecking=accept-new",
	)
	var saida, erros strings.Builder
	cmd.Stdout = &saida
	cmd.Stderr = &erros

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("git %s: tempo esgotado depois de %s", args[0], limite)
		}
		msg := strings.TrimSpace(erros.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), primeiraLinha(msg))
	}
	return saida.String(), nil
}

func primeiraLinha(s string) string {
	if i := strings.IndexByte(s, '\n'); i > 0 {
		return s[:i]
	}
	return s
}
