package runstore

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"time"

	"fraud-detector-jev/internal/domain"
)

// idPattern é a única forma aceita de id. Ele vira nome de pasta, então nada
// fora desse formato (como ../) pode passar.
var idPattern = regexp.MustCompile(`^run_\d{8}_\d{6}_[0-9a-f]{4}$`)

// Store guarda as execuções em disco.
type Store struct{ dir string }

// New cria a pasta se preciso.
func New(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("criando a pasta das execuções: %w", err)
	}
	return &Store{dir: dir}, nil
}

// NewRunID gera um id como run_20260930_143005_ab12.
func (s *Store) NewRunID(now time.Time) string {
	var suffix [2]byte
	_, _ = rand.Read(suffix[:])
	return fmt.Sprintf("run_%s_%s", now.UTC().Format("20060102_150405"), hex.EncodeToString(suffix[:]))
}

func (s *Store) runDir(id string) (string, error) {
	if !idPattern.MatchString(id) {
		return "", fmt.Errorf("id de execução inválido %q: %w", id, domain.ErrNotFound)
	}
	return filepath.Join(s.dir, id), nil
}

// SaveSummary grava o summary.json de forma atômica (arquivo temporário e rename).
func (s *Store) SaveSummary(sum domain.RunSummary) error {
	dir, err := s.runDir(sum.RunID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("criando a pasta da execução: %w", err)
	}
	raw, err := json.MarshalIndent(sum, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(filepath.Join(dir, "summary.json"), raw)
}

// ReadSummary lê o summary.json de uma execução.
func (s *Store) ReadSummary(id string) (domain.RunSummary, error) {
	dir, err := s.runDir(id)
	if err != nil {
		return domain.RunSummary{}, err
	}
	var sum domain.RunSummary
	if err := readJSON(filepath.Join(dir, "summary.json"), &sum); err != nil {
		return domain.RunSummary{}, err
	}
	return sum, nil
}

// List devolve todas as execuções, da mais nova para a mais antiga. Pastas sem
// summary.json ou com arquivo ilegível são ignoradas.
func (s *Store) List() ([]domain.RunSummary, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, fmt.Errorf("listando as execuções: %w", err)
	}
	var out []domain.RunSummary
	for _, e := range entries {
		if !e.IsDir() || !idPattern.MatchString(e.Name()) {
			continue
		}
		sum, err := s.ReadSummary(e.Name())
		if err != nil {
			continue
		}
		out = append(out, sum)
	}
	slices.SortFunc(out, func(a, b domain.RunSummary) int { return b.StartedAt.Compare(a.StartedAt) })
	return out, nil
}

type resultsWriter struct {
	f   *os.File
	w   *bufio.Writer
	enc *json.Encoder
}

// OpenResultsWriter cria o results.jsonl da execução (a pasta precisa existir,
// ou seja, o SaveSummary já rodou).
func (s *Store) OpenResultsWriter(id string) (domain.ResultsSink, error) {
	dir, err := s.runDir(id)
	if err != nil {
		return nil, err
	}
	f, err := os.Create(filepath.Join(dir, "results.jsonl"))
	if err != nil {
		return nil, fmt.Errorf("criando o results.jsonl: %w", err)
	}
	w := bufio.NewWriterSize(f, 1<<20)
	return &resultsWriter{f: f, w: w, enc: json.NewEncoder(w)}, nil
}

func (r *resultsWriter) Write(res domain.Result) error { return r.enc.Encode(res) }

func (r *resultsWriter) Close() error {
	return errors.Join(r.w.Flush(), r.f.Close())
}

// OpenResults abre o results.jsonl para leitura. Quem chama precisa fechar.
func (s *Store) OpenResults(id string) (io.ReadCloser, error) {
	dir, err := s.runDir(id)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(filepath.Join(dir, "results.jsonl"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("results da execução %s: %w", id, domain.ErrNotFound)
	}
	return f, err
}

// MarkStaleRunsFailed marca como failed as execuções que ficaram "running" de
// uma vida anterior do servidor. Devolve quantas mudou.
func (s *Store) MarkStaleRunsFailed(now time.Time) (int, error) {
	lista, err := s.List()
	if err != nil {
		return 0, err
	}
	n := 0
	for _, sum := range lista {
		if sum.Status != domain.StatusRunning {
			continue
		}
		fim := now
		sum.Status = domain.StatusFailed
		sum.Error = "execução interrompida porque o servidor foi reiniciado"
		sum.FinishedAt = &fim
		sum.DurationS = now.Sub(sum.StartedAt).Seconds()
		if err := s.SaveSummary(sum); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// WriteDatasetStats grava o dataset_stats.json (o seeder chama isso).
func (s *Store) WriteDatasetStats(stats domain.DatasetStats) error {
	raw, err := json.MarshalIndent(stats, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(filepath.Join(s.dir, "dataset_stats.json"), raw)
}

// ReadDatasetStats lê o dataset_stats.json. Devolve ErrNotFound se o seeder
// ainda não rodou.
func (s *Store) ReadDatasetStats() (domain.DatasetStats, error) {
	var stats domain.DatasetStats
	if err := readJSON(filepath.Join(s.dir, "dataset_stats.json"), &stats); err != nil {
		return domain.DatasetStats{}, err
	}
	return stats, nil
}

func writeFileAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func readJSON(path string, into any) error {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%s: %w", filepath.Base(path), domain.ErrNotFound)
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, into)
}
