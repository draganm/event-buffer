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

	"github.com/go-logr/zapr"
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
		},
		Action: func(c *cli.Context) error {
			log := zapr.NewLogger(logger)
			defer log.Info("server exiting")
			eg, ctx := errgroup.WithContext(context.Background())

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

				s := &http.Server{}

				go func() {
					<-ctx.Done()
					shutdownContext, cancel := context.WithTimeout(context.Background(), time.Second)
					defer cancel()
					log.Info("graceful shutdown of the server")
					err := s.Shutdown(shutdownContext)
					if errors.Is(err, context.DeadlineExceeded) {
						log.Info("server did not shut down gracefully, forcing close")
						s.Close()
					}
				}()

				log.Info("server started", "addr", l.Addr().String())
				return s.Serve(l)
			})

			return eg.Wait()

		},
	}
	app.RunAndExitOnError()
}
