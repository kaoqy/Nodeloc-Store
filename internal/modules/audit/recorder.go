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
type Recorder struct {
	service *application.Service
}

func NewRecorder(service *application.Service) *Recorder {
	return &Recorder{service: service}
}

func (r *Recorder) Record(action, target, detail string, actorID uint, ip string) {
	if r == nil || r.service == nil || action == "" {
		return
	}
	input := domain.LogActionInput{
		ActorID: optionalUint(actorID),
		Action:  action,
		Target:  optionalText(target, 120),
		Detail:  optionalText(detail, 0),
		IP:      optionalText(ip, 64),
	}
	if _, err := r.service.LogAction(context.Background(), input); err != nil {
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
