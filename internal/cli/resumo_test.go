package cli

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alexkutzke/gitlab-classroom/internal/store"
)

var atualizar = flag.Bool("atualizar", false, "regrava os arquivos dourados em testdata")

// turmaDeTeste copia a turma fictícia de testdata para um .classroom/
// temporário. Os arquivos ficam soltos em testdata, e não dentro de um
// .classroom/, porque o .gitignore bloqueia esse nome em qualquer lugar.
func turmaDeTeste(t *testing.T) string {
	t.Helper()
	pasta := t.TempDir()
	destino := filepath.Join(pasta, store.Dir)
	if err := os.Mkdir(destino, 0o755); err != nil {
		t.Fatal(err)
	}
	nomes, err := filepath.Glob(filepath.Join("testdata", "*.csv"))
	if err != nil {
		t.Fatal(err)
	}
	for _, origem := range append(nomes, filepath.Join("testdata", "config.toml")) {
		dados, err := os.ReadFile(origem)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(destino, filepath.Base(origem)), dados, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return pasta
}

// executarResumo roda o comando sobre a pasta informada, com o relógio
// congelado, e devolve a saída padrão.
func executarResumo(t *testing.T, pasta string, args ...string) string {
	t.Helper()
	dirAntes, agoraAntes := dirFlag, agora
	t.Cleanup(func() { dirFlag, agora = dirAntes, agoraAntes })
	dirFlag = pasta
	fuso := time.FixedZone("-03", -3*60*60)
	agora = func() time.Time { return time.Date(2026, time.October, 6, 14, 0, 0, 123, fuso) }

	var saida bytes.Buffer
	c := cmdResumo()
	c.SetOut(&saida)
	// Lista nil faz o cobra ler os.Args, que trazem as flags do go test.
	c.SetArgs(append([]string{}, args...))
	if err := c.Execute(); err != nil {
		t.Fatalf("resumo %v: %v", args, err)
	}
	return saida.String()
}

// O arquivo dourado é o que o painel copia para os testes dele. Mudança aqui
// é mudança de contrato: campo novo pode entrar sem mudar a versão, mas campo
// que muda de sentido ou some pede VersaoResumo nova.
func TestResumoJSONConfereComArquivoDourado(t *testing.T) {
	saida := executarResumo(t, turmaDeTeste(t), "--json")

	dourado := filepath.Join("testdata", "resumo.golden.json")
	if *atualizar {
		if err := os.WriteFile(dourado, []byte(saida), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	esperado, err := os.ReadFile(dourado)
	if err != nil {
		t.Fatalf("%v (rode `go test ./internal/cli -run Dourado -atualizar` para criar)", err)
	}
	if saida != string(esperado) {
		t.Errorf("a saída difere de %s:\n%s", dourado, saida)
	}
}

func TestResumoEmTextoTrazUmaLinhaPorExercicio(t *testing.T) {
	saida := executarResumo(t, turmaDeTeste(t))

	if !strings.Contains(saida, "Coleta mais recente: ") {
		t.Errorf("a saída não diz quando foi a coleta:\n%s", saida)
	}
	for _, id := range []string{"html", "css", "trabalho-1"} {
		if !strings.Contains(saida, "\n"+id+" ") {
			t.Errorf("exercício %s sem linha própria:\n%s", id, saida)
		}
	}
	if strings.Contains(saida, "antigo") {
		t.Errorf("exercício arquivado apareceu na saída:\n%s", saida)
	}
}
