package audit

import (
	"context"
	"log"

	"github.com/kaoqy/Nodeloc-Store/internal/modules/audit/application"
	"github.com/kaoqy/Nodeloc-Store/internal/modules/audit/domain"
)

// Recorder writes admin actions into the audit trail. It is deliberately
// best-effort: a failed audit write must never turn a successful mutation into
// an error response, so problems only reach the server log.
//
// The service is resolved per write rather than captured: saving settings
// rebuilds the application and closes the database the saving request itself was
// served from, so a recorder bound to that handle would be stale by the time it
// audits the action.
type Recorder struct {
	resolve func() *application.Service
}

func NewRecorder(resolve func() *application.Service) *Recorder {
	return &Recorder{resolve: resolve}
}

func (r *Recorder) Record(action, target, detail string, actorID uint, ip string) {
	if r == nil || r.resolve == nil || action == "" {
		return
	}
	service := r.resolve()
	if service == nil {
		return
	}
	input := domain.LogActionInput{
		ActorID: optionalUint(actorID),
		Action:  action,
		Target:  optionalText(target, 120),
		Detail:  optionalText(detail, 0),
		IP:      optionalText(ip, 64),
	}
	if _, err := service.LogAction(context.Background(), input); err != nil {
		log.Printf("[audit] record %s: %v", action, err)
	}
}

func optionalText(value string, maxLen int) *string {
	if value == "" {
		return nil
	}
	if maxLen > 0 && len(value) > maxLen {
		value = value[:maxLen]
	}
	return &value
}

func optionalUint(value uint) *uint {
	if value == 0 {
		return nil
	}
	return &value
}
