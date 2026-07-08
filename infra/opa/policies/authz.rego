package nexus.authz

import future.keywords.if
import future.keywords.in

default allow := false

allow if {
  "platform_admin" in input.subject.roles
}

allow if {
  input.action == "inventory.movement.create"
  "inventory.movement.create" in input.subject.permissions
  branch_allowed
  mfa_ok
  adjustment_within_limit
}

allow if {
  input.action == "inventory.balance.read"
  "inventory.balance.read" in input.subject.permissions
  branch_allowed
}

allow if {
  input.action == "inventory.catalog.read"
  catalog_read_allowed
  branch_allowed
}

allow if {
  input.action == "inventory.label.read"
  label_read_allowed
  branch_allowed
}

allow if {
  input.action == "inventory.movement.read"
  "inventory.movement.read" in input.subject.permissions
  branch_allowed
}

# Area / regional managers may void movements only in warehouses they manage.
allow if {
  input.action == "inventory.movement.void"
  "inventory.movement.void" in input.subject.permissions
  branch_allowed
  mfa_ok
  warehouse_managed
}

allow if {
  input.action == "payroll.run.prepare"
  "payroll.run.prepare" in input.subject.permissions
  branch_allowed
  mfa_ok
}

allow if {
  input.action == "payroll.run.approve"
  "payroll.run.approve" in input.subject.permissions
  branch_allowed
  mfa_ok
  not self_approve
  amount_within_limit
}

allow if {
  input.action == "payroll.run.read"
  "payroll.run.read" in input.subject.permissions
  branch_allowed
}

branch_allowed if {
  not input.resource.branch_id
}

branch_allowed if {
  input.resource.branch_id in input.subject.branch_ids
}

branch_allowed if {
  "*" in input.subject.branch_ids
}

mfa_ok if {
  input.context.mfa_level >= 1
}

self_approve if {
  input.resource.prepared_by
  input.resource.prepared_by == input.subject.sub
}

amount_within_limit if {
  not input.resource.total_amount
}

amount_within_limit if {
  input.resource.total_amount <= object.get(input.subject.attrs, "max_payroll_amount", 0)
}

adjustment_within_limit if {
  not input.resource.quantity
}

adjustment_within_limit if {
  qty := input.resource.quantity
  qty >= 0
  qty <= object.get(input.subject.attrs, "max_adjustment", 0)
}

adjustment_within_limit if {
  qty := input.resource.quantity
  qty < 0
  -qty <= object.get(input.subject.attrs, "max_adjustment", 0)
}

warehouse_managed if {
  "platform_admin" in input.subject.roles
}

warehouse_managed if {
  wh := input.resource.warehouse_id
  wh != ""
  managed := object.get(input.subject.attrs, "managed_warehouses", [])
  wh in managed
}

warehouse_managed if {
  "*" in object.get(input.subject.attrs, "managed_warehouses", [])
}

catalog_read_allowed if {
  "inventory.catalog.read" in input.subject.permissions
}

catalog_read_allowed if {
  "inventory.balance.read" in input.subject.permissions
}

label_read_allowed if {
  "inventory.label.read" in input.subject.permissions
}

label_read_allowed if {
  "inventory.balance.read" in input.subject.permissions
}
