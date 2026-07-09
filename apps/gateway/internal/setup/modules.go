package setup

// ModuleDef describes an install-time feature that can be unlocked or locked.
type ModuleDef struct {
	Code         string   `json:"code"`
	Required     bool     `json:"required"`
	DefaultOn    bool     `json:"default_on"`
	Permissions  []string `json:"permissions"`
	NavIDs       []string `json:"nav_ids"`
	LabelKey     string   `json:"label_key"`
	HintKey      string   `json:"hint_key"`
}

// ModuleCatalog is the install-time lock/unlock matrix.
func ModuleCatalog() []ModuleDef {
	return []ModuleDef{
		{
			Code:      "inventory",
			Required:  true,
			DefaultOn: true,
			LabelKey:  "modInventory",
			HintKey:   "modInventoryHint",
			NavIDs:    []string{"nav.inventory", "nav.search"},
			Permissions: []string{
				"inventory.balance.read", "inventory.movement.create", "inventory.movement.read",
				"inventory.catalog.read", "inventory.label.read", "inventory.warehouse.read",
				"inventory.movement.void.request", "session.operator", "store.setup.read",
			},
		},
		{
			Code:      "receiving",
			Required:  false,
			DefaultOn: true,
			LabelKey:  "modReceiving",
			HintKey:   "modReceivingHint",
			NavIDs:    []string{"nav.receiving"},
			Permissions: []string{
				"inventory.receipt.read", "inventory.receipt.create", "inventory.receipt.post",
			},
		},
		{
			Code:      "adjustments",
			Required:  false,
			DefaultOn: false,
			LabelKey:  "modAdjustments",
			HintKey:   "modAdjustmentsHint",
			NavIDs:    []string{"nav.adjustments"},
			Permissions: []string{
				"inventory.adjustment.create", "inventory.adjustment.read",
			},
		},
		{
			Code:      "pos",
			Required:  false,
			DefaultOn: true,
			LabelKey:  "modPos",
			HintKey:   "modPosHint",
			NavIDs:    []string{"nav.pos"},
			Permissions: []string{
				"pos.sale.read", "pos.sale.create", "pos.sale.void",
				"pos.invoice.request", "pos.invoice.read", "pos.settings.manage",
			},
		},
		{
			Code:      "card_payments",
			Required:  false,
			DefaultOn: false,
			LabelKey:  "modCardPayments",
			HintKey:   "modCardPaymentsHint",
			NavIDs:    []string{"nav.cardWaits"},
			Permissions: []string{
				"pos.card.wait.read", "pos.card.wait.create",
				"pos.card.wait.confirm", "pos.card.wait.cancel",
			},
		},
		{
			Code:      "customers",
			Required:  false,
			DefaultOn: true,
			LabelKey:  "modCustomers",
			HintKey:   "modCustomersHint",
			NavIDs:    []string{"nav.customers"},
			Permissions: []string{
				"customer.read", "customer.manage", "customer.card.read", "customer.card.manage",
			},
		},
		{
			Code:      "storefront",
			Required:  false,
			DefaultOn: false,
			LabelKey:  "modStorefront",
			HintKey:   "modStorefrontHint",
			NavIDs:    []string{"nav.storefront"},
			Permissions: []string{
				"store.storefront.read", "store.storefront.manage",
			},
		},
		{
			Code:      "logistics",
			Required:  false,
			DefaultOn: false,
			LabelKey:  "modLogistics",
			HintKey:   "modLogisticsHint",
			NavIDs:    []string{"nav.slips", "nav.transport", "nav.transfers", "nav.parcels"},
			Permissions: []string{
				"inventory.slip.read", "inventory.slip.create", "inventory.slip.print",
				"inventory.slip.ship", "inventory.slip.receive",
				"inventory.transport.read", "inventory.transport.create", "inventory.transport.print",
				"inventory.transfer.read", "inventory.transfer.create", "inventory.transfer.ship", "inventory.transfer.receive",
				"inventory.parcel.read", "inventory.warranty.read", "inventory.return.read",
			},
		},
		{
			Code:      "reports",
			Required:  false,
			DefaultOn: true,
			LabelKey:  "modReports",
			HintKey:   "modReportsHint",
			NavIDs:    []string{"nav.reports", "nav.imageReports", "nav.webmasterReports"},
			Permissions: []string{
				"reporting.read", "reporting.image.read", "reporting.image.create",
				"reporting.export", "reporting.print", "reporting.catalog.read",
			},
		},
		{
			Code:      "security",
			Required:  false,
			DefaultOn: false,
			LabelKey:  "modSecurity",
			HintKey:   "modSecurityHint",
			NavIDs:    []string{"nav.security"},
			Permissions: []string{
				"reporting.security.read", "inventory.seal.verify",
				"inventory.transport.read", "inventory.shipment.read",
			},
		},
		{
			Code:      "mail",
			Required:  false,
			DefaultOn: false,
			LabelKey:  "modMail",
			HintKey:   "modMailHint",
			NavIDs:    []string{"nav.mail"},
			Permissions: []string{
				"mail.read", "mail.send", "mail.announce",
			},
		},
		{
			Code:      "approvals",
			Required:  false,
			DefaultOn: false,
			LabelKey:  "modApprovals",
			HintKey:   "modApprovalsHint",
			NavIDs:    []string{"nav.approvals"},
			Permissions: []string{
				"approval.read", "approval.decide",
			},
		},
		{
			Code:      "hr",
			Required:  false,
			DefaultOn: false,
			LabelKey:  "modHr",
			HintKey:   "modHrHint",
			NavIDs:    []string{"nav.hr", "nav.payroll"},
			Permissions: []string{
				"employee.read", "employee.write", "payroll.run.read", "payroll.run.prepare",
			},
		},
		{
			Code:      "dept_managers",
			Required:  false,
			DefaultOn: false,
			LabelKey:  "modDeptManagers",
			HintKey:   "modDeptManagersHint",
			NavIDs:    []string{"nav.deptManagers"},
			Permissions: []string{
				"store.department.manager.read", "store.department.manager.assign",
			},
		},
		{
			Code:      "purchasing",
			Required:  false,
			DefaultOn: false,
			LabelKey:  "modPurchasing",
			HintKey:   "modPurchasingHint",
			NavIDs:    []string{},
			Permissions: []string{
				"purchasing.order.read", "purchasing.order.create",
			},
		},
		{
			Code:      "facilities",
			Required:  false,
			DefaultOn: false,
			LabelKey:  "modFacilities",
			HintKey:   "modFacilitiesHint",
			NavIDs:    []string{},
			Permissions: []string{
				"facilities.workorder.read", "facilities.workorder.create", "facilities.workorder.close",
			},
		},
	}
}

// DefaultEnabledModules returns codes that are on by default (including required).
func DefaultEnabledModules() []string {
	out := make([]string, 0)
	for _, m := range ModuleCatalog() {
		if m.Required || m.DefaultOn {
			out = append(out, m.Code)
		}
	}
	return out
}

// NormalizeEnabledModules forces required modules on and drops unknown codes.
func NormalizeEnabledModules(selected []string) []string {
	catalog := ModuleCatalog()
	byCode := map[string]ModuleDef{}
	for _, m := range catalog {
		byCode[m.Code] = m
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(selected)+2)
	for _, m := range catalog {
		if m.Required {
			seen[m.Code] = true
			out = append(out, m.Code)
		}
	}
	if len(selected) == 0 {
		for _, m := range catalog {
			if m.DefaultOn && !seen[m.Code] {
				seen[m.Code] = true
				out = append(out, m.Code)
			}
		}
		return out
	}
	for _, code := range selected {
		m, ok := byCode[code]
		if !ok || seen[m.Code] {
			continue
		}
		seen[m.Code] = true
		out = append(out, m.Code)
	}
	return out
}

// PermissionsForModules unions permission codes for the enabled module set.
func PermissionsForModules(enabled []string) []string {
	enabled = NormalizeEnabledModules(enabled)
	on := map[string]bool{}
	for _, c := range enabled {
		on[c] = true
	}
	seen := map[string]bool{}
	out := make([]string, 0, 64)
	for _, m := range ModuleCatalog() {
		if !on[m.Code] {
			continue
		}
		for _, p := range m.Permissions {
			if seen[p] {
				continue
			}
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}

// ModuleEnabled reports whether code is in the enabled list.
// Empty/nil list means "all unlocked" (enterprise / legacy).
func ModuleEnabled(enabled []string, code string) bool {
	if len(enabled) == 0 {
		return true
	}
	for _, c := range enabled {
		if c == code {
			return true
		}
	}
	return false
}
