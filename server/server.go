package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/draganm/bolted"
	"github.com/draganm/bolted/dbpath"
	"github.com/go-logr/logr"
	"github.com/gofrs/uuid"
	"github.com/gorilla/mux"
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

		w.WriteHeader(http.StatusNoContent)

	})

	r.Methods("GET").Path("/events").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log := log.WithValues("method", r.Method, "path", r.URL.Path)

		changes, done := db.Observe(eventsPath.ToMatcher().AppendAnyElementMatcher())
		defer done()
		events := []event{}
		maxSize := 100

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
				for it := tx.Iterator(eventsPath); !it.IsDone() && len(events) < maxSize; it.Next() {
					events = append(events, event{it.GetKey(), it.GetValue()})
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

	return &Server{
		Handler: r,
		db:      db,
		log:     log,
	}, nil
}

func (s *Server) Close() error {
	return nil
}
