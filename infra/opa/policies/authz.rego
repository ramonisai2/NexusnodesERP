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
  input.action == "reporting.read"
  reporting_read_allowed
  branch_allowed
}

allow if {
  input.action == "reporting.catalog.read"
  reporting_catalog_allowed
  branch_allowed
}

allow if {
  input.action == "reporting.export"
  reporting_export_allowed
  branch_allowed
  mfa_ok
}

allow if {
  input.action == "reporting.print"
  reporting_print_allowed
  branch_allowed
  mfa_ok
}

allow if {
  input.action == "reporting.image.read"
  image_report_read_allowed
  branch_allowed
}

allow if {
  input.action == "reporting.image.create"
  "reporting.image.create" in input.subject.permissions
  branch_allowed
  mfa_ok
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

# Clerks request void; bosses decide via approval queue.
allow if {
  input.action == "inventory.movement.void.request"
  "inventory.movement.void.request" in input.subject.permissions
  branch_allowed
  mfa_ok
}

allow if {
  input.action == "approval.read"
  approval_read_allowed
  branch_allowed
}

allow if {
  input.action == "approval.decide"
  "approval.decide" in input.subject.permissions
  branch_allowed
  mfa_ok
}

allow if {
  input.action == "session.operator"
  "session.operator" in input.subject.permissions
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

allow if {
  input.action == "employee.read"
  employee_read_allowed
  branch_allowed
}

allow if {
  input.action == "employee.write"
  employee_write_allowed
  branch_allowed
  mfa_ok
}

allow if {
  input.action == "inventory.adjustment.create"
  adjustment_create_allowed
  branch_allowed
  mfa_ok
  adjustment_within_limit
}

allow if {
  input.action == "inventory.adjustment.read"
  adjustment_read_allowed
  branch_allowed
}

allow if {
  input.action == "reporting.security.read"
  security_report_allowed
  branch_allowed
}

allow if {
  input.action == "inventory.seal.verify"
  seal_verify_allowed
  branch_allowed
  mfa_ok
}

allow if {
  input.action == "search.query"
  search_query_allowed
  branch_allowed
}

allow if {
  input.action == "search.reindex"
  search_reindex_allowed
}

allow if {
  input.action == "inventory.warehouse.read"
  warehouse_read_allowed
  branch_allowed
}

allow if {
  input.action == "inventory.receipt.read"
  receipt_read_allowed
  branch_allowed
}

allow if {
  input.action == "inventory.receipt.create"
  receipt_write_allowed
  branch_allowed
  mfa_ok
}

allow if {
  input.action == "inventory.receipt.post"
  receipt_write_allowed
  branch_allowed
  mfa_ok
}

allow if {
  input.action == "inventory.shipment.read"
  shipment_read_allowed
  branch_allowed
}

allow if {
  input.action == "inventory.shipment.create"
  shipment_write_allowed
  branch_allowed
  mfa_ok
}

allow if {
  input.action == "inventory.shipment.post"
  shipment_write_allowed
  branch_allowed
  mfa_ok
}

allow if {
  input.action == "inventory.slip.read"
  slip_read_allowed
  branch_allowed
}

allow if {
  input.action == "inventory.slip.create"
  slip_write_allowed
  branch_allowed
  mfa_ok
}

allow if {
  input.action == "inventory.slip.print"
  slip_write_allowed
  branch_allowed
  mfa_ok
}

allow if {
  input.action == "inventory.slip.ship"
  slip_lifecycle_allowed
  branch_allowed
  mfa_ok
}

allow if {
  input.action == "inventory.slip.receive"
  slip_lifecycle_allowed
  branch_allowed
  mfa_ok
}

allow if {
  input.action == "inventory.slip.cancel"
  slip_cancel_allowed
  mfa_ok
}

allow if {
  input.action == "inventory.transport.read"
  transport_read_allowed
  branch_allowed
}

allow if {
  input.action == "inventory.transport.create"
  transport_write_allowed
  branch_allowed
  mfa_ok
}

allow if {
  input.action == "inventory.transport.print"
  transport_write_allowed
  branch_allowed
  mfa_ok
}

allow if {
  input.action == "inventory.transport.depart"
  transport_lifecycle_allowed
  branch_allowed
  mfa_ok
}

allow if {
  input.action == "inventory.transport.deliver"
  transport_lifecycle_allowed
  branch_allowed
  mfa_ok
}

allow if {
  input.action == "inventory.transport.cancel"
  transport_cancel_allowed
  mfa_ok
}

allow if {
  input.action == "inventory.parcel.read"
  parcel_read_allowed
  branch_allowed
}

allow if {
  input.action == "inventory.warranty.read"
  warranty_read_allowed
  branch_allowed
}

allow if {
  input.action == "inventory.warranty.create"
  warranty_write_allowed
  branch_allowed
  mfa_ok
}

allow if {
  input.action == "inventory.return.read"
  return_read_allowed
  branch_allowed
}

allow if {
  input.action == "inventory.return.create"
  return_write_allowed
  branch_allowed
  mfa_ok
}

allow if {
  input.action == "inventory.transfer.read"
  transfer_read_allowed
  branch_allowed
}

allow if {
  input.action == "inventory.transfer.create"
  transfer_write_allowed
  branch_allowed
  mfa_ok
}

allow if {
  input.action == "inventory.transfer.ship"
  transfer_ship_allowed
  branch_allowed
  mfa_ok
}

allow if {
  input.action == "inventory.transfer.receive"
  transfer_receive_allowed
  branch_allowed
  mfa_ok
}

allow if {
  input.action == "inventory.transfer.cancel"
  transfer_cancel_allowed
  mfa_ok
}

allow if {
  input.action == "store.storefront.read"
  storefront_read_allowed
  branch_allowed
}

allow if {
  input.action == "store.storefront.manage"
  storefront_manage_allowed
  branch_allowed
  mfa_ok
}

allow if {
  input.action == "store.department.manager.read"
  department_manager_read_allowed
  branch_allowed
}

allow if {
  input.action == "store.department.manager.assign"
  department_manager_assign_allowed
  branch_allowed
  mfa_ok
}

allow if {
  input.action == "pos.sale.read"
  pos_sale_read_allowed
  branch_allowed
}

allow if {
  input.action == "pos.sale.create"
  pos_sale_create_allowed
  branch_allowed
  mfa_ok
}

allow if {
  input.action == "pos.sale.void"
  pos_sale_void_allowed
  branch_allowed
  mfa_ok
}

allow if {
  input.action == "pos.invoice.request"
  pos_invoice_request_allowed
  branch_allowed
  mfa_ok
}

allow if {
  input.action == "pos.invoice.read"
  pos_invoice_read_allowed
  branch_allowed
}

allow if {
  input.action == "pos.settings.manage"
  pos_settings_manage_allowed
  branch_allowed
  mfa_ok
}

allow if {
  input.action == "customer.read"
  customer_read_allowed
  branch_allowed
}

allow if {
  input.action == "customer.manage"
  customer_manage_allowed
  branch_allowed
  mfa_ok
}

allow if {
  input.action == "customer.card.read"
  customer_card_read_allowed
  branch_allowed
}

allow if {
  input.action == "customer.card.manage"
  customer_card_manage_allowed
  branch_allowed
  mfa_ok
}

allow if {
  input.action == "purchasing.order.read"
  purchasing_read_allowed
  branch_allowed
}

allow if {
  input.action == "purchasing.order.create"
  "purchasing.order.create" in input.subject.permissions
  branch_allowed
  mfa_ok
}

allow if {
  input.action == "purchasing.order.approve"
  "purchasing.order.approve" in input.subject.permissions
  branch_allowed
  mfa_ok
}

allow if {
  input.action == "facilities.workorder.read"
  facilities_read_allowed
  branch_allowed
}

allow if {
  input.action == "facilities.workorder.create"
  "facilities.workorder.create" in input.subject.permissions
  branch_allowed
  mfa_ok
}

allow if {
  input.action == "facilities.workorder.close"
  "facilities.workorder.close" in input.subject.permissions
  branch_allowed
  mfa_ok
}

allow if {
  input.action == "mail.read"
  mail_read_allowed
}

allow if {
  input.action == "mail.send"
  mail_send_allowed
  mfa_ok
}

allow if {
  input.action == "mail.announce"
  mail_announce_allowed
  mfa_ok
}

branch_allowed if {
  not input.resource.branch_id
}

branch_allowed if {
  input.resource.branch_id == ""
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

employee_read_allowed if {
  "employee.read" in input.subject.permissions
}

employee_read_allowed if {
  "payroll.run.read" in input.subject.permissions
}

employee_write_allowed if {
  "employee.write" in input.subject.permissions
}

adjustment_create_allowed if {
  "inventory.adjustment.create" in input.subject.permissions
}

adjustment_create_allowed if {
  "inventory.movement.create" in input.subject.permissions
}

adjustment_read_allowed if {
  "inventory.adjustment.read" in input.subject.permissions
}

adjustment_read_allowed if {
  "inventory.movement.read" in input.subject.permissions
}

adjustment_read_allowed if {
  "inventory.balance.read" in input.subject.permissions
}

security_report_allowed if {
  "reporting.security.read" in input.subject.permissions
}

security_report_allowed if {
  "inventory.transport.read" in input.subject.permissions
}

security_report_allowed if {
  "inventory.shipment.read" in input.subject.permissions
}

seal_verify_allowed if {
  "inventory.seal.verify" in input.subject.permissions
}

seal_verify_allowed if {
  "inventory.transport.depart" in input.subject.permissions
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

reporting_read_allowed if {
  "reporting.read" in input.subject.permissions
}

reporting_read_allowed if {
  "inventory.balance.read" in input.subject.permissions
}

reporting_read_allowed if {
  "payroll.run.read" in input.subject.permissions
}

reporting_catalog_allowed if {
  "reporting.catalog.read" in input.subject.permissions
}

reporting_catalog_allowed if {
  "reporting.export" in input.subject.permissions
}

reporting_catalog_allowed if {
  "reporting.print" in input.subject.permissions
}

reporting_catalog_allowed if {
  reporting_read_allowed
}

reporting_export_allowed if {
  "reporting.export" in input.subject.permissions
}

reporting_export_allowed if {
  "platform_admin" in input.subject.roles
}

reporting_export_allowed if {
  "webmaster" in input.subject.roles
}

reporting_print_allowed if {
  "reporting.print" in input.subject.permissions
}

reporting_print_allowed if {
  "platform_admin" in input.subject.roles
}

reporting_print_allowed if {
  "webmaster" in input.subject.roles
}

image_report_read_allowed if {
  "reporting.image.read" in input.subject.permissions
}

image_report_read_allowed if {
  "reporting.read" in input.subject.permissions
}

image_report_read_allowed if {
  "inventory.balance.read" in input.subject.permissions
}

search_query_allowed if {
  "search.query" in input.subject.permissions
}

search_query_allowed if {
  "inventory.balance.read" in input.subject.permissions
}

search_query_allowed if {
  "inventory.catalog.read" in input.subject.permissions
}

search_query_allowed if {
  "reporting.image.read" in input.subject.permissions
}

search_query_allowed if {
  "reporting.read" in input.subject.permissions
}

search_reindex_allowed if {
  "platform_admin" in input.subject.roles
}

search_reindex_allowed if {
  "search.reindex" in input.subject.permissions
}

warehouse_read_allowed if {
  "inventory.warehouse.read" in input.subject.permissions
}

warehouse_read_allowed if {
  "inventory.balance.read" in input.subject.permissions
}

receipt_read_allowed if {
  "inventory.receipt.read" in input.subject.permissions
}

receipt_read_allowed if {
  "inventory.balance.read" in input.subject.permissions
}

receipt_write_allowed if {
  "inventory.receipt.create" in input.subject.permissions
}

receipt_write_allowed if {
  "inventory.receipt.post" in input.subject.permissions
}

receipt_write_allowed if {
  "inventory.movement.create" in input.subject.permissions
}

shipment_read_allowed if {
  "inventory.shipment.read" in input.subject.permissions
}

shipment_read_allowed if {
  "inventory.receipt.read" in input.subject.permissions
}

shipment_read_allowed if {
  "inventory.balance.read" in input.subject.permissions
}

shipment_write_allowed if {
  "inventory.shipment.create" in input.subject.permissions
}

shipment_write_allowed if {
  "inventory.shipment.post" in input.subject.permissions
}

shipment_write_allowed if {
  "inventory.receipt.create" in input.subject.permissions
}

shipment_write_allowed if {
  "inventory.movement.create" in input.subject.permissions
}

slip_read_allowed if {
  "inventory.slip.read" in input.subject.permissions
}

slip_read_allowed if {
  "inventory.balance.read" in input.subject.permissions
}

slip_write_allowed if {
  "inventory.slip.create" in input.subject.permissions
}

slip_write_allowed if {
  "inventory.slip.print" in input.subject.permissions
}

slip_write_allowed if {
  "inventory.movement.create" in input.subject.permissions
}

slip_lifecycle_allowed if {
  "inventory.slip.ship" in input.subject.permissions
}

slip_lifecycle_allowed if {
  "inventory.slip.receive" in input.subject.permissions
}

slip_lifecycle_allowed if {
  "inventory.slip.create" in input.subject.permissions
}

slip_lifecycle_allowed if {
  "inventory.movement.create" in input.subject.permissions
}

slip_cancel_allowed if {
  "inventory.slip.cancel" in input.subject.permissions
}

slip_cancel_allowed if {
  "warehouse_manager" in input.subject.roles
}

slip_cancel_allowed if {
  "regional_manager" in input.subject.roles
}

slip_cancel_allowed if {
  "store_owner" in input.subject.roles
}

slip_cancel_allowed if {
  "platform_admin" in input.subject.roles
}

transport_read_allowed if {
  "inventory.transport.read" in input.subject.permissions
}

transport_read_allowed if {
  "inventory.balance.read" in input.subject.permissions
}

transport_write_allowed if {
  "inventory.transport.create" in input.subject.permissions
}

transport_write_allowed if {
  "inventory.transport.print" in input.subject.permissions
}

transport_write_allowed if {
  "inventory.movement.create" in input.subject.permissions
}

transport_lifecycle_allowed if {
  "inventory.transport.depart" in input.subject.permissions
}

transport_lifecycle_allowed if {
  "inventory.transport.deliver" in input.subject.permissions
}

transport_lifecycle_allowed if {
  "inventory.transport.create" in input.subject.permissions
}

transport_lifecycle_allowed if {
  "inventory.movement.create" in input.subject.permissions
}

transport_cancel_allowed if {
  "inventory.transport.cancel" in input.subject.permissions
}

transport_cancel_allowed if {
  "warehouse_manager" in input.subject.roles
}

transport_cancel_allowed if {
  "regional_manager" in input.subject.roles
}

transport_cancel_allowed if {
  "store_owner" in input.subject.roles
}

transport_cancel_allowed if {
  "platform_admin" in input.subject.roles
}

parcel_read_allowed if {
  "inventory.parcel.read" in input.subject.permissions
}

parcel_read_allowed if {
  "inventory.slip.read" in input.subject.permissions
}

parcel_read_allowed if {
  "inventory.balance.read" in input.subject.permissions
}

warranty_read_allowed if {
  "inventory.warranty.read" in input.subject.permissions
}

warranty_read_allowed if {
  "inventory.parcel.read" in input.subject.permissions
}

warranty_read_allowed if {
  "inventory.balance.read" in input.subject.permissions
}

warranty_write_allowed if {
  "inventory.warranty.create" in input.subject.permissions
}

warranty_write_allowed if {
  "inventory.slip.create" in input.subject.permissions
}

warranty_write_allowed if {
  "inventory.movement.create" in input.subject.permissions
}

return_read_allowed if {
  "inventory.return.read" in input.subject.permissions
}

return_read_allowed if {
  "inventory.parcel.read" in input.subject.permissions
}

return_read_allowed if {
  "inventory.balance.read" in input.subject.permissions
}

return_write_allowed if {
  "inventory.return.create" in input.subject.permissions
}

return_write_allowed if {
  "inventory.slip.create" in input.subject.permissions
}

return_write_allowed if {
  "inventory.movement.create" in input.subject.permissions
}

transfer_read_allowed if {
  "inventory.transfer.read" in input.subject.permissions
}

transfer_read_allowed if {
  "inventory.balance.read" in input.subject.permissions
}

transfer_write_allowed if {
  "inventory.transfer.create" in input.subject.permissions
}

transfer_write_allowed if {
  "inventory.movement.create" in input.subject.permissions
}

transfer_ship_allowed if {
  "inventory.transfer.ship" in input.subject.permissions
}

transfer_ship_allowed if {
  "inventory.transfer.create" in input.subject.permissions
}

transfer_ship_allowed if {
  "inventory.movement.create" in input.subject.permissions
}

transfer_receive_allowed if {
  "inventory.transfer.receive" in input.subject.permissions
}

transfer_receive_allowed if {
  "inventory.transfer.create" in input.subject.permissions
}

transfer_receive_allowed if {
  "inventory.movement.create" in input.subject.permissions
}

transfer_cancel_allowed if {
  "inventory.transfer.cancel" in input.subject.permissions
}

transfer_cancel_allowed if {
  "warehouse_manager" in input.subject.roles
}

transfer_cancel_allowed if {
  "regional_manager" in input.subject.roles
}

transfer_cancel_allowed if {
  "store_owner" in input.subject.roles
}

transfer_cancel_allowed if {
  "platform_admin" in input.subject.roles
}

storefront_read_allowed if {
  "store.storefront.read" in input.subject.permissions
}

storefront_read_allowed if {
  "store.storefront.manage" in input.subject.permissions
}

storefront_read_allowed if {
  "inventory.balance.read" in input.subject.permissions
}

storefront_manage_allowed if {
  "store.storefront.manage" in input.subject.permissions
}

storefront_manage_allowed if {
  "store_owner" in input.subject.roles
}

storefront_manage_allowed if {
  "platform_admin" in input.subject.roles
}

department_manager_read_allowed if {
  "store.department.manager.read" in input.subject.permissions
}

department_manager_read_allowed if {
  "store.department.manager.assign" in input.subject.permissions
}

department_manager_read_allowed if {
  "inventory.balance.read" in input.subject.permissions
}

department_manager_assign_allowed if {
  "store.department.manager.assign" in input.subject.permissions
}

department_manager_assign_allowed if {
  "store_owner" in input.subject.roles
}

department_manager_assign_allowed if {
  "regional_manager" in input.subject.roles
}

department_manager_assign_allowed if {
  "platform_admin" in input.subject.roles
}

pos_sale_read_allowed if {
  "pos.sale.read" in input.subject.permissions
}

pos_sale_read_allowed if {
  "pos.sale.create" in input.subject.permissions
}

pos_sale_read_allowed if {
  "inventory.balance.read" in input.subject.permissions
}

pos_sale_create_allowed if {
  "pos.sale.create" in input.subject.permissions
}

pos_sale_create_allowed if {
  "store_owner" in input.subject.roles
}

pos_sale_create_allowed if {
  "platform_admin" in input.subject.roles
}

pos_sale_void_allowed if {
  "pos.sale.void" in input.subject.permissions
}

pos_sale_void_allowed if {
  "store_owner" in input.subject.roles
}

pos_sale_void_allowed if {
  "platform_admin" in input.subject.roles
}

pos_invoice_request_allowed if {
  "pos.invoice.request" in input.subject.permissions
}

pos_invoice_request_allowed if {
  "pos.sale.create" in input.subject.permissions
}

pos_invoice_read_allowed if {
  "pos.invoice.read" in input.subject.permissions
}

pos_invoice_read_allowed if {
  "pos.sale.read" in input.subject.permissions
}

pos_settings_manage_allowed if {
  "pos.settings.manage" in input.subject.permissions
}

pos_settings_manage_allowed if {
  "store_owner" in input.subject.roles
}

pos_settings_manage_allowed if {
  "platform_admin" in input.subject.roles
}

customer_read_allowed if {
  "customer.read" in input.subject.permissions
}

customer_read_allowed if {
  "customer.manage" in input.subject.permissions
}

customer_read_allowed if {
  "inventory.balance.read" in input.subject.permissions
}

customer_manage_allowed if {
  "customer.manage" in input.subject.permissions
}

customer_manage_allowed if {
  "store_owner" in input.subject.roles
}

customer_manage_allowed if {
  "platform_admin" in input.subject.roles
}

customer_card_read_allowed if {
  "customer.card.read" in input.subject.permissions
}

customer_card_read_allowed if {
  "customer.read" in input.subject.permissions
}

customer_card_read_allowed if {
  "inventory.balance.read" in input.subject.permissions
}

customer_card_manage_allowed if {
  "customer.card.manage" in input.subject.permissions
}

customer_card_manage_allowed if {
  "customer.manage" in input.subject.permissions
}

customer_card_manage_allowed if {
  "inventory.movement.create" in input.subject.permissions
}

mail_read_allowed if {
  "mail.read" in input.subject.permissions
}

mail_read_allowed if {
  "mail.send" in input.subject.permissions
}

mail_read_allowed if {
  "inventory.balance.read" in input.subject.permissions
}

mail_send_allowed if {
  "mail.send" in input.subject.permissions
}

mail_send_allowed if {
  "inventory.balance.read" in input.subject.permissions
}

mail_announce_allowed if {
  "mail.announce" in input.subject.permissions
}

mail_announce_allowed if {
  "platform_admin" in input.subject.roles
}

mail_announce_allowed if {
  "store_owner" in input.subject.roles
}

mail_announce_allowed if {
  "regional_manager" in input.subject.roles
}

mail_announce_allowed if {
  "warehouse_manager" in input.subject.roles
}

mail_announce_allowed if {
  "payroll_approver" in input.subject.roles
}

approval_read_allowed if {
  "approval.read" in input.subject.permissions
}

approval_read_allowed if {
  "approval.decide" in input.subject.permissions
}

purchasing_read_allowed if {
  "purchasing.order.read" in input.subject.permissions
}

purchasing_read_allowed if {
  "purchasing.order.create" in input.subject.permissions
}

purchasing_read_allowed if {
  "purchasing.order.approve" in input.subject.permissions
}

facilities_read_allowed if {
  "facilities.workorder.read" in input.subject.permissions
}

facilities_read_allowed if {
  "facilities.workorder.create" in input.subject.permissions
}

facilities_read_allowed if {
  "facilities.workorder.close" in input.subject.permissions
}
