-- 003_payroll.sql

CREATE TABLE employees (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id UUID NOT NULL REFERENCES organizations(id),
  branch_id UUID NOT NULL REFERENCES branches(id),
  user_id UUID REFERENCES users(id),
  employee_number TEXT NOT NULL,
  display_name TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'ACTIVE',
  UNIQUE (org_id, employee_number)
);

CREATE TABLE employment_contracts (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  employee_id UUID NOT NULL REFERENCES employees(id),
  contract_type TEXT NOT NULL DEFAULT 'FULL_TIME',
  base_salary_encrypted BYTEA,
  cost_center_id UUID,
  start_date DATE NOT NULL,
  end_date DATE
);

CREATE TABLE payroll_periods (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id UUID NOT NULL REFERENCES organizations(id),
  label TEXT NOT NULL,
  start_date DATE NOT NULL,
  end_date DATE NOT NULL,
  status TEXT NOT NULL DEFAULT 'OPEN',
  UNIQUE (org_id, label)
);

CREATE TABLE payroll_concepts (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id UUID NOT NULL REFERENCES organizations(id),
  code TEXT NOT NULL,
  name TEXT NOT NULL,
  concept_type TEXT NOT NULL, -- EARNING | DEDUCTION
  calc_rule TEXT NOT NULL DEFAULT 'FIXED',
  UNIQUE (org_id, code)
);

CREATE TABLE payroll_runs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  period_id UUID NOT NULL REFERENCES payroll_periods(id),
  branch_id UUID NOT NULL REFERENCES branches(id),
  status TEXT NOT NULL DEFAULT 'OPEN',
  prepared_by UUID REFERENCES users(id),
  total_amount NUMERIC(18,2) NOT NULL DEFAULT 0,
  version INT NOT NULL DEFAULT 1,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE payroll_lines (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  run_id UUID NOT NULL REFERENCES payroll_runs(id) ON DELETE CASCADE,
  employee_id UUID NOT NULL REFERENCES employees(id),
  concept_id UUID NOT NULL REFERENCES payroll_concepts(id),
  amount NUMERIC(18,2) NOT NULL,
  idempotency_key TEXT NOT NULL,
  UNIQUE (run_id, idempotency_key)
);

CREATE TABLE payroll_approvals (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  run_id UUID NOT NULL REFERENCES payroll_runs(id) ON DELETE CASCADE,
  actor_user_id UUID NOT NULL REFERENCES users(id),
  action TEXT NOT NULL,
  at TIMESTAMPTZ NOT NULL DEFAULT now(),
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb
);

-- Prevent self-approval at DB level when both actions exist for same actor prepare+approve
CREATE OR REPLACE FUNCTION enforce_payroll_sod()
RETURNS TRIGGER AS $$
DECLARE
  preparer UUID;
BEGIN
  IF NEW.action = 'APPROVE' THEN
    SELECT prepared_by INTO preparer FROM payroll_runs WHERE id = NEW.run_id;
    IF preparer IS NOT NULL AND preparer = NEW.actor_user_id THEN
      RAISE EXCEPTION 'segregation of duties: preparer cannot approve';
    END IF;
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_payroll_sod
  BEFORE INSERT ON payroll_approvals
  FOR EACH ROW EXECUTE FUNCTION enforce_payroll_sod();
