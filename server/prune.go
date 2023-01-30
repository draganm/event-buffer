package server

import (
	"fmt"
	"time"

	"github.com/draganm/bolted"
	"github.com/gofrs/uuid"
)

func (s Server) Prune(cutoffTime time.Time) (err error) {
	return bolted.SugaredWrite(s.db, func(tx bolted.SugaredWriteTx) (err error) {
		toDelete := []string{}
		defer func() {
			if err == nil {
				s.log.Info("pruned state events", "count", len(toDelete))
			}
		}()
		it := tx.Iterator(eventsPath)
		it.Last()
		for ; !it.IsDone(); it.Prev() {
			id, err := uuid.FromString(it.GetKey())
			if err != nil {
				return fmt.Errorf("could not parse uuid %s: %w", it.GetKey(), err)
			}

			ts, err := uuid.TimestampFromV6(id)
			if err != nil {
				return fmt.Errorf("could not get uuid timestamp: %w", err)
			}

			t, err := ts.Time()
			if err != nil {
				return fmt.Errorf("could not get time from uuid timestamp: %w", err)
			}

			if !t.Before(cutoffTime) {
				break
			}
			toDelete = append(toDelete, it.GetKey())

		}

		for _, id := range toDelete {
			tx.Delete(eventsPath.Append(id))
		}
		return nil
	})
}
