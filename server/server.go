package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/draganm/bolted"
	"github.com/draganm/bolted/dbpath"
	"github.com/go-logr/logr"
	"github.com/gofrs/uuid"
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus"
)

type Server struct {
	db  bolted.Database
	log logr.Logger
	http.Handler
}

var eventsPath = dbpath.ToPath("events")

func New(log logr.Logger, db bolted.Database) (*Server, error) {
	err := bolted.SugaredWrite(db, func(tx bolted.SugaredWriteTx) error {
		if !tx.Exists(eventsPath) {
			tx.CreateMap(eventsPath)

		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("could not initialize db: %w", err)
	}

	r := mux.NewRouter()

	r.Methods("POST").Path("/events").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		log := log.WithValues("method", r.Method, "path", r.URL.Path)
		events := []json.RawMessage{}

		err := json.NewDecoder(r.Body).Decode(&events)

		if err != nil {
			log.Error(err, "could not decode request")
			http.Error(w, fmt.Errorf("could not decode request: %w", err).Error(), http.StatusBadRequest)
			return
		}

		uuids := make([]string, len(events))
		for i := range events {
			id, err := uuid.NewV6()
			if err != nil {
				log.Error(err, "could not generate UUID")
				http.Error(w, fmt.Errorf("could not generate UUID: %w", err).Error(), http.StatusInternalServerError)
				return
			}
			uuids[i] = id.String()
		}

		err = bolted.SugaredWrite(db, func(tx bolted.SugaredWriteTx) error {
			for i, ev := range events {
				tx.Put(eventsPath.Append(uuids[i]), ev)
			}
			return nil
		})

		if err != nil {
			log.Error(err, "could not store events")
			http.Error(w, fmt.Errorf("could not store events: %w", err).Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)

	})
	const maxLimit = 1000

	r.Methods("GET").Path("/events").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log := log.WithValues("method", r.Method, "path", r.URL.Path)

		q := r.URL.Query()

		sort := "asc"
		sortStr := q.Get("sort")

		if sortStr != "asc" && sortStr != "desc" {
			http.Error(w, fmt.Errorf("invalid sort value: %s", sortStr).Error(), http.StatusBadRequest)
			return
		}

		if sortStr == "desc" {
			sort = sortStr
		}

		after := q.Get("after")

		limit := 100
		limitString := q.Get("limit")
		if limitString != "" {
			limit64, err := strconv.ParseInt(limitString, 10, 64)
			if err != nil {
				log.Error(err, "could not parse limit", "limit", limitString)
				http.Error(w, fmt.Errorf("could not parse limit: %w", err).Error(), http.StatusBadRequest)
				return
			}
			if limit64 > maxLimit {
				log.Error(err, "too large limit requested", "limit", limit64)
				http.Error(w, fmt.Errorf("requested limit %d is larger than allowed %d", limit64, maxLimit).Error(), http.StatusBadRequest)
				return
			}
			limit = int(limit64)
		}

		changes, done := db.Observe(eventsPath.ToMatcher().AppendAnyElementMatcher())
		defer done()
		events := []event{}

		timeout := time.Second * 20

		ctx, done := context.WithTimeout(r.Context(), timeout)
		defer done()

		for ctx.Err() == nil {

			select {
			case <-changes:
			case <-ctx.Done():
				continue
			}

			err := bolted.SugaredRead(db, func(tx bolted.SugaredReadTx) error {
				it := tx.Iterator(eventsPath)
				if after != "" {
					it.Seek(after)
					if sort == "asc" && !it.IsDone() && it.GetKey() == after {
						it.Next()
					} else if sort == "desc" && !it.IsDone() && it.GetKey() == after {
						it.Prev()
					}
				}
				if sort == "asc" {
					for ; !it.IsDone() && len(events) < limit; it.Next() {
						events = append(events, event{it.GetKey(), it.GetValue()})
					}
				} else {
					for ; !it.IsDone() && len(events) < limit; it.Prev() {
						events = append(events, event{it.GetKey(), it.GetValue()})
					}
				}
				return nil
			})

			if err != nil {
				log.Error(err, "could not read events: %w", err)
				http.Error(w, fmt.Errorf("could not read events: %w", err).Error(), http.StatusInternalServerError)
				return
			}

			if len(events) > 0 {
				break
			}
		}

		if ctx.Err() == context.DeadlineExceeded {
			log.Error(err, "request timed out")
			http.Error(w, fmt.Errorf("request timed out: %w", err).Error(), http.StatusRequestTimeout)
			return
		}

		if ctx.Err() != nil {
			log.Error(err, "request context cancelled")
			http.Error(w, fmt.Errorf("request context cancelled: %w", err).Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("content-type", "application/json")
		json.NewEncoder(w).Encode(events)

	})

	prometheus.Register(newStatsCollector(db, log))

	return &Server{
		Handler: r,
		db:      db,
		log:     log,
	}, nil
}
