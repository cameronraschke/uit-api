package types

import (
	"time"

	"golang.org/x/time/rate"
)

type GeneralNoteResponse struct {
	Time        *time.Time `json:"time"`
	NoteType    *string    `json:"note_type"`
	NoteContent *string    `json:"note"`
	ToDo        *string    `json:"todo"`
	Projects    *string    `json:"projects"`
	Misc        *string    `json:"misc"`
	Bugs        *string    `json:"bugs"`
}

type RateLimiter struct {
	Type     string
	Limiter  *rate.Limiter
	LastSeen time.Time
}
