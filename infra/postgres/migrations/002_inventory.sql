-- 002_inventory.sql

CREATE TABLE warehouses (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  branch_id UUID NOT NULL REFERENCES branches(id),
  code TEXT NOT NULL,
  name TEXT NOT NULL,
  allow_negative BOOLEAN NOT NULL DEFAULT FALSE,
  UNIQUE (branch_id, code)
);

CREATE TABLE products (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id UUID NOT NULL REFERENCES organizations(id),
  sku_base TEXT NOT NULL,
  name TEXT NOT NULL,
  UNIQUE (org_id, sku_base)
);

CREATE TABLE product_skus (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  product_id UUID NOT NULL REFERENCES products(id),
  sku TEXT NOT NULL UNIQUE,
  uom TEXT NOT NULL DEFAULT 'EA'
);

CREATE TABLE stock_balances (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  warehouse_id UUID NOT NULL REFERENCES warehouses(id),
  sku_id UUID NOT NULL REFERENCES product_skus(id),
  on_hand NUMERIC(18,4) NOT NULL DEFAULT 0,
  reserved NUMERIC(18,4) NOT NULL DEFAULT 0,
  version INT NOT NULL DEFAULT 1,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (warehouse_id, sku_id),
  CHECK (reserved >= 0)
);

CREATE TABLE inventory_movements (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id UUID NOT NULL REFERENCES organizations(id),
  branch_id UUID NOT NULL REFERENCES branches(id),
  sku_id UUID NOT NULL REFERENCES product_skus(id),
  warehouse_id UUID NOT NULL REFERENCES warehouses(id),
  movement_type TEXT NOT NULL,
  quantity NUMERIC(18,4) NOT NULL,
  status TEXT NOT NULL DEFAULT 'POSTED',
  posted_by UUID REFERENCES users(id),
  idempotency_key TEXT NOT NULL,
  version INT NOT NULL DEFAULT 1,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (org_id, idempotency_key)
);

CREATE TABLE stock_reservations (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  stock_balance_id UUID NOT NULL REFERENCES stock_balances(id),
  quantity NUMERIC(18,4) NOT NULL CHECK (quantity > 0),
  status TEXT NOT NULL DEFAULT 'ACTIVE',
  expires_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE outbox (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  event_type TEXT NOT NULL,
  payload JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  published_at TIMESTAMPTZ
);

CREATE INDEX inventory_movements_branch_created_idx
  ON inventory_movements (branch_id, created_at DESC);
