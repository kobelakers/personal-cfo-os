package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type SQLiteStoreConfig struct {
	DSN string
}

type SQLiteMemoryDB struct {
	db  *sql.DB
	dsn string
}

type SQLiteMemoryStores struct {
	DB          *SQLiteMemoryDB
	Store       MemoryStore
	Query       MemoryQueryStore
	Relations   MemoryRelationStore
	Audit       MemoryAuditStore
	WriteEvents MemoryWriteEventStore
	Embeddings  MemoryEmbeddingStore
}

type sqliteMemoryStore struct {
	db *SQLiteMemoryDB
}

func NewSQLiteMemoryStores(config SQLiteStoreConfig) (*SQLiteMemoryStores, error) {
	db, err := NewSQLiteMemoryDB(config)
	if err != nil {
		return nil, err
	}
	store := &sqliteMemoryStore{db: db}
	return &SQLiteMemoryStores{
		DB:          db,
		Store:       store,
		Query:       store,
		Relations:   store,
		Audit:       store,
		WriteEvents: store,
		Embeddings:  store,
	}, nil
}

func NewSQLiteMemoryDB(config SQLiteStoreConfig) (*SQLiteMemoryDB, error) {
	if strings.TrimSpace(config.DSN) == "" {
		return nil, fmt.Errorf("sqlite memory store requires injected dsn/path")
	}
	if strings.HasPrefix(config.DSN, "file:") {
		if cut, _, ok := strings.Cut(strings.TrimPrefix(config.DSN, "file:"), "?"); ok {
			if cut != "" && cut != ":memory:" {
				if err := os.MkdirAll(filepath.Dir(cut), 0o755); err != nil {
					return nil, err
				}
			}
		}
	} else if config.DSN != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(config.DSN), 0o755); err != nil {
			return nil, err
		}
	}
	db, err := sql.Open("sqlite", config.DSN)
	if err != nil {
		return nil, err
	}
	store := &SQLiteMemoryDB{db: db, dsn: config.DSN}
	if _, err := db.Exec(`PRAGMA journal_mode = WAL;`); err != nil {
		_ = db.Close()
		return nil, err
	}
	if _, err := db.Exec(`PRAGMA busy_timeout = 5000;`); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := store.EnsureSchema(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (db *SQLiteMemoryDB) Close() error {
	if db == nil || db.db == nil {
		return nil
	}
	return db.db.Close()
}

func (db *SQLiteMemoryDB) EnsureSchema() error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS memory_records (
			memory_id TEXT PRIMARY KEY,
			kind TEXT NOT NULL,
			summary TEXT NOT NULL,
			facts_json TEXT NOT NULL,
			source_json TEXT NOT NULL,
			confidence_json TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			search_text TEXT NOT NULL,
			token_count INTEGER NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_memory_records_kind_updated
			ON memory_records(kind, updated_at DESC);`,
		`CREATE TABLE IF NOT EXISTS memory_relations (
			memory_id TEXT NOT NULL,
			relation_type TEXT NOT NULL,
			target_memory_id TEXT NOT NULL,
			description TEXT,
			PRIMARY KEY (memory_id, relation_type, target_memory_id)
		);`,
		`CREATE TABLE IF NOT EXISTS memory_embeddings (
			memory_id TEXT NOT NULL,
			provider TEXT NOT NULL,
			model TEXT NOT NULL,
			vector_json TEXT NOT NULL,
			dimensions INTEGER NOT NULL,
			embedded_at TEXT NOT NULL,
			content_hash TEXT,
			PRIMARY KEY (memory_id, model)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_memory_embeddings_model
			ON memory_embeddings(model);`,
		`CREATE TABLE IF NOT EXISTS memory_terms (
			memory_id TEXT NOT NULL,
			term TEXT NOT NULL,
			term_freq INTEGER NOT NULL,
			PRIMARY KEY (memory_id, term)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_memory_terms_term
			ON memory_terms(term);`,
		`CREATE TABLE IF NOT EXISTS memory_access_audit (
			audit_id TEXT PRIMARY KEY,
			memory_id TEXT NOT NULL,
			workflow_id TEXT,
			task_id TEXT,
			trace_id TEXT,
			query_id TEXT,
			accessor TEXT NOT NULL,
			purpose TEXT NOT NULL,
			action TEXT NOT NULL,
			reason TEXT,
			score REAL,
			accessed_at TEXT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_memory_access_audit_memory_time
			ON memory_access_audit(memory_id, accessed_at DESC);`,
		`CREATE TABLE IF NOT EXISTS memory_write_events (
			event_id TEXT PRIMARY KEY,
			memory_id TEXT NOT NULL,
			workflow_id TEXT,
			task_id TEXT,
			trace_id TEXT,
			action TEXT NOT NULL,
			summary TEXT,
			provider TEXT,
			model TEXT,
			occurred_at TEXT NOT NULL,
			details_json TEXT
		);`,
		`CREATE INDEX IF NOT EXISTS idx_memory_write_events_memory_time
			ON memory_write_events(memory_id, occurred_at DESC);`,
	}
	for _, stmt := range statements {
		if _, err := db.db.Exec(stmt); err != nil {
			return err
		}
	}
	_, err := db.db.Exec(`INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (1, ?)`, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

func (s *sqliteMemoryStore) Put(ctx context.Context, record MemoryRecord) error {
	if err := record.Validate(); err != nil {
		return err
	}
	factsJSON, err := json.Marshal(record.Facts)
	if err != nil {
		return err
	}
	sourceJSON, err := json.Marshal(record.Source)
	if err != nil {
		return err
	}
	confidenceJSON, err := json.Marshal(record.Confidence)
	if err != nil {
		return err
	}
	searchText := semanticText(record)
	_, err = s.db.db.ExecContext(ctx, `
		INSERT INTO memory_records(memory_id, kind, summary, facts_json, source_json, confidence_json, created_at, updated_at, search_text, token_count)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(memory_id) DO UPDATE SET
			kind = excluded.kind,
			summary = excluded.summary,
			facts_json = excluded.facts_json,
			source_json = excluded.source_json,
			confidence_json = excluded.confidence_json,
			created_at = excluded.created_at,
			updated_at = excluded.updated_at,
			search_text = excluded.search_text,
			token_count = excluded.token_count
	`, record.ID, string(record.Kind), record.Summary, string(factsJSON), string(sourceJSON), string(confidenceJSON), record.CreatedAt.Format(time.RFC3339Nano), record.UpdatedAt.Format(time.RFC3339Nano), searchText, len(tokenize(searchText)))
	return err
}

func (s *sqliteMemoryStore) Get(ctx context.Context, id string) (MemoryRecord, bool, error) {
	row := s.db.db.QueryRowContext(ctx, `
		SELECT memory_id, kind, summary, facts_json, source_json, confidence_json, created_at, updated_at
		FROM memory_records
		WHERE memory_id = ?
	`, id)
	record, ok, err := scanMemoryRecord(row)
	if err != nil || !ok {
		return MemoryRecord{}, ok, err
	}
	relations, err := s.LoadRelations(ctx, id)
	if err != nil {
		return MemoryRecord{}, false, err
	}
	record.Relations = relations.Relations
	record.Supersedes = relations.Supersedes
	record.Conflicts = relations.Conflicts
	return record, true, nil
}

func (s *sqliteMemoryStore) List(ctx context.Context) ([]MemoryRecord, error) {
	rows, err := s.db.db.QueryContext(ctx, `
		SELECT memory_id, kind, summary, facts_json, source_json, confidence_json, created_at, updated_at
		FROM memory_records
		ORDER BY updated_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := make([]MemoryRecord, 0)
	for rows.Next() {
		record, err := scanMemoryRecordFromRows(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return s.hydrateRelations(ctx, records)
}

func (s *sqliteMemoryStore) LoadByIDs(ctx context.Context, ids []string) ([]MemoryRecord, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	query, args := inClauseQuery(`
		SELECT memory_id, kind, summary, facts_json, source_json, confidence_json, created_at, updated_at
		FROM memory_records
		WHERE memory_id IN (%s)
	`, ids)
	rows, err := s.db.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := make([]MemoryRecord, 0, len(ids))
	for rows.Next() {
		record, err := scanMemoryRecordFromRows(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	records, err = s.hydrateRelations(ctx, records)
	if err != nil {
		return nil, err
	}
	order := make(map[string]int, len(ids))
	for i, id := range ids {
		order[id] = i
	}
	sort.Slice(records, func(i, j int) bool {
		return order[records[i].ID] < order[records[j].ID]
	})
	return records, nil
}

func (s *sqliteMemoryStore) ListByKind(ctx context.Context, filter MemoryListFilter) ([]MemoryRecord, error) {
	base := `
		SELECT memory_id, kind, summary, facts_json, source_json, confidence_json, created_at, updated_at
		FROM memory_records
	`
	args := make([]any, 0)
	where := make([]string, 0)
	if len(filter.Kinds) > 0 {
		kindArgs := make([]string, 0, len(filter.Kinds))
		for _, kind := range filter.Kinds {
			kindArgs = append(kindArgs, string(kind))
		}
		clause, clauseArgs := inClauseWhere("kind", kindArgs)
		where = append(where, clause)
		args = append(args, clauseArgs...)
	}
	if len(where) > 0 {
		base += " WHERE " + strings.Join(where, " AND ")
	}
	if filter.Recent {
		base += " ORDER BY updated_at DESC"
	}
	if filter.Limit > 0 {
		base += fmt.Sprintf(" LIMIT %d", filter.Limit)
	}
	rows, err := s.db.db.QueryContext(ctx, base, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := make([]MemoryRecord, 0)
	for rows.Next() {
		record, err := scanMemoryRecordFromRows(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return s.hydrateRelations(ctx, records)
}

func (s *sqliteMemoryStore) ListRecent(ctx context.Context, limit int) ([]MemoryRecord, error) {
	rows, err := s.db.db.QueryContext(ctx, `
		SELECT memory_id, kind, summary, facts_json, source_json, confidence_json, created_at, updated_at
		FROM memory_records
		ORDER BY updated_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := make([]MemoryRecord, 0)
	for rows.Next() {
		record, err := scanMemoryRecordFromRows(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return s.hydrateRelations(ctx, records)
}

func (s *sqliteMemoryStore) SearchLexical(ctx context.Context, terms []string, limit int) ([]LexicalCandidate, error) {
	terms = dedupeTokens(terms)
	if len(terms) == 0 {
		return nil, nil
	}
	var docCount int
	var avgTokenCount sql.NullFloat64
	if err := s.db.db.QueryRowContext(ctx, `SELECT COUNT(*), AVG(token_count) FROM memory_records`).Scan(&docCount, &avgTokenCount); err != nil {
		return nil, err
	}
	if docCount == 0 {
		return nil, nil
	}
	query, args := inClauseQuery(`
		SELECT t.memory_id, t.term, t.term_freq, r.token_count
		FROM memory_terms t
		JOIN memory_records r ON r.memory_id = t.memory_id
		WHERE t.term IN (%s)
	`, terms)
	rows, err := s.db.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type posting struct {
		memoryID   string
		term       string
		termFreq   int
		tokenCount int
	}
	postings := make([]posting, 0)
	docFreq := make(map[string]int)
	seenDocs := make(map[string]map[string]struct{})
	for rows.Next() {
		var item posting
		if err := rows.Scan(&item.memoryID, &item.term, &item.termFreq, &item.tokenCount); err != nil {
			return nil, err
		}
		postings = append(postings, item)
		if seenDocs[item.term] == nil {
			seenDocs[item.term] = make(map[string]struct{})
		}
		if _, ok := seenDocs[item.term][item.memoryID]; !ok {
			seenDocs[item.term][item.memoryID] = struct{}{}
			docFreq[item.term]++
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	avgdl := avgTokenCount.Float64
	if avgdl == 0 {
		avgdl = 1
	}
	type scored struct {
		score float64
		terms []string
	}
	candidates := make(map[string]scored)
	for _, item := range postings {
		df := float64(docFreq[item.term])
		if df == 0 {
			continue
		}
		idf := math.Log(1 + (float64(docCount)-df+0.5)/(df+0.5))
		tf := float64(item.termFreq)
		dl := float64(maxInt(item.tokenCount, 1))
		k1 := 1.2
		b := 0.75
		termScore := idf * ((tf * (k1 + 1)) / (tf + k1*(1-b+b*(dl/avgdl))))
		existing := candidates[item.memoryID]
		existing.score += termScore
		existing.terms = append(existing.terms, item.term)
		candidates[item.memoryID] = existing
	}
	results := make([]LexicalCandidate, 0, len(candidates))
	for memoryID, item := range candidates {
		results = append(results, LexicalCandidate{
			MemoryID:     memoryID,
			Score:        roundTo(item.score, 4),
			MatchedTerms: dedupeTokens(item.terms),
		})
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].Score == results[j].Score {
			return results[i].MemoryID < results[j].MemoryID
		}
		return results[i].Score > results[j].Score
	})
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

func (s *sqliteMemoryStore) SaveRelations(ctx context.Context, memoryID string, relations MemoryRelations) error {
	tx, err := s.db.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	if _, err = tx.ExecContext(ctx, `DELETE FROM memory_relations WHERE memory_id = ?`, memoryID); err != nil {
		return err
	}
	for _, relation := range relations.Relations {
		if _, err = tx.ExecContext(ctx, `INSERT INTO memory_relations(memory_id, relation_type, target_memory_id, description) VALUES (?, ?, ?, ?)`, memoryID, "relation:"+relation.Type, relation.TargetMemoryID, relation.Description); err != nil {
			return err
		}
	}
	for _, relation := range relations.Supersedes {
		if _, err = tx.ExecContext(ctx, `INSERT INTO memory_relations(memory_id, relation_type, target_memory_id, description) VALUES (?, ?, ?, ?)`, memoryID, "supersedes", relation.MemoryID, relation.Reason); err != nil {
			return err
		}
	}
	for _, relation := range relations.Conflicts {
		if _, err = tx.ExecContext(ctx, `INSERT INTO memory_relations(memory_id, relation_type, target_memory_id, description) VALUES (?, ?, ?, ?)`, memoryID, "conflicts_with", relation.MemoryID, relation.Reason); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *sqliteMemoryStore) LoadRelations(ctx context.Context, memoryID string) (MemoryRelations, error) {
	rows, err := s.db.db.QueryContext(ctx, `
		SELECT relation_type, target_memory_id, description
		FROM memory_relations
		WHERE memory_id = ?
		ORDER BY relation_type, target_memory_id
	`, memoryID)
	if err != nil {
		return MemoryRelations{}, err
	}
	defer rows.Close()
	result := MemoryRelations{}
	for rows.Next() {
		var relationType, targetMemoryID, description string
		if err := rows.Scan(&relationType, &targetMemoryID, &description); err != nil {
			return MemoryRelations{}, err
		}
		switch {
		case strings.HasPrefix(relationType, "relation:"):
			result.Relations = append(result.Relations, MemoryRelation{
				Type:           strings.TrimPrefix(relationType, "relation:"),
				TargetMemoryID: targetMemoryID,
				Description:    description,
			})
		case relationType == "supersedes":
			result.Supersedes = append(result.Supersedes, SupersedesRef{MemoryID: targetMemoryID, Reason: description})
		case relationType == "conflicts_with":
			result.Conflicts = append(result.Conflicts, ConflictRef{MemoryID: targetMemoryID, Reason: description})
		}
	}
	return result, rows.Err()
}

func (s *sqliteMemoryStore) AppendAccess(ctx context.Context, audit MemoryAccessAudit) error {
	_, err := s.db.db.ExecContext(ctx, `
		INSERT INTO memory_access_audit(audit_id, memory_id, workflow_id, task_id, trace_id, query_id, accessor, purpose, action, reason, score, accessed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, audit.ID, audit.MemoryID, audit.WorkflowID, audit.TaskID, audit.TraceID, audit.QueryID, audit.Accessor, audit.Purpose, audit.Action, audit.Reason, audit.Score, audit.AccessedAt.Format(time.RFC3339Nano))
	return err
}

func (s *sqliteMemoryStore) ListAccess(ctx context.Context, memoryID string) ([]MemoryAccessAudit, error) {
	rows, err := s.db.db.QueryContext(ctx, `
		SELECT audit_id, memory_id, workflow_id, task_id, trace_id, query_id, accessor, purpose, action, reason, score, accessed_at
		FROM memory_access_audit
		WHERE memory_id = ?
		ORDER BY accessed_at DESC
	`, memoryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := make([]MemoryAccessAudit, 0)
	for rows.Next() {
		var entry MemoryAccessAudit
		var accessedAt string
		if err := rows.Scan(&entry.ID, &entry.MemoryID, &entry.WorkflowID, &entry.TaskID, &entry.TraceID, &entry.QueryID, &entry.Accessor, &entry.Purpose, &entry.Action, &entry.Reason, &entry.Score, &accessedAt); err != nil {
			return nil, err
		}
		entry.AccessedAt, err = time.Parse(time.RFC3339Nano, accessedAt)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

func (s *sqliteMemoryStore) AppendWriteEvent(ctx context.Context, event MemoryWriteEvent) error {
	detailsJSON, err := json.Marshal(event.Details)
	if err != nil {
		return err
	}
	_, err = s.db.db.ExecContext(ctx, `
		INSERT INTO memory_write_events(event_id, memory_id, workflow_id, task_id, trace_id, action, summary, provider, model, occurred_at, details_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, event.ID, event.MemoryID, event.WorkflowID, event.TaskID, event.TraceID, event.Action, event.Summary, event.Provider, event.Model, event.OccurredAt.Format(time.RFC3339Nano), string(detailsJSON))
	return err
}

func (s *sqliteMemoryStore) ListWriteEvents(ctx context.Context, memoryID string) ([]MemoryWriteEvent, error) {
	rows, err := s.db.db.QueryContext(ctx, `
		SELECT event_id, memory_id, workflow_id, task_id, trace_id, action, summary, provider, model, occurred_at, details_json
		FROM memory_write_events
		WHERE memory_id = ?
		ORDER BY occurred_at DESC
	`, memoryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := make([]MemoryWriteEvent, 0)
	for rows.Next() {
		var item MemoryWriteEvent
		var occurredAt string
		var detailsJSON string
		if err := rows.Scan(&item.ID, &item.MemoryID, &item.WorkflowID, &item.TaskID, &item.TraceID, &item.Action, &item.Summary, &item.Provider, &item.Model, &occurredAt, &detailsJSON); err != nil {
			return nil, err
		}
		item.OccurredAt, err = time.Parse(time.RFC3339Nano, occurredAt)
		if err != nil {
			return nil, err
		}
		item.Details = detailsJSON
		events = append(events, item)
	}
	return events, rows.Err()
}

func (s *sqliteMemoryStore) SaveEmbedding(ctx context.Context, record MemoryEmbeddingRecord) error {
	vectorJSON, err := json.Marshal(record.Vector)
	if err != nil {
		return err
	}
	_, err = s.db.db.ExecContext(ctx, `
		INSERT INTO memory_embeddings(memory_id, provider, model, vector_json, dimensions, embedded_at, content_hash)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(memory_id, model) DO UPDATE SET
			provider = excluded.provider,
			vector_json = excluded.vector_json,
			dimensions = excluded.dimensions,
			embedded_at = excluded.embedded_at,
			content_hash = excluded.content_hash
	`, record.MemoryID, record.Provider, record.Model, string(vectorJSON), record.Dimensions, record.EmbeddedAt.Format(time.RFC3339Nano), record.ContentHash)
	return err
}

func (s *sqliteMemoryStore) LoadEmbedding(ctx context.Context, memoryID string, model string) (MemoryEmbeddingRecord, bool, error) {
	row := s.db.db.QueryRowContext(ctx, `
		SELECT memory_id, provider, model, vector_json, dimensions, embedded_at, content_hash
		FROM memory_embeddings
		WHERE memory_id = ? AND model = ?
	`, memoryID, model)
	return scanEmbedding(row)
}

func (s *sqliteMemoryStore) ListEmbeddings(ctx context.Context, model string) ([]MemoryEmbeddingRecord, error) {
	query := `
		SELECT memory_id, provider, model, vector_json, dimensions, embedded_at, content_hash
		FROM memory_embeddings
	`
	args := make([]any, 0, 1)
	if strings.TrimSpace(model) != "" {
		query += ` WHERE model = ?`
		args = append(args, model)
	}
	rows, err := s.db.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := make([]MemoryEmbeddingRecord, 0)
	for rows.Next() {
		record, _, err := scanEmbeddingFromRows(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func (s *sqliteMemoryStore) DeleteEmbeddings(ctx context.Context, model string) error {
	if strings.TrimSpace(model) == "" {
		_, err := s.db.db.ExecContext(ctx, `DELETE FROM memory_embeddings`)
		return err
	}
	_, err := s.db.db.ExecContext(ctx, `DELETE FROM memory_embeddings WHERE model = ?`, model)
	return err
}

func (s *sqliteMemoryStore) ReplaceTerms(ctx context.Context, memoryID string, terms map[string]int) error {
	tx, err := s.db.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	if _, err = tx.ExecContext(ctx, `DELETE FROM memory_terms WHERE memory_id = ?`, memoryID); err != nil {
		return err
	}
	for term, freq := range terms {
		if strings.TrimSpace(term) == "" || freq <= 0 {
			continue
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO memory_terms(memory_id, term, term_freq) VALUES (?, ?, ?)`, memoryID, term, freq); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *sqliteMemoryStore) LoadIndexedModels(ctx context.Context) ([]string, error) {
	rows, err := s.db.db.QueryContext(ctx, `SELECT DISTINCT model FROM memory_embeddings ORDER BY model`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	models := make([]string, 0)
	for rows.Next() {
		var model string
		if err := rows.Scan(&model); err != nil {
			return nil, err
		}
		models = append(models, model)
	}
	return models, rows.Err()
}

func (s *sqliteMemoryStore) hydrateRelations(ctx context.Context, records []MemoryRecord) ([]MemoryRecord, error) {
	for i := range records {
		relations, err := s.LoadRelations(ctx, records[i].ID)
		if err != nil {
			return nil, err
		}
		records[i].Relations = relations.Relations
		records[i].Supersedes = relations.Supersedes
		records[i].Conflicts = relations.Conflicts
	}
	return records, nil
}

func scanMemoryRecord(row *sql.Row) (MemoryRecord, bool, error) {
	var (
		record       MemoryRecord
		kind         string
		factsJSON    string
		sourceJSON   string
		confJSON     string
		createdAtRaw string
		updatedAtRaw string
	)
	if err := row.Scan(&record.ID, &kind, &record.Summary, &factsJSON, &sourceJSON, &confJSON, &createdAtRaw, &updatedAtRaw); err != nil {
		if err == sql.ErrNoRows {
			return MemoryRecord{}, false, nil
		}
		return MemoryRecord{}, false, err
	}
	record.Kind = MemoryKind(kind)
	if err := json.Unmarshal([]byte(factsJSON), &record.Facts); err != nil {
		return MemoryRecord{}, false, err
	}
	if err := json.Unmarshal([]byte(sourceJSON), &record.Source); err != nil {
		return MemoryRecord{}, false, err
	}
	if err := json.Unmarshal([]byte(confJSON), &record.Confidence); err != nil {
		return MemoryRecord{}, false, err
	}
	var err error
	record.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAtRaw)
	if err != nil {
		return MemoryRecord{}, false, err
	}
	record.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAtRaw)
	if err != nil {
		return MemoryRecord{}, false, err
	}
	return record, true, nil
}

func scanMemoryRecordFromRows(rows *sql.Rows) (MemoryRecord, error) {
	var (
		record       MemoryRecord
		kind         string
		factsJSON    string
		sourceJSON   string
		confJSON     string
		createdAtRaw string
		updatedAtRaw string
	)
	if err := rows.Scan(&record.ID, &kind, &record.Summary, &factsJSON, &sourceJSON, &confJSON, &createdAtRaw, &updatedAtRaw); err != nil {
		return MemoryRecord{}, err
	}
	record.Kind = MemoryKind(kind)
	if err := json.Unmarshal([]byte(factsJSON), &record.Facts); err != nil {
		return MemoryRecord{}, err
	}
	if err := json.Unmarshal([]byte(sourceJSON), &record.Source); err != nil {
		return MemoryRecord{}, err
	}
	if err := json.Unmarshal([]byte(confJSON), &record.Confidence); err != nil {
		return MemoryRecord{}, err
	}
	var err error
	record.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAtRaw)
	if err != nil {
		return MemoryRecord{}, err
	}
	record.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAtRaw)
	if err != nil {
		return MemoryRecord{}, err
	}
	return record, nil
}

func scanEmbedding(row *sql.Row) (MemoryEmbeddingRecord, bool, error) {
	var (
		record       MemoryEmbeddingRecord
		vectorJSON   string
		embeddedAt   string
	)
	if err := row.Scan(&record.MemoryID, &record.Provider, &record.Model, &vectorJSON, &record.Dimensions, &embeddedAt, &record.ContentHash); err != nil {
		if err == sql.ErrNoRows {
			return MemoryEmbeddingRecord{}, false, nil
		}
		return MemoryEmbeddingRecord{}, false, err
	}
	if err := json.Unmarshal([]byte(vectorJSON), &record.Vector); err != nil {
		return MemoryEmbeddingRecord{}, false, err
	}
	var err error
	record.EmbeddedAt, err = time.Parse(time.RFC3339Nano, embeddedAt)
	if err != nil {
		return MemoryEmbeddingRecord{}, false, err
	}
	return record, true, nil
}

func scanEmbeddingFromRows(rows *sql.Rows) (MemoryEmbeddingRecord, bool, error) {
	var (
		record     MemoryEmbeddingRecord
		vectorJSON string
		embeddedAt string
	)
	if err := rows.Scan(&record.MemoryID, &record.Provider, &record.Model, &vectorJSON, &record.Dimensions, &embeddedAt, &record.ContentHash); err != nil {
		return MemoryEmbeddingRecord{}, false, err
	}
	if err := json.Unmarshal([]byte(vectorJSON), &record.Vector); err != nil {
		return MemoryEmbeddingRecord{}, false, err
	}
	var err error
	record.EmbeddedAt, err = time.Parse(time.RFC3339Nano, embeddedAt)
	if err != nil {
		return MemoryEmbeddingRecord{}, false, err
	}
	return record, true, nil
}

func inClauseQuery(format string, ids []string) (string, []any) {
	placeholders := make([]string, 0, len(ids))
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		placeholders = append(placeholders, "?")
		args = append(args, id)
	}
	return fmt.Sprintf(format, strings.Join(placeholders, ",")), args
}

func inClauseWhere(column string, values []string) (string, []any) {
	placeholders := make([]string, 0, len(values))
	args := make([]any, 0, len(values))
	for _, value := range values {
		placeholders = append(placeholders, "?")
		args = append(args, value)
	}
	return fmt.Sprintf("%s IN (%s)", column, strings.Join(placeholders, ",")), args
}
