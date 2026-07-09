package store

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/ramonisai2/NexusnodesERP/apps/payroll/internal/domain"
)

type employee struct {
	ID         string
	BranchID   string
	Name       string
	BaseSalary float64
}

type Memory struct {
	mu        sync.Mutex
	employees []employee
	runs      map[string]*domain.PayrollRun
}

func NewMemory() *Memory {
	return &Memory{runs: map[string]*domain.PayrollRun{}}
}

func (s *Memory) SeedDemo() {
	s.employees = []employee{
		{ID: "emp_1", BranchID: "br_norte", Name: "Ana López", BaseSalary: 25000},
		{ID: "emp_2", BranchID: "br_norte", Name: "Luis Pérez", BaseSalary: 22000},
		{ID: "emp_3", BranchID: "br_sur", Name: "María Ruiz", BaseSalary: 28000},
	}
}

func (s *Memory) ListRuns(_ context.Context, _ string) ([]domain.PayrollRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]domain.PayrollRun, 0, len(s.runs))
	for _, r := range s.runs {
		out = append(out, *r)
	}
	return out, nil
}

func (s *Memory) GetRun(_ context.Context, _, id string) (domain.PayrollRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.runs[id]
	if !ok {
		return domain.PayrollRun{}, domain.ErrNotFound
	}
	return *r, nil
}

func (s *Memory) CreateAndCalculate(_ context.Context, req domain.CreateRunRequest) (domain.PayrollRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if req.BranchID == "" || req.PeriodLabel == "" {
		return domain.PayrollRun{}, errors.New("branch_id and period_label required")
	}
	run := &domain.PayrollRun{
		ID:          "run_" + uuid.NewString(),
		PeriodLabel: req.PeriodLabel,
		BranchID:    req.BranchID,
		Status:      "IN_REVIEW",
		PreparedBy:  req.PreparedBy,
		Version:     1,
	}
	var total float64
	for _, e := range s.employees {
		if e.BranchID != req.BranchID {
			continue
		}
		run.Lines = append(run.Lines, domain.PayrollLine{
			EmployeeID: e.ID, EmployeeName: e.Name, ConceptCode: "BASE", Amount: e.BaseSalary,
		})
		total += e.BaseSalary
	}
	run.TotalAmount = total
	s.runs[run.ID] = run
	return *run, nil
}

func (s *Memory) Approve(_ context.Context, _, id, actor string) (domain.PayrollRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[id]
	if !ok {
		return domain.PayrollRun{}, domain.ErrNotFound
	}
	if run.Status != "IN_REVIEW" {
		return domain.PayrollRun{}, domain.ErrInvalidState
	}
	if actor == run.PreparedBy {
		return domain.PayrollRun{}, domain.ErrSoDViolation
	}
	run.Status = "APPROVED"
	run.ApprovedBy = actor
	run.Version++
	return *run, nil
}

func (s *Memory) ListEmployees(_ context.Context, filter domain.EmployeeFilter) ([]domain.Employee, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]domain.Employee, 0, len(s.employees))
	for i, e := range s.employees {
		if filter.BranchCode != "" && e.BranchID != filter.BranchCode {
			continue
		}
		out = append(out, domain.Employee{
			ID:             e.ID,
			BranchID:       e.BranchID,
			EmployeeNumber: fmt.Sprintf("E-%03d", i+1),
			DisplayName:    e.Name,
			Status:         "ACTIVE",
		})
	}
	return out, nil
}
