package main

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/souvikmndl/greenlight-api/internal/data"
	"github.com/souvikmndl/greenlight-api/internal/validator"
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

	if !app.config.limiter.enabled {
		return next
	}

	type client struct {
		limiter  *rate.Limiter
		lastSeen time.Time
	}

	var (
		mu      sync.Mutex
		clients = make(map[string]*client) // for client based rate limiting
	)

	// clean up client ip entries that are older than 3 seconds to allow for fresh requests
	go func() {
		for {
			time.Sleep(time.Minute)

			// Lock the mutex to prevent any rate limiter checks happening while clean up
			mu.Lock()

			for ip, client := range clients {
				if time.Since(client.lastSeen) > 3*time.Minute {
					delete(clients, ip)
				}
			}

			mu.Unlock()
		}
	}()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// fetch real IP of client, sometimes it might be hidden behind proxies
		ip := realip.FromRequest(r) // looks for the X-Forwarded-For or X-Real-IP header

		// Lock the rate limiter as requests are concurrently processed
		mu.Lock()

		// check to see if the client IP already exists in the map. if it doesnt, then
		// initialise a new rate limiter and add to map for the IP
		if _, found := clients[ip]; !found {
			clients[ip] = &client{limiter: rate.NewLimiter(rate.Limit(app.config.limiter.rps), app.config.limiter.burst)}
		}

		clients[ip].lastSeen = time.Now()

		// call the rate limiter check for this client only
		if !clients[ip].limiter.Allow() {
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

func (app *application) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		// this header indicates to any caches that the response may vary
		// based in the value of the "Authorization" headers in the request
		w.Header().Add("Vary", "Authorization")

		authorizationHeader := r.Header.Get("Authorization")

		// if there is no auth header, we use Anonymous user and call the
		// next mw in the chain and return
		if authorizationHeader == "" {
			r = app.contextSetUser(r, data.AnonymousUser)
			next.ServeHTTP(w, r)
			return
		}

		// if auth header is present, we extract to verify user details
		headerParts := strings.Split(authorizationHeader, " ")
		if len(headerParts) != 2 || headerParts[0] != "Bearer" {
			app.invalidCredentialsResponse(w, r)
			return
		}

		token := headerParts[1]
		// validate token
		v := validator.New()
		data.ValidateTokenPlaintext(v, token)
		if !v.Valid() {
			app.invalidCredentialsResponse(w, r)
			return
		}

		user, err := app.models.Users.GetForToken(data.ScopeAuthentication, token)
		if err != nil {
			switch {
			case errors.Is(err, data.ErrRecordNotFound):
				app.invalidAuthenticationTokenResponse(w, r)
			default:
				app.serverErrorResponse(w, r, err)
			}

			return
		}

		r = app.contextSetUser(r, user)

		next.ServeHTTP(w, r)
	})
}

// checks if user is authenticated
func (app *application) requireAuthenticatedUser(next http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := app.contextGetUser(r)

		if user.IsAnonymous() {
			app.authenticationRequiredResponse(w, r)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// checks if user is both authenticated and active
func (app *application) requireActivatedUser(next http.HandlerFunc) http.HandlerFunc {
	fn := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := app.contextGetUser(r)

		if !user.Activated {
			app.inactiveAccountResponse(w, r)
			return
		}

		next.ServeHTTP(w, r)
	})

	return app.requireAuthenticatedUser(fn)
}

// for rbac
func (app *application) requirePermission(code string, next http.HandlerFunc) http.HandlerFunc {
	// fn := func(w http.ResponseWriter, r *http.Request) {
	// 	user := app.contextGetUser(r)

	// 	permissions, err := app.models.Permissions.GetAllForuser(user.ID)
	// 	if err != nil {
	// 		app.serverErrorResponse(w, r, err)
	// 		return
	// 	}

	// 	if !permissions.Include(code) {
	// 		app.notPermittedResponse(w, r)
	// 		return
	// 	}

	// 	next.ServeHTTP(w, r)
	// }

	return app.requireActivatedUser(next)
}
