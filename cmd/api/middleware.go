package main

import (
	"fmt"
	"net/http"
	"sync"

	"github.com/tomasen/realip"
	"golang.org/x/time/rate"
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

/*
func (app *application) rateLimit(next http.Handler) http.Handler {
	// code in this section will run only once, when we wrap something with the mw

	// init our rate limiter. This will allow average of 2 requests per second,
	// with a max 4 requests in a single burst
	limiter := rate.NewLimiter(2, 4)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Any code here will run for every request the the mw handles

		// Call limiter.Allow() to see if the request is permitted, and if its not
		// then we call the rateLimitExceededResponse() helper to return 429 code
		if !limiter.Allow() {

			// whenever Allow() is called, exactly one token will be consumed from the bucket. If
			// there are no tokens left in the bucket, Allow() will return false and that acts
			// as the trigger to send err 429

			app.rateLimitExceededResponse(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}
*/

func (app *application) rateLimit(next http.Handler) http.Handler {
	var (
		mu      sync.Mutex
		clients = make(map[string]*rate.Limiter) // for client based rate limiting
	)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// fetch real IP of client, sometimes it might be hidden behind proxies
		ip := realip.FromRequest(r) // looks for the X-Forwarded-For or X-Real-IP header

		// Lock the rate limiter as requests are concurrently processed
		mu.Lock()

		// check to see if the client IP already exists in the map. if it doesnt, then
		// initialise a new rate limiter and add to map for the IP
		if _, found := clients[ip]; !found {
			clients[ip] = rate.NewLimiter(2, 4)
		}

		// call the rate limiter check for this client only
		if !clients[ip].Allow() {
			mu.Unlock()
			app.rateLimitExceededResponse(w, r)
			return
		}

		// unlock mutex for the else path
		// we dont use defer to unlock as that would mean that the mutex isnt unlocked untill
		// all the handlers downstream of this mw have also returned
		mu.Unlock()
		next.ServeHTTP(w, r)
	})
}
