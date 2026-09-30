package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCountDataRows(t *testing.T) {
	casos := map[string]struct {
		conteudo string
		want     int64
	}{
		"com quebra no fim":    {"cabecalho\na\nb\nc\n", 3},
		"sem quebra no fim":    {"cabecalho\na\nb\nc", 3},
		"só o cabeçalho":       {"cabecalho\n", 0},
		"arquivo vazio":        {"", 0},
		"quebras CRLF":         {"cabecalho\r\na\r\nb\r\n", 2},
		"cabeçalho sem quebra": {"cabecalho", 0},
	}
	for nome, c := range casos {
		t.Run(nome, func(t *testing.T) {
			arquivo := filepath.Join(t.TempDir(), "x.csv")
			if err := os.WriteFile(arquivo, []byte(c.conteudo), 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := countDataRows(arquivo)
			if err != nil || got != c.want {
				t.Errorf("countDataRows = %d, %v (esperado %d)", got, err, c.want)
			}
		})
	}
	if _, err := countDataRows("/nao/existe.csv"); err == nil {
		t.Error("arquivo inexistente deveria dar erro")
	}
}
