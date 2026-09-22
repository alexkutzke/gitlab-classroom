package gitlab

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// VarAmbiente é a variável consultada quando o token não vem por opção.
const VarAmbiente = "GITLAB_TOKEN"

// ChaveSecretTool identifica o token no chaveiro do sistema.
var ChaveSecretTool = []string{"service", "gitlab", "user", "alexkutzke"}

// ErrSemToken é devolvido quando nenhuma das fontes tem o token.
var ErrSemToken = errors.New("token de acesso ao GitLab não encontrado")

// Instrucoes explica como fornecer o token. Acompanha ErrSemToken em todas as
// mensagens de erro dos comandos.
const Instrucoes = `Gere um token pessoal em
https://gitlab.com/-/user_settings/personal_access_tokens, com escopo read_api
para os comandos que apenas leem ou api para também publicar devolutiva, e
forneça-o de uma destas formas:

  classroom <comando> --token glpat-...
  export GITLAB_TOKEN=glpat-...
  secret-tool store --label='GitLab classroom' service gitlab user alexkutzke
  token_arquivo = "/caminho/para/o/token" no config.toml

O token nunca é gravado dentro de .classroom/.`

// ResolverToken procura o token nas fontes disponíveis, em ordem de
// prioridade: opção da linha de comando, variável de ambiente, chaveiro do
// sistema e, por último, arquivo indicado na configuração.
func ResolverToken(opcao, arquivoConfig string) (string, error) {
	if t := strings.TrimSpace(opcao); t != "" {
		return t, nil
	}
	if t := strings.TrimSpace(os.Getenv(VarAmbiente)); t != "" {
		return t, nil
	}
	if t, err := doSecretTool(); err == nil && t != "" {
		return t, nil
	}
	if arquivoConfig != "" {
		b, err := os.ReadFile(expandir(arquivoConfig))
		if err != nil {
			return "", fmt.Errorf("lendo o token em %s: %w", arquivoConfig, err)
		}
		if t := strings.TrimSpace(string(b)); t != "" {
			return t, nil
		}
	}
	return "", fmt.Errorf("%w\n\n%s", ErrSemToken, Instrucoes)
}

// doSecretTool consulta o chaveiro. Ausência do programa ou da chave não é
// erro: é só mais uma fonte que não tinha o token.
func doSecretTool() (string, error) {
	caminho, err := exec.LookPath("secret-tool")
	if err != nil {
		return "", err
	}
	args := append([]string{"lookup"}, ChaveSecretTool...)
	saida, err := exec.Command(caminho, args...).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(saida)), nil
}

func expandir(caminho string) string {
	if strings.HasPrefix(caminho, "~/") {
		if casa, err := os.UserHomeDir(); err == nil {
			return casa + caminho[1:]
		}
	}
	return caminho
}
