package search

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
)

const cursorVersion = 1

type CursorState struct {
	Version     int           `json:"v"`
	Org         string        `json:"org"`
	QueryHash   string        `json:"qh"`
	Limit       int           `json:"l"`
	SearchAfter []interface{} `json:"sa"`
}

func EncodeCursor(state CursorState) (string, error) {
	if state.Version == 0 {
		state.Version = cursorVersion
	}
	raw, err := json.Marshal(state)
	if err != nil {
		return "", fmt.Errorf("%w: encode cursor: %v", ErrInvalidCursor, err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func DecodeCursor(token string) (CursorState, error) {
	if token == "" {
		return CursorState{}, fmt.Errorf("%w: empty cursor", ErrInvalidCursor)
	}

	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return CursorState{}, fmt.Errorf("%w: invalid base64", ErrInvalidCursor)
	}

	var state CursorState
	if err := json.Unmarshal(raw, &state); err != nil {
		return CursorState{}, fmt.Errorf("%w: invalid payload", ErrInvalidCursor)
	}

	if state.Version != cursorVersion {
		return CursorState{}, fmt.Errorf("%w: unsupported version", ErrInvalidCursor)
	}
	if len(state.SearchAfter) == 0 {
		return CursorState{}, fmt.Errorf("%w: missing search_after", ErrInvalidCursor)
	}

	return state, nil
}

func ValidateCursor(state CursorState, org, qHash string, limit int) error {
	if state.Org != org {
		return fmt.Errorf("%w: org mismatch", ErrInvalidCursor)
	}
	if state.QueryHash != qHash {
		return fmt.Errorf("%w: query mismatch", ErrInvalidCursor)
	}
	if state.Limit != limit {
		return fmt.Errorf("%w: limit mismatch", ErrInvalidCursor)
	}
	return nil
}
