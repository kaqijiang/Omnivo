package contentarchive

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

var ErrArchiveConflict = errors.New("conflicting content archive event")

// Index owns a connection pool and tables independent of the gateway's billing
// and usage databases. No plaintext or ciphertext body is stored in these tables.
type Index struct{ db *gorm.DB }

type archiveRequest struct {
	RequestKey   string `gorm:"type:varchar(64);primaryKey"`
	RequestID    string `gorm:"type:varchar(64);not null"`
	UserID       int
	Model        string `gorm:"type:varchar(512)"`
	CreatedAt    int64
	ExpiresAt    int64 `gorm:"index:idx_ca_request_expiry"`
	HTTPStatus   int
	Stream       bool
	Outcome      string `gorm:"type:varchar(64)"`
	Truncated    bool
	Incomplete   bool
	HasStart     bool
	HasEnd       bool
	EndSequence  int64
	LastSequence int64
	EventCount   int64
	State        string `gorm:"type:varchar(16)"`
}

func (archiveRequest) TableName() string { return "content_archive_requests" }

type archiveBatch struct {
	BatchID     string `gorm:"type:varchar(64);primaryKey"`
	ObjectKey   string `gorm:"type:text;not null"`
	Topic       string `gorm:"type:varchar(249)"`
	Partition   int32
	FirstOffset int64
	LastOffset  int64
	Checksum    string `gorm:"type:varchar(64);not null"`
	ExpiresAt   int64  `gorm:"index:idx_ca_batch_expiry"`
}

func (archiveBatch) TableName() string { return "content_archive_batches" }

type archiveEvent struct {
	EventID    string `gorm:"type:varchar(64);primaryKey"`
	RequestKey string `gorm:"type:varchar(64);not null;uniqueIndex:idx_ca_event_sequence,priority:1"`
	Sequence   int64  `gorm:"uniqueIndex:idx_ca_event_sequence,priority:2"`
	BatchID    string `gorm:"type:varchar(64);not null;index:idx_ca_event_batch"`
	EventIndex int
	Digest     string `gorm:"type:varchar(64);not null"`
}

func (archiveEvent) TableName() string { return "content_archive_events" }

type EventReference struct {
	Sequence   int64
	ObjectKey  string
	EventIndex int
	Checksum   string
}

type BatchReference struct {
	Key       string
	Checksum  string
	ExpiresAt int64
}

// OpenIndex requires an explicit DSN. SQLite uses sqlite:// followed by its file
// or URI; it never silently falls back to the application's database.
func OpenIndex(dsn string) (*Index, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, errors.New("content archive index DSN is required")
	}
	var dialect gorm.Dialector
	switch {
	case strings.HasPrefix(dsn, "sqlite://"):
		path := strings.TrimPrefix(dsn, "sqlite://")
		if path == "" {
			return nil, errors.New("content archive SQLite path is required")
		}
		dialect = sqlite.Open(path)
	case strings.HasPrefix(dsn, "postgres://"), strings.HasPrefix(dsn, "postgresql://"):
		parsed, err := url.Parse(dsn)
		if err != nil {
			return nil, errors.New("invalid content archive PostgreSQL DSN")
		}
		query := parsed.Query()
		seconds, err := strconv.Atoi(query.Get("connect_timeout"))
		if err != nil || seconds < 1 || seconds > 5 {
			query.Set("connect_timeout", "5")
		}
		parsed.RawQuery = query.Encode()
		dialect = postgres.New(postgres.Config{DSN: parsed.String(), PreferSimpleProtocol: true})
	default:
		parsed, err := mysqldriver.ParseDSN(dsn)
		if err != nil {
			return nil, errors.New("invalid content archive MySQL DSN")
		}
		for _, timeout := range []*time.Duration{&parsed.Timeout, &parsed.ReadTimeout, &parsed.WriteTimeout} {
			if *timeout <= 0 || *timeout > 5*time.Second {
				*timeout = 5 * time.Second
			}
		}
		dialect = mysql.Open(parsed.FormatDSN())
	}
	db, err := gorm.Open(dialect, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		if db != nil {
			if pool, poolErr := db.DB(); poolErr == nil {
				_ = pool.Close()
			}
		}
		return nil, errors.New("could not open content archive index")
	}
	pool, err := db.DB()
	if err != nil {
		return nil, errors.New("could not initialize content archive index pool")
	}
	pool.SetMaxOpenConns(8)
	pool.SetMaxIdleConns(2)
	pool.SetConnMaxLifetime(5 * time.Minute)
	if db.Dialector.Name() == "sqlite" {
		pool.SetMaxOpenConns(1)
		pool.SetMaxIdleConns(1)
		if err := db.Exec("PRAGMA busy_timeout = 5000").Error; err != nil {
			_ = pool.Close()
			return nil, err
		}
	}
	index := &Index{db: db}
	if err := index.Migrate(); err != nil {
		_ = pool.Close()
		return nil, err
	}
	return index, nil
}

func (i *Index) Close() error {
	pool, err := i.db.DB()
	if err != nil {
		return err
	}
	return pool.Close()
}

func (i *Index) Migrate() error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return i.migrate(ctx)
}

func (i *Index) migrate(ctx context.Context) error {
	// Gateway and worker processes may discover the same empty database at
	// startup. AutoMigrate probes before creating, so its DDL can race. Retry
	// only recognized schema/lock races; never hide permissions, syntax or
	// existing-data constraint failures behind an unconditional retry loop.
	for attempt := range 6 {
		err := i.db.WithContext(ctx).AutoMigrate(&archiveRequest{}, &archiveBatch{}, &archiveEvent{})
		if err == nil || attempt == 5 || !archiveMigrationRace(err) {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		timer := time.NewTimer((50 * time.Millisecond) << attempt)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return ctx.Err()
}

func archiveMigrationRace(err error) bool {
	var mysqlErr *mysqldriver.MySQLError
	if errors.As(err, &mysqlErr) {
		switch mysqlErr.Number {
		case 1050, 1060, 1061: // Table, column, or index was just created.
			return true
		}
		return false
	}
	var postgresErr *pgconn.PgError
	if errors.As(err, &postgresErr) {
		switch postgresErr.Code {
		case "42P07", "42710": // duplicate_table / duplicate_object
			return true
		case "23505":
			// Concurrent CREATE TABLE can race while inserting the implicit row
			// type into pg_type, rather than reporting duplicate_table. Restrict
			// this exception to catalog names, never application unique data.
			return postgresErr.ConstraintName == "pg_type_typname_nsp_index" ||
				postgresErr.ConstraintName == "pg_class_relname_nsp_index"
		}
		return false
	}
	var sqliteErr interface{ Code() int }
	if errors.As(err, &sqliteErr) {
		switch sqliteErr.Code() & 0xff {
		case 5, 6: // SQLITE_BUSY / SQLITE_LOCKED, including extended codes.
			return true
		case 1: // SQLite reports duplicate table/index as SQLITE_ERROR.
			message := strings.ToLower(err.Error())
			return strings.Contains(message, "already exists") &&
				(strings.Contains(message, "content_archive_") || strings.Contains(message, "idx_ca_"))
		}
	}
	return false
}

// ReserveBatch creates no readable content. It inventories an object before its
// upload so crashes between upload and indexing still leave a cleanup record.
func (i *Index) ReserveBatch(ctx context.Context, batch StoredBatch) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	entry, err := batchIndexEntry(batch)
	if err != nil {
		return err
	}
	return i.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return reserveArchiveBatch(tx, entry)
	})
}

// SaveBatch makes persisted objects visible, deduplicating event identities even
// when Kafka replay changes the boundaries of the archived batches.
func (i *Index) SaveBatch(ctx context.Context, batch StoredBatch) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	entry, err := batchIndexEntry(batch)
	if err != nil {
		return err
	}
	byRequest := make(map[string][]int)
	digests := make([]string, len(batch.Events))
	for n, event := range batch.Events {
		data, err := common.Marshal(event)
		if err != nil {
			return err
		}
		digests[n] = archiveDigest(string(data))
		key := archiveDigest(event.RequestID)
		byRequest[key] = append(byRequest[key], n)
	}
	keys := make([]string, 0, len(byRequest))
	for key := range byRequest {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return i.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := reserveArchiveBatch(tx, entry); err != nil {
			return err
		}
		for _, requestKey := range keys {
			positions := byRequest[requestKey]
			first := batch.Events[positions[0]]
			record := archiveRequest{RequestKey: requestKey, RequestID: first.RequestID, UserID: first.UserID,
				CreatedAt: first.CreatedAt, ExpiresAt: first.ExpiresAt, EndSequence: -1, LastSequence: -1, State: "archiving"}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&record).Error; err != nil {
				return err
			}
			query := tx.Where("request_key = ?", requestKey)
			if tx.Dialector.Name() != "sqlite" {
				query = query.Clauses(clause.Locking{Strength: "UPDATE"})
			}
			if err := query.Take(&record).Error; err != nil {
				return err
			}
			sequences := make([]int64, 0, len(positions))
			for _, position := range positions {
				sequences = append(sequences, batch.Events[position].Sequence)
			}
			var existing []archiveEvent
			if err := tx.Where("request_key = ? AND sequence IN ?", requestKey, sequences).Find(&existing).Error; err != nil {
				return err
			}
			seen := make(map[int64]string, len(existing))
			for _, ref := range existing {
				seen[ref.Sequence] = ref.Digest
			}
			refs := make([]archiveEvent, 0, len(positions))
			for _, position := range positions {
				event := batch.Events[position]
				if record.RequestID != event.RequestID || record.UserID != event.UserID ||
					record.CreatedAt != event.CreatedAt || record.ExpiresAt != event.ExpiresAt {
					return ErrArchiveConflict
				}
				if digest, found := seen[event.Sequence]; found {
					if digest != digests[position] {
						return ErrArchiveConflict
					}
					continue
				}
				seen[event.Sequence] = digests[position]
				refs = append(refs, archiveEvent{EventID: archiveDigest(fmt.Sprintf("%s:%d", requestKey, event.Sequence)),
					RequestKey: requestKey, Sequence: event.Sequence, BatchID: entry.BatchID,
					EventIndex: position, Digest: digests[position]})
				if record.HasEnd && event.Sequence > record.EndSequence {
					return ErrArchiveConflict
				}
				if event.Kind == EventStart {
					record.HasStart = true
				}
				if event.Kind == EventEnd {
					if record.HasEnd || record.LastSequence > event.Sequence {
						return ErrArchiveConflict
					}
					record.HasEnd, record.EndSequence = true, event.Sequence
					record.HTTPStatus, record.Outcome = event.HTTPStatus, event.Outcome
				}
				if event.Model != "" {
					record.Model = event.Model
				}
				record.Stream = record.Stream || event.Stream
				record.Truncated = record.Truncated || event.Truncated
				record.Incomplete = record.Incomplete || event.Incomplete
				record.LastSequence = max(record.LastSequence, event.Sequence)
				record.EventCount++
			}
			if len(refs) != 0 {
				if err := tx.CreateInBatches(refs, 100).Error; err != nil {
					return err
				}
			}
			record.State = "archiving"
			if record.HasEnd {
				record.State = "partial"
				if record.HasStart && record.EventCount == record.EndSequence+1 &&
					record.LastSequence == record.EndSequence && !record.Truncated && !record.Incomplete {
					record.State = "complete"
				}
			}
			if err := tx.Save(&record).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// ListContent pages references rather than bodies, enforcing each request's
// deadline even when physical removal of a shared object happens later.
func (i *Index) ListContent(ctx context.Context, requestID string, afterSequence int64, limit int, now int64) (ContentPage, []EventReference, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if requestID == "" || len(requestID) > 64 || strings.TrimSpace(requestID) != requestID || afterSequence < -1 {
		return ContentPage{}, nil, errors.New("invalid content archive query")
	}
	limit = min(max(limit, 1), 128)
	var record archiveRequest
	err := i.db.WithContext(ctx).Where("request_key = ? AND expires_at > ?", archiveDigest(requestID), now).Take(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ContentPage{}, nil, nil
	}
	if err != nil {
		return ContentPage{}, nil, err
	}
	if record.RequestID != requestID {
		return ContentPage{}, nil, ErrArchiveConflict
	}
	page := ContentPage{RequestID: record.RequestID, State: record.State, UserID: record.UserID, Model: record.Model,
		CreatedAt: record.CreatedAt, ExpiresAt: record.ExpiresAt, HTTPStatus: record.HTTPStatus, Stream: record.Stream,
		Outcome: record.Outcome, Truncated: record.Truncated, Incomplete: record.Incomplete, Events: make([]Event, 0)}
	var refs []EventReference
	err = i.db.WithContext(ctx).Table("content_archive_events AS e").
		Select("e.sequence, b.object_key, e.event_index, b.checksum").
		Joins("JOIN content_archive_batches AS b ON b.batch_id = e.batch_id").
		Where("e.request_key = ? AND e.sequence > ? AND b.expires_at > ?", record.RequestKey, afterSequence, now).
		Order("e.sequence ASC").Limit(limit + 1).Scan(&refs).Error
	if err != nil {
		return ContentPage{}, nil, err
	}
	if len(refs) > limit {
		sequence := refs[limit-1].Sequence
		page.NextSequence = &sequence
		refs = refs[:limit]
	}
	return page, refs, nil
}

func (i *Index) ExpiredBatches(ctx context.Context, now int64, limit int) ([]BatchReference, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	var batches []archiveBatch
	err := i.db.WithContext(ctx).Where("expires_at <= ?", now).Order("expires_at ASC").Order("batch_id ASC").
		Limit(min(max(limit, 1), 500)).Find(&batches).Error
	refs := make([]BatchReference, 0, len(batches))
	for _, batch := range batches {
		refs = append(refs, BatchReference{Key: batch.ObjectKey, Checksum: batch.Checksum, ExpiresAt: batch.ExpiresAt})
	}
	return refs, err
}

// DeleteBatch follows successful object deletion (including not-found). Shared
// objects expire at their latest event; individual reads expire earlier.
func (i *Index) DeleteBatch(ctx context.Context, key string, now int64) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	batchID := archiveDigest(key)
	return i.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var batch archiveBatch
		err := tx.Where("batch_id = ?", batchID).Take(&batch).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if batch.ObjectKey != key || batch.ExpiresAt > now {
			return errors.New("content archive batch is not expired")
		}
		var requestKeys []string
		if err := tx.Model(&archiveEvent{}).Distinct("request_key").Where("batch_id = ?", batchID).Pluck("request_key", &requestKeys).Error; err != nil {
			return err
		}
		if err := tx.Where("batch_id = ?", batchID).Delete(&archiveEvent{}).Error; err != nil {
			return err
		}
		if err := tx.Where("batch_id = ?", batchID).Delete(&archiveBatch{}).Error; err != nil {
			return err
		}
		for start := 0; start < len(requestKeys); start += 100 {
			if err := tx.Where("request_key IN ? AND expires_at <= ?", requestKeys[start:min(start+100, len(requestKeys))], now).
				Delete(&archiveRequest{}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func reserveArchiveBatch(tx *gorm.DB, entry archiveBatch) error {
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&entry).Error; err != nil {
		return err
	}
	var existing archiveBatch
	if err := tx.Where("batch_id = ?", entry.BatchID).Take(&existing).Error; err != nil {
		return err
	}
	if existing.ObjectKey != entry.ObjectKey || existing.Checksum != entry.Checksum || existing.Topic != entry.Topic ||
		existing.Partition != entry.Partition || existing.FirstOffset != entry.FirstOffset ||
		existing.LastOffset != entry.LastOffset || existing.ExpiresAt != entry.ExpiresAt {
		return ErrArchiveConflict
	}
	return nil
}

func batchIndexEntry(batch StoredBatch) (archiveBatch, error) {
	checksum, err := hex.DecodeString(batch.Checksum)
	if batch.Key == "" || len(batch.Key) > 1024 || len(batch.Topic) > 249 || batch.Partition < 0 ||
		batch.FirstOffset < 0 || batch.LastOffset < batch.FirstOffset || err != nil || len(checksum) != sha256.Size ||
		len(batch.Events) == 0 || len(batch.Events) > 4096 {
		return archiveBatch{}, errors.New("invalid content archive batch")
	}
	var expiresAt, bodyBytes int64
	for _, event := range batch.Events {
		if err := ValidateEvent(event); err != nil {
			return archiveBatch{}, err
		}
		bodyBytes += int64(len(event.Data))
		if bodyBytes > MaxPlainBytes {
			return archiveBatch{}, errors.New("content archive batch exceeds its limit")
		}
		expiresAt = max(expiresAt, event.ExpiresAt)
	}
	if expiresAt <= 0 || expiresAt != batch.ExpiresAt {
		return archiveBatch{}, errors.New("invalid content archive batch expiry")
	}
	return archiveBatch{BatchID: archiveDigest(batch.Key), ObjectKey: batch.Key, Topic: batch.Topic, Partition: batch.Partition,
		FirstOffset: batch.FirstOffset, LastOffset: batch.LastOffset, Checksum: batch.Checksum, ExpiresAt: batch.ExpiresAt}, nil
}

func archiveDigest(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}
