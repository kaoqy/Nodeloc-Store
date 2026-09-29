package application

import (
	"context"
	"strings"
	"testing"

	"github.com/kaoqy/Nodeloc-Store/internal/modules/audit/domain"
)

// recordingRepo keeps whatever filter the service settled on so the tests can
// check what a download actually asks the database for.
type recordingRepo struct {
	filter domain.LogFilter
	limit  int
	rows   []domain.AuditLog
	// truncated is what the repo reports back, to check the service passes it on.
	truncated bool
}

func (r *recordingRepo) Create(ctx context.Context, entry *domain.AuditLog) error { return nil }

func (r *recordingRepo) List(ctx context.Context, filter domain.LogFilter) ([]domain.AuditLog, int64, error) {
	r.filter = filter
	return r.rows, int64(len(r.rows)), nil
}

func (r *recordingRepo) Export(ctx context.Context, filter domain.LogFilter, limit int) ([]domain.AuditLog, bool, error) {
	r.filter, r.limit = filter, limit
	return r.rows, r.truncated, nil
}

func (r *recordingRepo) Actions(ctx context.Context) ([]string, error) { return nil, nil }

// A keyword typed with stray spaces, or one long enough to turn into a
// full-table scan, is normalised the same way whether it came from the page or
// from the download button.
func TestExportNormalisesTheFilterLikeThePage(t *testing.T) {
	repo := &recordingRepo{}
	service := NewService(repo)

	_, _, err := service.ExportLogs(context.Background(), domain.LogFilter{
		Action: "  order.refund ",
		Search: "  " + strings.Repeat("密", maxSearchLen+20) + "  ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if repo.filter.Action != "order.refund" {
		t.Fatalf("action reached the store as %q, want it trimmed", repo.filter.Action)
	}
	if got := len([]rune(repo.filter.Search)); got != maxSearchLen {
		t.Fatalf("keyword reached the store at %d runes, want it capped at %d", got, maxSearchLen)
	}
	if repo.filter.Search != strings.Repeat("密", maxSearchLen) {
		t.Fatal("keyword was trimmed from the wrong end")
	}
	if repo.limit != maxExportLogs {
		t.Fatalf("export asked for a cap of %d, want the service's own %d", repo.limit, maxExportLogs)
	}
}

// A download that had to stop short must keep saying so on the way out, or the
// shop owner reads a partial file as the whole story.
func TestExportPassesTruncationThrough(t *testing.T) {
	repo := &recordingRepo{
		rows:      []domain.AuditLog{{Action: "order.refund"}},
		truncated: true,
	}
	service := NewService(repo)

	rows, truncated, err := service.ExportLogs(context.Background(), domain.LogFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if !truncated || len(rows) != 1 {
		t.Fatalf("got %d rows truncated=%v, want 1 row truncated", len(rows), truncated)
	}
}
