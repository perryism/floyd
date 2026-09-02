package observability

import (
	"encoding/json"
	"log"
	"time"
)

// Outcome values for a HandlerExecutionRecord (FR-008, data-model.md).
type Outcome string

const (
	OutcomePassed   Outcome = "passed"
	OutcomeModified Outcome = "modified"
	OutcomeRejected Outcome = "rejected"
	OutcomeErrored  Outcome = "errored"
)

// Direction values for a HandlerExecutionRecord.
type Direction string

const (
	DirectionRequest  Direction = "request"
	DirectionResponse Direction = "response"
)

// HandlerExecutionRecord is one log entry per handler invocation, for observability and
// troubleshooting (FR-008, data-model.md). It has no "route" field: a single process serves
// exactly one destination, so that would be constant within a process's own logs.
type HandlerExecutionRecord struct {
	RequestID  string    `json:"request_id"`
	Handler    string    `json:"handler_name"`
	Direction  Direction `json:"direction"`
	Outcome    Outcome   `json:"outcome"`
	DurationMS int64     `json:"duration_ms"`
	Timestamp  time.Time `json:"timestamp"`
}

// LogHandlerExecution records one HandlerExecutionRecord. Kept as a small seam (rather than a
// direct log.Println call at every call site) so the output format changes in exactly one
// place if this ever needs to become structured/shipped elsewhere.
func LogHandlerExecution(rec HandlerExecutionRecord) {
	b, err := json.Marshal(rec)
	if err != nil {
		log.Printf("observability: failed to marshal handler execution record: %v", err)
		return
	}
	log.Println(string(b))
}
