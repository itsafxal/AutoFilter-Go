package mongodb

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"

	"github.com/itsafxal/MYTGBOTZ/internal/config"
)

// ClientManager manages connections to the authoritative MongoDB cluster(s).
type ClientManager struct {
	cfg           *config.Config
	logger        *slog.Logger
	primaryClient *mongo.Client
	client2       *mongo.Client
	client3       *mongo.Client
	primaryDB     *mongo.Database
	db2           *mongo.Database
	db3           *mongo.Database
}

// NewClientManager constructs a new ClientManager.
func NewClientManager(cfg *config.Config, logger *slog.Logger) *ClientManager {
	return &ClientManager{
		cfg:    cfg,
		logger: logger,
	}
}

// Connect establishes connections to configured MongoDB instances and verifies health via ping.
func (m *ClientManager) Connect(ctx context.Context) error {
	if m.cfg.DatabaseURI == "" {
		return errors.New("cannot connect to MongoDB: DATABASE_URI is empty")
	}

	// Connect primary client
	m.logger.Info("connecting to primary MongoDB...",
		"database", m.cfg.DatabaseName,
		"uri", config.MaskURI(m.cfg.DatabaseURI),
	)

	primary, err := m.connectClient(ctx, m.cfg.DatabaseURI)
	if err != nil {
		return fmt.Errorf("failed to connect to primary MongoDB: %w", err)
	}
	m.primaryClient = primary
	m.primaryDB = primary.Database(m.cfg.DatabaseName)

	// Connect secondary client if different from primary
	if m.cfg.DatabaseURI2 != "" && m.cfg.DatabaseURI2 != m.cfg.DatabaseURI {
		m.logger.Info("connecting to secondary MongoDB (DB2)...",
			"database", m.cfg.DatabaseName,
			"uri", config.MaskURI(m.cfg.DatabaseURI2),
		)
		c2, err := m.connectClient(ctx, m.cfg.DatabaseURI2)
		if err != nil {
			m.logger.Warn("failed to connect to secondary MongoDB (DB2); falling back to primary", "error", err)
			m.client2 = primary
			m.db2 = m.primaryDB
		} else {
			m.client2 = c2
			m.db2 = c2.Database(m.cfg.DatabaseName)
		}
	} else {
		m.client2 = primary
		m.db2 = m.primaryDB
	}

	// Connect tertiary client if different from primary
	if m.cfg.DatabaseURI3 != "" && m.cfg.DatabaseURI3 != m.cfg.DatabaseURI && m.cfg.DatabaseURI3 != m.cfg.DatabaseURI2 {
		m.logger.Info("connecting to tertiary MongoDB (DB3)...",
			"database", m.cfg.DatabaseName,
			"uri", config.MaskURI(m.cfg.DatabaseURI3),
		)
		c3, err := m.connectClient(ctx, m.cfg.DatabaseURI3)
		if err != nil {
			m.logger.Warn("failed to connect to tertiary MongoDB (DB3); falling back to primary", "error", err)
			m.client3 = primary
			m.db3 = m.primaryDB
		} else {
			m.client3 = c3
			m.db3 = c3.Database(m.cfg.DatabaseName)
		}
	} else {
		m.client3 = m.client2
		m.db3 = m.db2
	}

	m.logger.Info("MongoDB connection established and verified successfully",
		"database", m.cfg.DatabaseName,
	)
	return nil
}

func (m *ClientManager) connectClient(ctx context.Context, uri string) (*mongo.Client, error) {
	opts := options.Client().
		ApplyURI(uri).
		SetConnectTimeout(15 * time.Second).
		SetServerSelectionTimeout(15 * time.Second).
		SetSocketTimeout(120 * time.Second).
		SetMaxPoolSize(100).
		SetMinPoolSize(5).
		SetRetryReads(true)

	// mongo.Connect binds to this context for the lifecycle of its background monitoring goroutines and connection pool.
	// Using a temporary context that is cancelled on function return breaks active pooled connections with 'context canceled'.
	client, err := mongo.Connect(context.Background(), opts)
	if err != nil {
		return nil, fmt.Errorf("connection error: %w", err)
	}

	// Verify connectivity with a scoped timeout
	pingCtx, pingCancel := context.WithTimeout(ctx, 10*time.Second)
	defer pingCancel()
	if err := client.Ping(pingCtx, readpref.Primary()); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, fmt.Errorf("ping verification failed: %w", err)
	}

	return client, nil
}

// Disconnect gracefully closes all open MongoDB connections.
func (m *ClientManager) Disconnect(ctx context.Context) error {
	var errs []error

	closedClients := make(map[*mongo.Client]struct{})

	for _, c := range []*mongo.Client{m.primaryClient, m.client2, m.client3} {
		if c == nil {
			continue
		}
		if _, seen := closedClients[c]; seen {
			continue
		}
		closedClients[c] = struct{}{}

		disconnectCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		if err := c.Disconnect(disconnectCtx); err != nil {
			errs = append(errs, err)
		}
		cancel()
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors disconnecting MongoDB clients: %v", errs)
	}
	m.logger.Info("MongoDB connections disconnected cleanly")
	return nil
}

// PrimaryDB returns the authoritative primary database.
func (m *ClientManager) PrimaryDB() *mongo.Database {
	return m.primaryDB
}

// DB2 returns the secondary media database (or primary if not distinct).
func (m *ClientManager) DB2() *mongo.Database {
	return m.db2
}

// HasDB2 reports whether a distinct DB2 secondary database was configured and connected.
func (m *ClientManager) HasDB2() bool {
	return m.cfg.DatabaseURI2 != "" && m.cfg.DatabaseURI2 != m.cfg.DatabaseURI && m.db2 != nil && m.db2 != m.primaryDB
}

// DB3 returns the tertiary media database (or primary if not distinct).
func (m *ClientManager) DB3() *mongo.Database {
	return m.db3
}

// HasDB3 reports whether a distinct DB3 tertiary database was configured and connected.
func (m *ClientManager) HasDB3() bool {
	return m.cfg.DatabaseURI3 != "" && m.cfg.DatabaseURI3 != m.cfg.DatabaseURI && m.cfg.DatabaseURI3 != m.cfg.DatabaseURI2 && m.db3 != nil && m.db3 != m.primaryDB && m.db3 != m.db2
}
