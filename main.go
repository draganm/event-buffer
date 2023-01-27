package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/draganm/bolted/embedded"
	"github.com/draganm/event-buffer/server"
	"github.com/go-logr/zapr"
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/urfave/cli/v2"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"golang.org/x/sync/errgroup"
)

func main() {
	logger, _ := zap.Config{
		Encoding:    "json",
		Level:       zap.NewAtomicLevelAt(zapcore.DebugLevel),
		OutputPaths: []string{"stdout"},
		EncoderConfig: zapcore.EncoderConfig{
			MessageKey:   "message",
			LevelKey:     "level",
			EncodeLevel:  zapcore.CapitalLevelEncoder,
			TimeKey:      "time",
			EncodeTime:   zapcore.ISO8601TimeEncoder,
			CallerKey:    "caller",
			EncodeCaller: zapcore.ShortCallerEncoder,
		},
	}.Build()

	defer logger.Sync()
	app := &cli.App{
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "addr",
				Value:   ":5566",
				EnvVars: []string{"ADDR"},
			},
			&cli.StringFlag{
				Name:    "metrics-addr",
				Value:   ":3000",
				EnvVars: []string{"METRICS_ADDR"},
			},
			&cli.StringFlag{
				Name:    "state-file",
				Value:   "state",
				EnvVars: []string{"STATE_FILE"},
			},
			&cli.DurationFlag{
				Name:    "retention-period",
				EnvVars: []string{"RETENTION_PERIOD"},
				Value:   2 * time.Hour,
			},
			&cli.DurationFlag{
				Name:    "prune-frequency",
				EnvVars: []string{"PRUNE_FREQUENCY"},
				Value:   5 * time.Minute,
			},
		},
		Action: func(c *cli.Context) error {
			log := zapr.NewLogger(logger)
			defer log.Info("server exiting")
			eg, ctx := errgroup.WithContext(context.Background())

			db, err := embedded.Open(c.String("state-file"), 0700, embedded.Options{})
			if err != nil {
				return fmt.Errorf("could not open state: %w", err)
			}

			srv, err := server.New(log, db)

			eg.Go(func() error {
				sigChan := make(chan os.Signal)
				signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
				select {
				case sig := <-sigChan:
					log.Info("received signal", "signal", sig.String())
					return fmt.Errorf("received signal %s", sig.String())
				case <-ctx.Done():
					return nil
				}
			})

			eg.Go(func() error {
				l, err := net.Listen("tcp", c.String("addr"))
				if err != nil {
					return fmt.Errorf("could not listen: %w", err)

				}

				s := &http.Server{
					Handler: srv,
				}

				go func() {
					<-ctx.Done()
					shutdownContext, cancel := context.WithTimeout(context.Background(), time.Second)
					defer cancel()
					log.Info("graceful shutdown of the server")
					err := s.Shutdown(shutdownContext)
					if errors.Is(err, context.DeadlineExceeded) {
						log.Info("http server did not shut down gracefully, forcing close")
						s.Close()
					}
				}()

				log.Info("server started", "addr", l.Addr().String())
				return s.Serve(l)
			})

			eg.Go(func() error {
				l, err := net.Listen("tcp", c.String("metrics-addr"))
				if err != nil {
					return fmt.Errorf("could not listen for metric requests: %w", err)

				}

				r := mux.NewRouter()
				r.Methods("GET").Path("/metrics").Handler(promhttp.Handler())

				s := &http.Server{
					Handler: r,
				}

				go func() {
					<-ctx.Done()
					shutdownContext, cancel := context.WithTimeout(context.Background(), time.Second)
					defer cancel()
					log.Info("graceful shutdown of the metrics server")
					err := s.Shutdown(shutdownContext)
					if errors.Is(err, context.DeadlineExceeded) {
						log.Info("metrics server did not shut down gracefully, forcing close")
						s.Close()
					}
				}()

				log.Info("metrics server started", "addr", l.Addr().String())
				return s.Serve(l)
			})

			eg.Go(func() error {

				ticker := time.NewTicker(c.Duration("prune-frequency"))

				for {
					select {
					case <-ctx.Done():
						return nil
					case <-ticker.C:
						err = srv.Prune(time.Now().Add(-c.Duration("retention-period")))
						if err != nil {
							log.Error(err, "prune failed")
						}
					}

				}
			})

			return eg.Wait()

		},
	}
	app.RunAndExitOnError()
}
