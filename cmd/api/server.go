package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func (app *application) serve() error {
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", app.config.port),
		Handler:      app.routes(),
		IdleTimeout:  time.Minute,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		ErrorLog:     slog.NewLogLogger(app.logger.Handler(), slog.LevelError),
	}

	shutdownError := make(chan error)

	// start a background go routine, it will rn for the lifetime of our application
	// and catch signals we specify
	go func() {
		// create a channel which carries os.Signal values
		quit := make(chan os.Signal, 1)
		// we are using a buffered channel because signal.Notify() does not wait for a
		// receiver to be available when sending the signal. So a regular channel might
		// miss the signal from Notify()

		// use signal.Notify() to listen for incoming SIGINT and SIGTERM signals and
		// relay them to the quit channel. Other signals will not be caught, retaining original behaviour
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

		// read the signal here, code will block until signal is received
		s := <-quit

		app.logger.Info("caught signal", "signal", s.String())

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// graceful shutdown
		shutdownError <- srv.Shutdown(ctx)
	}()

	app.logger.Info("starting server", "addr", srv.Addr, "env", app.config.env)

	// calling shutdown() on our server will cause ListenAndServe() to immediately return
	// a http.ErrServerClosed error. So if we encounter it, it means Graceful Shutdown worked
	// that is why we return error here if it is not http.ErrServerClosed
	err := srv.ListenAndServe()
	if !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	// if graceful shutdown returned an error it is caught here
	// if it is successful then err is nil
	err = <-shutdownError
	if err != nil {
		return err
	}

	app.logger.Info("stopped server", "addr", srv.Addr)

	return nil
}

/*
It’s important to be aware that the Shutdown() method does not wait for any background
tasks to complete, nor does it close hijacked long-lived connections like WebSockets.
Instead, you will need to implement your own logic to coordinate a graceful shutdown of
these things.
*/
