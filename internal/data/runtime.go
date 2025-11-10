package data

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Runtime has same type as our runtime field in Movie struct
type Runtime int32

var ErrInvalidRuntimeFormat = errors.New("invalid runtime format")

/*
MarshalJSON ([]byte, error) --> Go has its own version for every type. This dictates how a type should be
converted to JSON format. We will use the Runtime type (of int32) inside the Movie struct to represent the
Runtime field of each movie. In this way we can create our own MarshalJSON() function to convert the run time
value to `intval minutes` instead of just an integer value in the json output
*/
func (r Runtime) MarshalJSON() ([]byte, error) {
	jsonValue := fmt.Sprintf("%d mins", r)

	// this wraps jsonValue in double quotes, making it a valid JSON value
	quotedJSONValue := strconv.Quote(jsonValue)
	return []byte(quotedJSONValue), nil
}

/*
in this func we are using a value receiver and not a pointer receiver for func (r Runtime)
this gives us a flexibility because Value receivers can be invoked on both pointers and values
but Pointer receiver funcs can only be invoked on pointers
*/

/*
Go has an UnmarshalJSON([]byte) error {} func for every type. It uses this func to
unmarshal json of every type. We can implement our own func for our custom types. So
when movie json data is being unmarshalled, for type Runtime, Go will use this func
that we are providing here.
*/
func (r *Runtime) UnmarshalJSON(jsonValue []byte) error {
	unquotedJSONValue, err := strconv.Unquote(string(jsonValue))
	if err != nil {
		return ErrInvalidRuntimeFormat
	}

	parts := strings.Split(unquotedJSONValue, " ")
	if len(parts) != 2 || parts[1] != "mins" {
		return ErrInvalidRuntimeFormat
	}

	i, err := strconv.ParseInt(parts[0], 10, 32)
	if err != nil {
		return ErrInvalidRuntimeFormat
	}

	*r = Runtime(i) // dereference and store the value

	return nil
}
