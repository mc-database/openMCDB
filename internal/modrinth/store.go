package modrinth

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
)

const (
	moduleID              = "modrinth"
	crawlLockID     int32 = 1
	migrationLockID int32 = 2
	cleanupTimeout        = 10 * time.Second
)

var crawlLockNamespace = func() int32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(moduleID))
	return int32(h.Sum32())
}()

var ErrCrawlBusy = errors.New("modrinth: another crawl is running")

type store struct {
	db        *sqlx.DB
	tx        *sqlx.Tx
	lockConn  *sqlx.Conn
	cfg       CrawlConfig
	crawlID   int
	committed bool
	coverage  map[string]int
	samples   map[string][]string

	stageMu sync.Mutex

	versionIDs    map[string]bool
	teamIDs       map[string]bool
	userIDs       map[string]bool
	orgIDs        map[string]bool
	embeddedUsers map[string]apiUser
}

func newStore(db *sqlx.DB, cfg CrawlConfig) *store {
	return &store{
		db: db, cfg: cfg, coverage: make(map[string]int), samples: make(map[string][]string),
		versionIDs: map[string]bool{}, teamIDs: map[string]bool{},
		userIDs: map[string]bool{}, orgIDs: map[string]bool{}, embeddedUsers: map[string]apiUser{},
	}
}

func (s *store) sampleIDs(kind string, ids []string) {
	for _, id := range ids {
		if len(s.samples[kind]) >= 10 {
			break
		}
		s.samples[kind] = append(s.samples[kind], id)
	}
}

func (s *store) remember(set map[string]bool, id string) {
	if id != "" {
		set[id] = true
	}
}

func (s *store) keys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	slices.Sort(out)
	return out
}

func (s *store) begin(ctx context.Context) (err error) {
	conn, err := s.db.Connx(ctx)
	if err != nil {
		return fmt.Errorf("get crawl connection: %w", err)
	}
	s.lockConn = conn
	var locked bool
	if err := conn.QueryRowxContext(ctx, `SELECT pg_try_advisory_lock($1, $2)`, crawlLockNamespace, crawlLockID).Scan(&locked); err != nil {
		s.discardLockConn()
		return fmt.Errorf("lock crawl: %w", err)
	}
	if !locked {
		s.lockConn = nil
		_ = conn.Close()
		return ErrCrawlBusy
	}
	defer func() {
		if err == nil {
			return
		}
		if s.tx != nil {
			_ = s.tx.Rollback()
			s.tx = nil
		}
		if s.crawlID != 0 {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
			_, updateErr := conn.ExecContext(cleanupCtx,
				`UPDATE modrinth.crawls SET end_time = NOW(), error = $1 WHERE crawl_id = $2`, err.Error(), s.crawlID)
			cancel()
			if updateErr != nil {
				err = fmt.Errorf("%w (and crawl status update failed: %v)", err, updateErr)
			}
		}
		if releaseErr := s.releaseLock(); releaseErr != nil {
			err = fmt.Errorf("%w (and crawl lock release failed: %v)", err, releaseErr)
		}
	}()

	if _, err := conn.ExecContext(ctx, `
		UPDATE modrinth.crawls SET end_time = clock_timestamp(), error = 'interrupted: process terminated before the crawl could finish'
		WHERE end_time IS NULL AND error IS NULL`); err != nil {
		return fmt.Errorf("heal interrupted crawl: %w", err)
	}
	if err := conn.QueryRowxContext(ctx,
		`INSERT INTO modrinth.crawls (start_time, scope_facet, max_projects) VALUES (clock_timestamp(), $1, $2) RETURNING crawl_id`, s.cfg.ScopeFacet, s.cfg.MaxProjects).
		Scan(&s.crawlID); err != nil {
		return fmt.Errorf("insert crawl: %w", err)
	}
	if err := s.beginTx(ctx); err != nil {
		return err
	}
	if err := s.createStaging(ctx); err != nil {
		return err
	}
	if err := s.tx.Commit(); err != nil {
		s.tx = nil
		return fmt.Errorf("commit staging setup: %w", err)
	}
	s.tx = nil
	return nil
}

func (s *store) beginTx(ctx context.Context) error {
	tx, err := s.lockConn.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	s.tx = tx
	wm, err := workMemLiteral(s.cfg.WorkMem)
	if err != nil {
		_ = tx.Rollback()
		s.tx = nil
		return err
	}
	if _, err := tx.ExecContext(ctx, `SET LOCAL work_mem = `+wm); err != nil {
		_ = tx.Rollback()
		s.tx = nil
		return fmt.Errorf("set work_mem: %w", err)
	}
	return nil
}

func (s *store) commit(ctx context.Context) error {
	if s.tx == nil {
		return errors.New("commit crawl: no snapshot transaction")
	}
	coverage, err := json.Marshal(s.coverage)
	if err != nil {
		return err
	}
	if _, err := s.tx.ExecContext(ctx, `UPDATE modrinth.crawls SET end_time = clock_timestamp(), error = NULL, coverage = $1 WHERE crawl_id = $2`, string(coverage), s.crawlID); err != nil {
		return fmt.Errorf("complete crawl status: %w", err)
	}
	tx := s.tx
	s.tx = nil
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit crawl snapshot: %w", err)
	}
	s.committed = true
	return nil
}

func (s *store) finish(_ context.Context, crawlErr error) error {
	if s.tx != nil {
		_ = s.tx.Rollback()
		s.tx = nil
	}
	var updateErr error
	if !s.committed && s.crawlID != 0 && s.lockConn != nil {
		if crawlErr == nil {
			crawlErr = errors.New("crawl ended without committing its snapshot")
		}
		cleanupCtx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
		coverage, _ := json.Marshal(s.coverage)
		_, updateErr = s.lockConn.ExecContext(cleanupCtx,
			`UPDATE modrinth.crawls SET end_time = clock_timestamp(), error = $1, coverage = $2 WHERE crawl_id = $3 AND end_time IS NULL`, crawlErr.Error(), string(coverage), s.crawlID)
		cancel()
	}
	return errors.Join(updateErr, s.releaseLock())
}

func (s *store) releaseLock() error {
	conn := s.lockConn
	if conn == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
	_, discardErr := conn.ExecContext(ctx, `DISCARD TEMP`)
	var unlocked bool
	unlockErr := conn.QueryRowxContext(ctx, `SELECT pg_advisory_unlock($1, $2)`, crawlLockNamespace, crawlLockID).Scan(&unlocked)
	cancel()
	s.lockConn = nil
	if discardErr != nil || unlockErr != nil || !unlocked {
		_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		_ = conn.Close()
		if discardErr != nil {
			return fmt.Errorf("discard crawl staging tables: %w", discardErr)
		}
		if unlockErr != nil {
			return fmt.Errorf("unlock crawl: %w", unlockErr)
		}
		return errors.New("unlock crawl: advisory lock was not held")
	}
	if err := conn.Close(); err != nil {
		return fmt.Errorf("close crawl connection: %w", err)
	}
	return nil
}

func (s *store) discardLockConn() {
	if s.lockConn == nil {
		return
	}
	conn := s.lockConn
	s.lockConn = nil
	_ = conn.Raw(func(any) error { return driver.ErrBadConn })
	_ = conn.Close()
}

func (s *store) exec(ctx context.Context, q string, args ...any) (err error) {
	ownTx := s.tx == nil
	if ownTx {
		if err := s.beginTx(ctx); err != nil {
			return err
		}
		tx := s.tx
		defer func() {
			_ = tx.Rollback()
			s.tx = nil
		}()
	}
	if _, err = s.tx.ExecContext(ctx, q, args...); err != nil {
		return err
	}
	if ownTx {
		return s.tx.Commit()
	}
	return nil
}

func sqlInt(n int) string { return strconv.Itoa(n) }

var workMemRe = regexp.MustCompile(`^([0-9]+)(B|kB|MB|GB|TB)?$`)

func workMemLiteral(s string) (string, error) {
	t := strings.TrimSpace(s)
	parts := workMemRe.FindStringSubmatch(t)
	if parts == nil {
		return "", fmt.Errorf("openmcdb/modrinth: invalid work_mem %q (use B, kB, MB, GB, TB or bare kB)", s)
	}
	n, err := strconv.ParseUint(parts[1], 10, 64)
	scale := map[string]uint64{"": 1024, "B": 1, "kB": 1024, "MB": 1 << 20, "GB": 1 << 30, "TB": 1 << 40}[parts[2]]
	const maxBytes = uint64(2147483647) * 1024
	if err != nil || n > maxBytes/scale || n*scale < 64*1024 {
		return "", fmt.Errorf("openmcdb/modrinth: work_mem %q must be between 64kB and 2147483647kB", s)
	}
	return "'" + t + "'", nil
}
