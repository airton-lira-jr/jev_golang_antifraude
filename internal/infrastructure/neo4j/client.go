package neo4j

import (
	"context"
	"fmt"
	"time"

	neo "github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// database é o banco padrão do Neo4j Community.
const database = "neo4j"

// Options configura a conexão. QueryTimeout vale para as queries de leitura
// de feature (padrão 2s).
type Options struct {
	URI          string
	User         string
	Password     string
	MaxPool      int
	QueryTimeout time.Duration
}

// Client é o acesso ao Neo4j. É seguro para uso concorrente.
type Client struct {
	driver       neo.DriverWithContext
	queryTimeout time.Duration
}

// New conecta e confere a conexão.
func New(ctx context.Context, o Options) (*Client, error) {
	driver, err := neo.NewDriverWithContext(o.URI, neo.BasicAuth(o.User, o.Password, ""), func(c *neo.Config) {
		if o.MaxPool > 0 {
			c.MaxConnectionPoolSize = o.MaxPool
		}
	})
	if err != nil {
		return nil, fmt.Errorf("criando o driver do Neo4j: %w", err)
	}
	if err := driver.VerifyConnectivity(ctx); err != nil {
		_ = driver.Close(ctx)
		return nil, fmt.Errorf("conectando no Neo4j em %s: %w", o.URI, err)
	}
	timeout := o.QueryTimeout
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	return &Client{driver: driver, queryTimeout: timeout}, nil
}

// Close fecha o driver.
func (c *Client) Close(ctx context.Context) error { return c.driver.Close(ctx) }

// Ping confere se o Neo4j está respondendo.
func (c *Client) Ping(ctx context.Context) error { return c.driver.VerifyConnectivity(ctx) }

// read roda uma leitura curta, com timeout no servidor e no cliente.
func (c *Client) read(ctx context.Context, query string, params map[string]any) (*neo.EagerResult, error) {
	ctx, cancel := context.WithTimeout(ctx, c.queryTimeout+time.Second)
	defer cancel()
	return neo.ExecuteQuery(ctx, c.driver, query, params, neo.EagerResultTransformer,
		neo.ExecuteQueryWithDatabase(database),
		neo.ExecuteQueryWithReadersRouting(),
		neo.ExecuteQueryWithTransactionConfig(neo.WithTxTimeout(c.queryTimeout)))
}

// readLong roda uma leitura sem timeout, para contagens e máximos na carga.
func (c *Client) readLong(ctx context.Context, query string) (*neo.EagerResult, error) {
	return neo.ExecuteQuery(ctx, c.driver, query, nil, neo.EagerResultTransformer,
		neo.ExecuteQueryWithDatabase(database),
		neo.ExecuteQueryWithReadersRouting())
}

// write roda uma escrita sem timeout.
func (c *Client) write(ctx context.Context, query string, params map[string]any) (*neo.EagerResult, error) {
	return neo.ExecuteQuery(ctx, c.driver, query, params, neo.EagerResultTransformer,
		neo.ExecuteQueryWithDatabase(database),
		neo.ExecuteQueryWithWritersRouting())
}

// recordInt lê um inteiro do registro e falha se a coluna não existir.
func recordInt(rec *neo.Record, key string) (int64, error) {
	v, ok := rec.Get(key)
	n, isInt := v.(int64)
	if !ok || !isInt {
		return 0, fmt.Errorf("a coluna %s veio ausente ou não é inteiro (%T)", key, v)
	}
	return n, nil
}
