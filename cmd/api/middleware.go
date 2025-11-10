package main

import (
	"fmt"
	"net/http"
)

func (app *application) recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// deferred func will be called after panic
		defer func() {
			// built-in recover() func returns err if there has been a panic
			if err := recover(); err != nil {
				/*
				   if there was a panic, set a "Connection: close" header on the
				   response. This acts as a trigger to make Go's HTTP server automatically
				   close the current connection after a response has been sent.
				*/
				w.Header().Set("Connection", "close")
				/*
				   the value returned by recover() has the type "any", so we use fmt.Errorf
				   to convert it to an error type, and use our custom error logger
				*/
				app.serverErrorResponse(w, r, fmt.Errorf("%s", err))
			}
		}()

		next.ServeHTTP(w, r)
	})
}
