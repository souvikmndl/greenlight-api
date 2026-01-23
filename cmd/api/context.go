package main

import (
	"context"
	"net/http"

	"github.com/souvikmndl/greenlight-api/internal/data"
)

// contextKey type to prevent collisions while storing key value pairs in context
type contextKey string

// "user" key of type contextKey to store user data in context
const userContextKey = contextKey("user")

// we change the context value or "r" to include our user data as well
// context.WithValue(r.Context(), userContextKey, user) creates as new ctx with our user
// data and all existing data in "r".
// r.WithContext(ctx) returns a shall copy of this ctx
func (app *application) contextSetUser(r *http.Request, user *data.User) *http.Request {
	ctx := context.WithValue(r.Context(), userContextKey, user)
	return r.WithContext(ctx)
}

// contextGetUser fetches a user data from a request ctx
func (app *application) contextGetUser(r *http.Request) *data.User {
	user, ok := r.Context().Value(userContextKey).(*data.User)
	if !ok {
		panic("missing user value in request context")
	}

	return user
}
