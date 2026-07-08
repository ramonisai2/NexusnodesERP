package domain

import (
	"context"
	"errors"
)

var (
	ErrSoDViolation = errors.New("segregation of duties violation")
	ErrInvalidState = errors.New("invalid state")
	ErrNotFound     = errors.New("not found")
)

type PayrollLine struct {
	EmployeeID  string  `json:"employee_id"`
	ConceptCode string  `json:"concept_code"`
	Amount      float64 `json:"amount"`
}

type PayrollRun struct {
	ID          string        `json:"id"`
	PeriodLabel string        `json:"period_label"`
	BranchID    string        `json:"branch_id"`
	Status      string        `json:"status"`
	PreparedBy  string        `json:"prepared_by"`
	ApprovedBy  string        `json:"approved_by,omitempty"`
	TotalAmount float64       `json:"total_amount"`
	Lines       []PayrollLine `json:"lines,omitempty"`
	Version     int           `json:"version"`
}

type CreateRunRequest struct {
	PeriodLabel string `json:"period_label"`
	BranchID    string `json:"branch_id"`
	PreparedBy  string `json:"prepared_by"`
	OrgID       string `json:"org_id"`
}

type Store interface {
	ListRuns(ctx context.Context, orgRef string) ([]PayrollRun, error)
	GetRun(ctx context.Context, orgRef, id string) (PayrollRun, error)
	CreateAndCalculate(ctx context.Context, req CreateRunRequest) (PayrollRun, error)
	Approve(ctx context.Context, orgRef, id, actor string) (PayrollRun, error)
}
