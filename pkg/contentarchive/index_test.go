package contentarchive

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// These are unrelated pre-feature rows in an already populated database. The
// separate archive migration must not rename, mutate, or remove their schema.
type indexLegacyOption struct {
	Key   string `gorm:"type:varchar(191);primaryKey"`
	Value string `gorm:"type:text"`
}

func (indexLegacyOption) TableName() string { return "content_archive_test_legacy_options" }

func TestContentArchiveIndexDatabaseMatrix(t *testing.T) {
	cases := []struct{ name, dsn string }{
		{"sqlite", "sqlite://" + filepath.Join(t.TempDir(), "archive.db")},
		{"mysql", os.Getenv("CONTENT_ARCHIVE_TEST_MYSQL_DSN")},
		{"postgres", os.Getenv("CONTENT_ARCHIVE_TEST_POSTGRES_DSN")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.dsn == "" {
				t.Skip("isolated database DSN was not provided")
			}
			index, err := OpenIndex(tc.dsn)
			require.NoError(t, err)
			t.Cleanup(func() { assert.NoError(t, index.Close()) })
			// Only feature-owned tables in an explicitly supplied isolated DB.
			require.NoError(t, index.db.Migrator().DropTable(&archiveEvent{}, &archiveBatch{}, &archiveRequest{}, &indexLegacyOption{}))
			var version string
			versionQuery := "SELECT version()"
			if tc.name == "sqlite" {
				versionQuery = "SELECT sqlite_version()"
			}
			require.NoError(t, index.db.Raw(versionQuery).Scan(&version).Error)
			t.Logf("database version: %s", version)
			ctx := context.Background()
			const now int64 = 1_800_000_000

			t.Run("gateway_and_worker_open_empty_index_concurrently", func(t *testing.T) {
				type opened struct {
					index *Index
					err   error
				}
				start := make(chan struct{})
				results := make(chan opened, 2)
				var workers sync.WaitGroup
				for range 2 {
					workers.Go(func() {
						<-start
						peer, err := OpenIndex(tc.dsn)
						results <- opened{peer, err}
					})
				}
				close(start)
				workers.Wait()
				peers := make([]*Index, 0, 2)
				for range 2 {
					result := <-results
					if result.index != nil {
						t.Cleanup(func() { assert.NoError(t, result.index.Close()) })
						peers = append(peers, result.index)
					}
					require.NoError(t, result.err)
				}
				events := indexFixtureEvents("concurrent-startup", now)
				batch := indexFixtureBatch(t, "concurrent-startup", events)
				require.NoError(t, peers[0].SaveBatch(ctx, batch))
				page, refs, err := peers[1].ListContent(ctx, events[0].RequestID, -1, 10, now)
				require.NoError(t, err)
				assert.Equal(t, "complete", page.State)
				assert.Len(t, refs, 4)
			})

			t.Run("fresh_and_existing_database_migrate_twice", func(t *testing.T) {
				require.NoError(t, index.Migrate())
				require.NoError(t, index.Migrate())
				for _, item := range []struct {
					model any
					name  string
				}{
					{&archiveRequest{}, "idx_ca_request_expiry"}, {&archiveBatch{}, "idx_ca_batch_expiry"},
					{&archiveEvent{}, "idx_ca_event_sequence"}, {&archiveEvent{}, "idx_ca_event_batch"},
				} {
					assert.True(t, index.db.Migrator().HasIndex(item.model, item.name), item.name)
				}
				require.NoError(t, index.db.Migrator().DropTable(&archiveEvent{}, &archiveBatch{}, &archiveRequest{}))
				require.NoError(t, index.db.AutoMigrate(&indexLegacyOption{}))
				options := []indexLegacyOption{{"request_content.enabled", "false"}, {"request_content.retention_days", "7"}, {"request_content.max_body_bytes", "1048576"}, {"existing_account_marker", "unchanged"}}
				require.NoError(t, index.db.Create(&options).Error)
				require.NoError(t, index.Migrate())
				require.NoError(t, index.Migrate())
				var existing []indexLegacyOption
				require.NoError(t, index.db.Find(&existing).Error)
				assert.ElementsMatch(t, options, existing)
			})

			t.Run("segments_visible_before_end_and_replayed_without_duplicates", func(t *testing.T) {
				events := indexFixtureEvents("stream-request", now)
				first := indexFixtureBatch(t, "stream-first", events[:2])
				require.NoError(t, index.ReserveBatch(ctx, first))
				page, refs, err := index.ListContent(ctx, events[0].RequestID, -1, 10, now)
				require.NoError(t, err)
				assert.Empty(t, page.RequestID)
				assert.Empty(t, refs, "reservation must not expose unpersisted content")
				require.NoError(t, index.SaveBatch(ctx, first))
				page, refs, err = index.ListContent(ctx, events[0].RequestID, -1, 10, now)
				require.NoError(t, err)
				assert.Equal(t, "archiving", page.State)
				require.Len(t, refs, 2)
				assert.Equal(t, "stream-first", refs[1].ObjectKey)
				require.NoError(t, index.SaveBatch(ctx, first))
				require.NoError(t, index.SaveBatch(ctx, indexFixtureBatch(t, "stream-overlap", events[1:])))
				page, refs, err = index.ListContent(ctx, events[0].RequestID, -1, 2, now)
				require.NoError(t, err)
				assert.Equal(t, "complete", page.State)
				require.Len(t, refs, 2)
				require.NotNil(t, page.NextSequence)
				assert.Equal(t, int64(1), *page.NextSequence)
				assert.Equal(t, "stream-first", refs[1].ObjectKey, "replay preserves original segment mapping")
				page, refs, err = index.ListContent(ctx, events[0].RequestID, *page.NextSequence, 2, now)
				require.NoError(t, err)
				require.Len(t, refs, 2)
				assert.Equal(t, int64(2), refs[0].Sequence)
				assert.Nil(t, page.NextSequence)
				require.NoError(t, index.Migrate())
				require.NoError(t, index.Migrate())
				var count int64
				require.NoError(t, index.db.Model(&archiveEvent{}).Where("request_key = ?", archiveDigest(events[0].RequestID)).Count(&count).Error)
				assert.Equal(t, int64(4), count)
				var duplicate archiveEvent
				require.NoError(t, index.db.Where("request_key = ?", archiveDigest(events[0].RequestID)).Take(&duplicate).Error)
				duplicate.EventID = archiveDigest("different-primary-same-sequence")
				assert.Error(t, index.db.Create(&duplicate).Error, "sequence uniqueness survives repeated migration")
			})

			t.Run("out_of_order_and_missing_sequences_are_not_complete", func(t *testing.T) {
				events := indexFixtureEvents("out-of-order", now)
				require.NoError(t, index.SaveBatch(ctx, indexFixtureBatch(t, "end-first", events[2:])))
				page, _, err := index.ListContent(ctx, events[0].RequestID, -1, 10, now)
				require.NoError(t, err)
				assert.Equal(t, "partial", page.State)
				require.NoError(t, index.SaveBatch(ctx, indexFixtureBatch(t, "start-later", events[:1])))
				page, _, err = index.ListContent(ctx, events[0].RequestID, -1, 10, now)
				require.NoError(t, err)
				assert.Equal(t, "partial", page.State)
				require.NoError(t, index.SaveBatch(ctx, indexFixtureBatch(t, "missing-data", events[1:2])))
				page, _, err = index.ListContent(ctx, events[0].RequestID, -1, 10, now)
				require.NoError(t, err)
				assert.Equal(t, "complete", page.State)
			})

			t.Run("case_sensitive_identity_and_content_conflicts", func(t *testing.T) {
				upper := indexFixtureEvents("CaseSensitive", now)
				lower := indexFixtureEvents("casesensitive", now)
				for n := range lower {
					lower[n].UserID = 2
				}
				require.NoError(t, index.SaveBatch(ctx, indexFixtureBatch(t, "case-upper", upper)))
				require.NoError(t, index.SaveBatch(ctx, indexFixtureBatch(t, "case-lower", lower)))
				upperPage, _, err := index.ListContent(ctx, upper[0].RequestID, -1, 10, now)
				require.NoError(t, err)
				lowerPage, _, err := index.ListContent(ctx, lower[0].RequestID, -1, 10, now)
				require.NoError(t, err)
				assert.Equal(t, 1, upperPage.UserID)
				assert.Equal(t, 2, lowerPage.UserID)
				conflict := upper[1]
				conflict.Data = []byte("different synthetic body")
				assert.ErrorIs(t, index.SaveBatch(ctx, indexFixtureBatch(t, "conflicting-event", []Event{conflict})), ErrArchiveConflict)
				var count int64
				require.NoError(t, index.db.Model(&archiveBatch{}).Where("batch_id = ?", archiveDigest("conflicting-event")).Count(&count).Error)
				assert.Zero(t, count, "failed visibility transaction must roll back its reservation")
				conflict = upper[1]
				conflict.UserID = 2
				assert.ErrorIs(t, index.SaveBatch(ctx, indexFixtureBatch(t, "conflicting-owner", []Event{conflict})), ErrArchiveConflict)
			})

			t.Run("truncated_and_interrupted_records_remain_partial", func(t *testing.T) {
				for _, field := range []string{"truncated", "incomplete"} {
					events := indexFixtureEvents(field, now)
					events[3].Truncated = field == "truncated"
					events[3].Incomplete = field == "incomplete"
					require.NoError(t, index.SaveBatch(ctx, indexFixtureBatch(t, field, events)))
					page, _, err := index.ListContent(ctx, field, -1, 10, now)
					require.NoError(t, err)
					assert.Equal(t, "partial", page.State)
				}
			})

			t.Run("concurrent_replay_is_idempotent", func(t *testing.T) {
				batch := indexFixtureBatch(t, "concurrent", indexFixtureEvents("concurrent", now))
				start := make(chan struct{})
				results := make(chan error, 2)
				var workers sync.WaitGroup
				for range 2 {
					workers.Go(func() { <-start; results <- index.SaveBatch(ctx, batch) })
				}
				close(start)
				workers.Wait()
				for range 2 {
					require.NoError(t, <-results)
				}
				page, refs, err := index.ListContent(ctx, "concurrent", -1, 10, now)
				require.NoError(t, err)
				assert.Equal(t, "complete", page.State)
				assert.Len(t, refs, 4)
			})

			t.Run("read_expiry_shared_object_cleanup_and_reserved_orphans", func(t *testing.T) {
				short := indexFixtureEvents("short-lived", now)
				long := indexFixtureEvents("long-lived", now)
				for n := range short {
					short[n].ExpiresAt = now + 10
				}
				for n := range long {
					long[n].ExpiresAt = now + 20
				}
				batch := indexFixtureBatch(t, "mixed-expiry", append(short, long...))
				require.NoError(t, index.SaveBatch(ctx, batch))
				page, _, err := index.ListContent(ctx, "short-lived", -1, 10, now+10)
				require.NoError(t, err)
				assert.Empty(t, page.RequestID)
				page, refs, err := index.ListContent(ctx, "long-lived", -1, 10, now+10)
				require.NoError(t, err)
				assert.Equal(t, "complete", page.State)
				assert.Len(t, refs, 4)
				assert.Error(t, index.DeleteBatch(ctx, batch.Key, now+10))
				expired, err := index.ExpiredBatches(ctx, now+10, 100)
				require.NoError(t, err)
				assert.Empty(t, expired)
				orphan := indexFixtureBatch(t, "reserved-only", short)
				require.NoError(t, index.ReserveBatch(ctx, orphan))
				expired, err = index.ExpiredBatches(ctx, now+10, 100)
				require.NoError(t, err)
				require.Len(t, expired, 1)
				assert.Equal(t, orphan.Key, expired[0].Key)
				require.NoError(t, index.DeleteBatch(ctx, orphan.Key, now+10))
				require.NoError(t, index.DeleteBatch(ctx, orphan.Key, now+10))
				require.NoError(t, index.DeleteBatch(ctx, batch.Key, now+20))
				var count int64
				require.NoError(t, index.db.Model(&archiveRequest{}).Where("request_key IN ?", []string{archiveDigest("short-lived"), archiveDigest("long-lived")}).Count(&count).Error)
				assert.Zero(t, count)
			})

			t.Run("metadata_boundary_and_invalid_input", func(t *testing.T) {
				events := indexFixtureEvents("long-model", now)
				for n := range events {
					events[n].Model = strings.Repeat("m", 512)
				}
				require.NoError(t, index.SaveBatch(ctx, indexFixtureBatch(t, "long-model", events)))
				page, _, err := index.ListContent(ctx, "long-model", -1, 10, now)
				require.NoError(t, err)
				assert.Len(t, page.Model, 512)
				bad := indexFixtureBatch(t, "bad-start", indexFixtureEvents("bad", now))
				bad.Events[0].Sequence = 1
				assert.Error(t, index.SaveBatch(ctx, bad))
				_, _, err = index.ListContent(ctx, " bad ", -1, 10, now)
				assert.Error(t, err)
			})
		})
	}
}

func indexFixtureEvents(requestID string, now int64) []Event {
	base := Event{Version: EventVersion, RequestID: requestID, UserID: 1, TokenID: 3, Model: "test-model", Path: "/v1/chat/completions", CreatedAt: now, ExpiresAt: now + 3600}
	events := []Event{base, base, base, base}
	events[0].Kind = EventStart
	events[1].Kind, events[1].Sequence, events[1].Direction, events[1].Data = EventData, 1, DirectionRequest, []byte(`{"messages":[{"role":"user","content":"synthetic input"}]}`)
	events[2].Kind, events[2].Sequence, events[2].Direction, events[2].Data = EventData, 2, DirectionResponse, []byte("data: synthetic response\n\n")
	events[3].Kind, events[3].Sequence, events[3].HTTPStatus, events[3].Stream, events[3].Outcome = EventEnd, 3, 200, true, "completed"
	return events
}

func indexFixtureBatch(t *testing.T, key string, events []Event) StoredBatch {
	t.Helper()
	data, err := common.Marshal(events)
	require.NoError(t, err)
	var expiry int64
	for _, event := range events {
		expiry = max(expiry, event.ExpiresAt)
	}
	return StoredBatch{Key: key, Topic: "index-test", Partition: 0, FirstOffset: 0, LastOffset: int64(len(events) - 1), Checksum: archiveDigest(string(data)), ExpiresAt: expiry, Events: events}
}

func TestContentArchiveIndexUnavailableHandshakeReturns(t *testing.T) {
	for _, engine := range []string{"mysql", "postgres"} {
		t.Run(engine, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			defer listener.Close()
			accepted := make(chan net.Conn, 1)
			go func() {
				conn, err := listener.Accept()
				if err == nil {
					accepted <- conn
				}
			}()
			dsn := "synthetic@tcp(" + listener.Addr().String() + ")/archive?timeout=50ms&readTimeout=50ms&writeTimeout=50ms"
			if engine == "postgres" {
				dsn = "postgres://synthetic@" + listener.Addr().String() + "/archive?sslmode=disable&connect_timeout=1"
			}
			result := make(chan error, 1)
			go func() {
				index, err := OpenIndex(dsn)
				if index != nil {
					_ = index.Close()
				}
				result <- err
			}()
			select {
			case conn := <-accepted:
				defer conn.Close() // Accept TCP but deliberately never handshake.
			case err := <-result:
				t.Fatalf("connection failed before exercising handshake: %v", err)
			case <-time.After(3 * time.Second):
				t.Fatal("driver did not connect to isolated listener")
			}
			select {
			case err := <-result:
				require.Error(t, err)
			case <-time.After(3 * time.Second):
				t.Fatal("archive initialization waited indefinitely for handshake")
			}
		})
	}
}

func TestContentArchiveIndexMigrationHonorsCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "locked-index.db")
	index, err := OpenIndex("sqlite://" + path)
	require.NoError(t, err)
	defer index.Close()
	locker, err := gorm.Open(sqlite.Open(path), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	pool, err := locker.DB()
	require.NoError(t, err)
	defer pool.Close()
	conn, err := pool.Conn(context.Background())
	require.NoError(t, err)
	defer conn.Close()
	_, err = conn.ExecContext(context.Background(), "BEGIN EXCLUSIVE")
	require.NoError(t, err)
	defer conn.ExecContext(context.Background(), "ROLLBACK")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- index.migrate(ctx) }()
	select {
	case err := <-result:
		require.Error(t, err)
	case <-time.After(7 * time.Second):
		t.Fatal("migration exceeded the SQLite lock wait and context bound")
	}
}
