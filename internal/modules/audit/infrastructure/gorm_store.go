package infrastructure

import (
	"context"
	"strings"

	"github.com/kaoqy/Nodeloc-Store/internal/modules/audit/contract"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/audit/domain"
	"gorm.io/gorm"
)

var _ contract.AuditRepo = (*GormStore)(nil)

// GormStore persists audit logs using GORM.
type GormStore struct {
	db *gorm.DB
}

func NewGormStore(db *gorm.DB) *GormStore {
	if db == nil {
		panic("audit: nil gorm database")
	}
	return &GormStore{db: db}
}

func (s *GormStore) Create(ctx context.Context, log *domain.AuditLog) error {
	return s.db.WithContext(ctx).Create(log).Error
}

func (s *GormStore) List(ctx context.Context, filter domain.LogFilter) ([]domain.AuditLog, int64, error) {
	query := s.filtered(ctx, filter)

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	logs := make([]domain.AuditLog, 0)
	offset := (filter.Page - 1) * filter.Limit
	if err := query.Order("created_at DESC, id DESC").Offset(offset).Limit(filter.Limit).Find(&logs).Error; err != nil {
		return nil, 0, err
	}
	s.attachActorNames(ctx, logs)
	return logs, total, nil
}

// attachActorNames resolves this page's operator ids with one query, because a
// log line reading "kaoqy 退了这单" is actionable and "#7" is not.
func (s *GormStore) attachActorNames(ctx context.Context, logs []domain.AuditLog) {
	seen := make(map[uint]bool, len(logs))
	ids := make([]uint, 0, len(logs))
	for i := range logs {
		if logs[i].ActorID == nil || seen[*logs[i].ActorID] {
			continue
		}
		seen[*logs[i].ActorID] = true
		ids = append(ids, *logs[i].ActorID)
	}
	if len(ids) == 0 {
		return
	}

	var accounts []struct {
		ID       uint
		Username string
	}
	// The query goes to the table rather than the model on purpose: an account
	// deleted since then still has to be named in the shop's own history. A
	// lookup that fails only costs the names, never the log rows, so the page
	// still opens.
	if err := s.db.WithContext(ctx).Table("users").Select("id, username").Where("id IN ?", ids).Find(&accounts).Error; err != nil {
		return
	}

	names := make(map[uint]string, len(accounts))
	for _, account := range accounts {
		names[account.ID] = account.Username
	}
	for i := range logs {
		if logs[i].ActorID != nil {
			logs[i].ActorName = names[*logs[i].ActorID]
		}
	}
}

// exportBatch is how many log rows one download query pulls. Walking the
// filtered set in batches keeps a busy shop's whole week out of a single
// result set.
const exportBatch = 500

// Export walks the same WHERE clauses as List but across pages, so the CSV a
// shop owner downloads carries the batch the log page was showing rather than
// only its first screen. The second return says the cap was hit: a shortened
// file must never read like a complete one.
func (s *GormStore) Export(ctx context.Context, filter domain.LogFilter, limit int) ([]domain.AuditLog, bool, error) {
	var logs []domain.AuditLog
	for {
		size := exportBatch
		if room := limit - len(logs); room < size {
			size = room
		}
		if size <= 0 {
			return logs, true, nil
		}

		// Ask for one row beyond the batch: it answers whether anything is left,
		// so a download that fits the cap exactly is not reported as cut short.
		batch := make([]domain.AuditLog, 0, size+1)
		if err := s.filtered(ctx, filter).Order("created_at DESC, id DESC").
			Offset(len(logs)).Limit(size + 1).Find(&batch).Error; err != nil {
			return nil, false, err
		}
		if len(batch) <= size {
			s.attachActorNames(ctx, batch)
			return append(logs, batch...), false, nil
		}
		batch = batch[:size]
		// Names are resolved per batch: the download can span thousands of rows,
		// and one giant id list is not worth the single query it saves.
		s.attachActorNames(ctx, batch)
		logs = append(logs, batch...)
		if len(logs) >= limit {
			return logs, true, nil
		}
	}
}

// Actions returns the distinct recorded action names.
func (s *GormStore) Actions(ctx context.Context) ([]string, error) {
	actions := make([]string, 0)
	if err := s.db.WithContext(ctx).Model(&domain.AuditLog{}).
		Distinct().Order("action ASC").Pluck("action", &actions).Error; err != nil {
		return nil, err
	}
	return actions, nil
}

func (s *GormStore) filtered(ctx context.Context, filter domain.LogFilter) *gorm.DB {
	query := s.db.WithContext(ctx).Model(&domain.AuditLog{})
	if filter.Action != "" {
		// Prefix match so "order" also finds order.cancel and order.refund.
		query = query.Where("action LIKE ? ESCAPE '"+likeEscape+"'", escapeLike(filter.Action)+likeAny)
	}
	if filter.Search != "" {
		pattern := likeAny + escapeLike(filter.Search) + likeAny
		// The alternatives are parenthesised so AND with the other filters keeps
		// its precedence. Searching by who acted is part of the point: "kaoqy"
		// should find the rows that member wrote.
		one := " LIKE ? ESCAPE '" + likeEscape + "'"
		query = query.Where("(action"+one+" OR target"+one+" OR detail"+one+
			" OR actor_id IN (SELECT id FROM users WHERE username"+one+"))",
			pattern, pattern, pattern, pattern,
		)
	}
	if filter.ActorID != nil {
		query = query.Where("actor_id = ?", *filter.ActorID)
	}
	if filter.SystemOnly {
		query = query.Where("actor_id IS NULL")
	}
	if filter.Since != nil {
		query = query.Where("created_at >= ?", *filter.Since)
	}
	if filter.Before != nil {
		query = query.Where("created_at < ?", *filter.Before)
	}
	return query
}

// LIKE wildcards in a typed keyword would silently widen the match, so they are
// escaped. '/' is the escape character because a backslash needs different
// quoting in SQLite and MySQL string literals.
const (
	likeEscape = "/"
	likeAny    = "%"
)

func escapeLike(value string) string {
	replacer := strings.NewReplacer(likeEscape, likeEscape+likeEscape, "%", likeEscape+"%", "_", likeEscape+"_")
	return replacer.Replace(value)
}
