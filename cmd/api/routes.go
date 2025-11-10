package main

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
)

func (app *application) routes() http.Handler {
	router := httprouter.New()

	// we are setting our own error handlers for these two cases on the router
	// the default one send plain text resp while ours will send json
	router.NotFound = http.HandlerFunc(app.notFoundResponse)
	router.MethodNotAllowed = http.HandlerFunc(app.methodNotAllowedResponse)

	router.HandlerFunc(http.MethodGet, "/v1/healthcheck", app.healthCheckHandler)
	router.HandlerFunc(http.MethodGet, "/v1/movies/:id", app.showMovieHandler)
	router.HandlerFunc(http.MethodPost, "/v1/movies", app.createMovieHandler)

	// this recoverPanic middleware will only handle panics in main thread
	// if we spin up our own threads and there is a panic in them, that wont
	// be handled and our app will crash. We will need to handle panics in
	// each thread that we spin up.
	return app.recoverPanic(router)
}
