package obs

import (
	"context"
	"log"
	"time"
)

type ctxKey string

const RequestIDKey ctxKey = "req_id"

func Time(ctx context.Context, name string) func(errp *error) {
	start := time.Now()

	reqID, _ := ctx.Value(RequestIDKey).(string)

	return func(errp *error) {
		dur := time.Since(start)

		if errp != nil && *errp != nil {
			log.Printf("req_id=%s op=%s dur=%dms err=%v", reqID, name, dur.Milliseconds(), *errp)
			return
		}
		log.Printf("req_id=%s op=%s dur=%dms", reqID, name, dur.Milliseconds())
	}
}
