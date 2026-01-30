package main

import (
	"errors"
	"expvar"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/souvikmndl/greenlight-api/internal/data"
	"github.com/souvikmndl/greenlight-api/internal/validator"
	"github.com/tomasen/realip"
	"golang.org/x/time/rate"
)

// wrapping existing http.ResponseWriter interface to capture status codes
type metricsResponseWriter struct {
	wrapped       http.ResponseWriter
	statusCode    int
	headerWritten bool
}

func newMetricsResponseWriter(w http.ResponseWriter) *metricsResponseWriter {
	return &metricsResponseWriter{
		wrapped:    w,
		statusCode: http.StatusOK,
	}
}

// Header returns header map from origin http.ResponseWriter that we wrapped
func (mw *metricsResponseWriter) Header() http.Header {
	return mw.wrapped.Header()
}

// WriteHeader writes the statusCode to our wrapper if not already written
// It does a passthrough to the origin wrapper http.ResponseWriter
func (mw *metricsResponseWriter) WriteHeader(statusCode int) {
	mw.wrapped.WriteHeader(statusCode)

	if !mw.headerWritten {
		mw.statusCode = statusCode
		mw.headerWritten = true
	}
}

// Write does a pass through to the Write() method of the wrapped http.ResponseWriter()
// Calling this will automatically write any resp headers
func (mw *metricsResponseWriter) Write(b []byte) (int, error) {
	mw.headerWritten = true
	return mw.wrapped.Write(b)
}

// Unwrap returns the wrapped http.ResponseWriter
func (mw *metricsResponseWriter) Unwrap() http.ResponseWriter {
	return mw.wrapped
}

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

// Allows cors for whitelisted origins
func (app *application) enableCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		//w.Header().Set("Access-Control-Allow-Origin", "*")

		// Origin header might vary, based on the incoming request origin
		w.Header().Add("Vary", "Origin")

		// for preflight CORS
		w.Header().Add("Vary", "Access-Control-Request-Method")

		origin := r.Header.Get("Origin")

		if origin != "" {
			for i := range app.config.cors.trustedOrigins {
				if origin == app.config.cors.trustedOrigins[i] {
					w.Header().Set("Access-Control-Allow-Origin", origin)

					// Options request is for preflight cors, it asks for which methods and headers
					//  are allowed/needed. Only the CORS unsafe headers and methods are added here
					// as the safe ones are already allowed
					if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
						w.Header().Set("Access-Control-Allow-Methods", "OPTIONS, PUT, PATCH, DELETE")
						w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
					}

					break
				}
			}
		}

		next.ServeHTTP(w, r)
	})
}

func (app *application) metrics(next http.Handler) http.Handler {
	// initialise the new expvar variables when the mw chain is first built
	var (
		totalRequestsReceived           = expvar.NewInt("total_requests_received")
		totalResponsesSent              = expvar.NewInt("total_responses_sent")
		totalProcessingTimeMicroseconds = expvar.NewInt("total_processing_time_ms")
		totalResponsesSentByStatus      = expvar.NewMap("total_responses_sent_by_status")
	)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		totalRequestsReceived.Add(1)
		mw := newMetricsResponseWriter(w)

		next.ServeHTTP(w, r)
		totalResponsesSent.Add(1)

		totalResponsesSentByStatus.Add(strconv.Itoa(mw.statusCode), 1)
		duration := time.Since(start).Microseconds()
		totalProcessingTimeMicroseconds.Add(duration)
	})
}
