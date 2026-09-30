package jev

import (
	"context"
	"slices"
	"testing"

	"fraud-detector-jev/internal/infrastructure/jev/jevtest"
)

func TestListModelsEFiltro(t *testing.T) {
	srv := jevtest.New(jevtest.Options{})
	defer srv.Close()

	todos, err := ListModels(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(todos, "llama3:8b") || len(todos) != 4 {
		t.Errorf("todos = %v", todos)
	}
	decisao := DecisionModels(todos)
	if !slices.Equal(decisao, []string{"tev1:4b", "tev1:0.8b", "nimble:latest"}) {
		t.Errorf("modelos de decisão = %v", decisao)
	}
}

func TestListModelsFalhaQuandoOllamaEstaFora(t *testing.T) {
	srv := jevtest.New(jevtest.Options{})
	url := srv.URL
	srv.Close()
	if _, err := ListModels(context.Background(), url); err == nil {
		t.Fatal("esperava erro com o servidor fora do ar")
	}
}
