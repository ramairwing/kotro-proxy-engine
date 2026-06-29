package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
)

const bucketName = "sse_cache"

// Store is a bbolt-backed embedded database for cached SSE streams.
// Reads are lock-free at the Bolt layer; writes are serialized by the DB.
type Store struct {
	db   *bolt.DB
	path string
	mu   sync.RWMutex
}

// Open creates or opens the cache database at path.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil && filepath.Dir(path) != "." {
		return nil, fmt.Errorf("cache: mkdir: %w", err)
	}

	db, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: 2 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("cache: open db: %w", err)
	}

	if err := db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(bucketName))
		return err
	}); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("cache: init bucket: %w", err)
	}

	return &Store{db: db, path: path}, nil
}

// Get retrieves a cached SSE stream by semantic key. Returns nil, nil on miss.
func (s *Store) Get(key string) (*Entry, error) {
	var raw []byte
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketName))
		if b == nil {
			return nil
		}
		raw = b.Get([]byte(key))
		return nil
	})
	if err != nil {
		return nil, err
	}
	if raw == nil {
		return nil, nil
	}

	var entry Entry
	if err := json.Unmarshal(raw, &entry); err != nil {
		return nil, fmt.Errorf("cache: corrupt entry: %w", err)
	}
	return &entry, nil
}

// Put asynchronously-safe write of a complete SSE stream entry.
func (s *Store) Put(entry Entry) error {
	raw, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketName))
		return b.Put([]byte(entry.Key), raw)
	})
}

// Close shuts down the embedded database.
func (s *Store) Close() error {
	return s.db.Close()
}

// Path returns the on-disk database file path.
func (s *Store) Path() string { return s.path }
