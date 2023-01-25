package server

import (
	"fmt"
	"net/http"

	"github.com/draganm/bolted"
	"github.com/draganm/bolted/dbpath"
	"github.com/go-logr/logr"
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

	return &Server{
		Handler: r,
		db:      db,
		log:     log,
	}, nil
}
