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
