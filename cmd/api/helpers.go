package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/julienschmidt/httprouter"
)

type envelope map[string]any

func (app *application) readIDParams(r *http.Request) (int64, error) {
	params := httprouter.ParamsFromContext(r.Context())

	id, err := strconv.ParseInt(params.ByName("id"), 10, 64)
	if err != nil || id < 1 {
		return 0, errors.New("invalid id parameter")
	}

	return id, nil
}

func (app *application) writeJSON(w http.ResponseWriter, status int, data envelope, headers http.Header) error {
	js, err := json.MarshalIndent(data, "", "\t")
	if err != nil {
		return err
	}

	js = append(js, '\n') // adding a newline for better readability

	for key, value := range headers {
		w.Header()[key] = value
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(js)

	return nil
}

// readJSON will try to decode the incoming JSON payload into dst and return errors if any
/*
JSON Decode() error using NewDecoder() from json/encoding

- json.SyntexError, io.ErrEnexptedEOF --> syntax error with the JSON payload
- json.UnmarshalTypeError --> a JSON value is not appropriate for the destination Go type
- json.InvalidUnmarshalError --> err in app code, possibly because the destination is not a pointer
- io.EOF --> JSON being decoded in empty
*/
func (app *application) readJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields() // does not allow fields not defined in the dst struct

	err := dec.Decode(dst)
	if err != nil {
		var syntaxError *json.SyntaxError
		var unmarshalTypeError *json.UnmarshalTypeError
		var invalidUnmarshalError *json.InvalidUnmarshalError

		switch {
		case errors.As(err, &syntaxError):
			return fmt.Errorf("body contains bady-formed JSON (at character %d)", syntaxError.Offset)
		case errors.Is(err, io.ErrUnexpectedEOF):
			return errors.New("body contains badly-formed JSON")
		// this kind of error occours when JSON value is the wrong type for the target dest
		// if the err is related to a specific field, we show that else a generic msg
		case errors.As(err, &unmarshalTypeError):
			if unmarshalTypeError.Field != "" {
				return fmt.Errorf("body contains incorrect JSON type for field %q", unmarshalTypeError.Field)
			}
			return fmt.Errorf("body contains incorrect JSON type (at character %d)", unmarshalTypeError.Offset)
		case errors.Is(err, io.EOF):
			return errors.New("body must not be empty")
		case errors.As(err, &invalidUnmarshalError):
			panic(err) // read page 91 of Lets Go Further to understand why we are panicking here
			// basically this means there is a logical error in our code, and should be caught in dev
		default:
			return err
		}
	}

	//we can send multiple JSON object ina request, and attackers can use this feature
	//to send something malicious or send huge request body to slowdown our apis(in a DDOS attack)
	// when we call Decode() it only parses one JSON body at a time, so we need to call Decode() again, using
	// a pointer to an empty struct. If the req body contained a single JSON, this will throw an io.EOF error.
	//So if we get anything else, we know that there is additional data in the req body and we can return err.
	// curl -i -d '{"title": "Moana"}{"title": "Top Gun"}' localhost:4000/v1/movies (two JSON objs in req body)
	err = dec.Decode(&struct{}{})
	if !errors.Is(err, io.EOF) {
		return errors.New("body must contain a single JSON value")
	}

	return nil
}
