package domain

import (
	"context"
	"errors"
	"time"
)

var (
	ErrNotFound = errors.New("not found")
	ErrInvalid  = errors.New("invalid")
)

type ImageReport struct {
	ID               string    `json:"id"`
	OrgID            string    `json:"org_id"`
	BranchID         string    `json:"branch_id"`
	CreatedBy        string    `json:"created_by"`
	Title            string    `json:"title"`
	Notes            string    `json:"notes,omitempty"`
	StorageKey       string    `json:"-"`
	OriginalFilename string    `json:"original_filename,omitempty"`
	MimeType         string    `json:"mime_type"`
	Width            int       `json:"width"`
	Height           int       `json:"height"`
	ByteSize         int       `json:"byte_size"`
	OriginalWidth    *int      `json:"original_width,omitempty"`
	OriginalHeight   *int      `json:"original_height,omitempty"`
	OriginalByteSize *int      `json:"original_byte_size,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	ContentURL       string    `json:"content_url,omitempty"`
}

type CreateImageReport struct {
	OrgID            string
	BranchID         string
	CreatedBy        string
	Title            string
	Notes            string
	StorageKey       string
	OriginalFilename string
	MimeType         string
	Width            int
	Height           int
	ByteSize         int
	OriginalWidth    int
	OriginalHeight   int
	OriginalByteSize int
}

type Store interface {
	Create(ctx context.Context, in CreateImageReport) (ImageReport, error)
	List(ctx context.Context, orgRef, branchRef string) ([]ImageReport, error)
	Get(ctx context.Context, orgRef, id string) (ImageReport, error)
}
