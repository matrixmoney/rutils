package format

import "errors"

// Sentinel errors returned by the formatting engine.
//
// Note the deliberately short list: unlike internal/repair, there is NO
// ErrUnrepairable. An HTML formatter cannot "fail to repair" because the
// HTML5 parser accepts anything. The only two ways to fail are (a) the
// caller handed us zero bytes, or (b) we cannot identify the format.
var (
	// ErrUnsupportedFormat is returned when DetectFormat cannot identify
	// the input, or when no Formatter is registered for the detected format.
	ErrUnsupportedFormat = errors.New("input format is not supported by the formatter")

	// ErrEmptyInput is returned when the caller hands the engine zero bytes.
	ErrEmptyInput = errors.New("input is empty")
)