package fixture

// Case 是一条 E2E 用例的登记项。文档条目与代码用例必须双向对应，
// 任一方向缺失都被机械检出，避免「文档写 153 条、实际 40 条」的隐性债。
type Case struct {
	ID       string
	Priority string // P0 / P1 / P2 / P3
	Build    string // "" | fault | platform | perf
}

// Registry 是全部已实现用例的唯一清单。
// 新增用例必须先在此登记，否则 TestRegistryComplete 会红。
var Registry = []Case{
	// 4.1 入口契约
	{"E2E-IO-001", "P0", ""},
	{"E2E-IO-002", "P0", ""},
	{"E2E-IO-003", "P0", ""},
	{"E2E-IO-004", "P0", ""},
	{"E2E-IO-005", "P0", ""},
	{"E2E-IO-006", "P0", ""},
	{"E2E-IO-007", "P0", ""},
	{"E2E-IO-008", "P0", ""},
	{"E2E-IO-009", "P0", ""},
	{"E2E-IO-010", "P0", ""},
	{"E2E-IO-011", "P0", ""},
	{"E2E-IO-012", "P0", ""},
	{"E2E-IO-013", "P0", ""},
	{"E2E-IO-014", "P0", ""},
	{"E2E-IO-015", "P0", ""},
	{"E2E-IO-016", "P1", ""},

	// 4.2 init
	{"E2E-INIT-001", "P0", ""},
	{"E2E-INIT-002", "P0", ""},
	{"E2E-INIT-003", "P0", ""},
	{"E2E-INIT-004", "P0", ""},
	{"E2E-INIT-005", "P0", ""},
	{"E2E-INIT-006", "P0", ""},
	{"E2E-INIT-007", "P1", ""},
	{"E2E-INIT-008", "P1", ""},
	{"E2E-INIT-009", "P0", ""},
	{"E2E-INIT-010", "P0", ""},
	{"E2E-INIT-011", "P0", ""},
	{"E2E-INIT-012", "P0", ""},
	{"E2E-INIT-013", "P0", ""},
	{"E2E-INIT-016", "P1", ""},
	{"E2E-INIT-017", "P1", ""},

	// 4.3 adopt
	{"E2E-ADOPT-001", "P1", ""},
	{"E2E-ADOPT-002", "P1", ""},
	{"E2E-ADOPT-003", "P1", ""},
	{"E2E-ADOPT-004", "P0", ""},
	{"E2E-ADOPT-005", "P1", ""},
	{"E2E-ADOPT-006", "P1", ""},
	{"E2E-ADOPT-007", "P1", ""},
	{"E2E-ADOPT-009", "P1", ""},
	{"E2E-ADOPT-010", "P1", ""},

	// 4.4 sync
	{"E2E-SYNC-001", "P1", ""},
	{"E2E-SYNC-002", "P1", ""},
	{"E2E-SYNC-003", "P1", ""},
	{"E2E-SYNC-004", "P1", ""},
	{"E2E-SYNC-005", "P1", ""},
	{"E2E-SYNC-007", "P1", ""},
	{"E2E-SYNC-008", "P1", ""},

	// 4.5 plan
	{"E2E-PLAN-001", "P1", ""},
	{"E2E-PLAN-002", "P1", ""},
	{"E2E-PLAN-003", "P1", ""},
	{"E2E-PLAN-004", "P1", ""},
	{"E2E-PLAN-005", "P1", ""},
	{"E2E-PLAN-006", "P1", ""},

	// 4.6 check
	{"E2E-CHECK-001", "P0", ""},
	{"E2E-CHECK-002", "P0", ""},
	{"E2E-CHECK-003", "P0", ""},
	{"E2E-CHECK-004", "P0", ""},
	{"E2E-CHECK-008", "P0", ""},
	{"E2E-CHECK-009", "P1", ""},
	{"E2E-CHECK-011", "P0", ""},
	{"E2E-CHECK-012", "P0", ""},
	{"E2E-CHECK-014", "P1", ""},
	{"E2E-CHECK-015", "P0", ""},

	// 4.7 verify
	{"E2E-VERIFY-001", "P0", ""},
	{"E2E-VERIFY-002", "P0", ""},
	{"E2E-VERIFY-003", "P0", ""},
	{"E2E-VERIFY-008", "P1", ""},

	// 4.10 journey
	{"E2E-JOURNEY-J1", "P0", ""},
	{"E2E-JOURNEY-J2", "P0", ""},
	{"E2E-JOURNEY-J5", "P1", ""},

	// 4.15 self
	{"E2E-SELF-001", "P0", ""},
	{"E2E-SELF-002", "P0", ""},
	{"E2E-SELF-003", "P0", ""},
	{"E2E-SELF-004", "P0", ""},
	{"E2E-SELF-005", "P0", ""},
	{"E2E-SELF-006", "P0", ""},
	{"E2E-SELF-007", "P0", ""},

	// 补充：INIT 产物自检
	{"E2E-INIT-014", "P1", ""},
	{"E2E-INIT-015", "P1", ""},

	// 补充：CHECK 引擎与自证
	{"E2E-CHECK-005", "P0", ""},
	{"E2E-CHECK-006", "P0", ""},
	{"E2E-CHECK-007", "P0", ""},
	{"E2E-CHECK-010", "P0", ""},
	{"E2E-CHECK-013", "P0", ""},
	{"E2E-CHECK-016", "P1", ""},
	{"E2E-CHECK-017", "P0", ""},
	{"E2E-CHECK-018", "P0", ""},

	// 补充：VERIFY 安全项
	{"E2E-VERIFY-004", "P0", ""},
	{"E2E-VERIFY-005", "P0", ""},
	{"E2E-VERIFY-006", "P1", ""},
	{"E2E-VERIFY-007", "P1", ""},
	{"E2E-VERIFY-009", "P0", ""},
	{"E2E-VERIFY-010", "P0", ""},
	{"E2E-VERIFY-011", "P1", ""},
	{"E2E-VERIFY-012", "P1", ""},
	{"E2E-VERIFY-013", "P1", ""},
	{"E2E-VERIFY-014", "P0", ""},
	{"E2E-VERIFY-015", "P1", ""},

	// 补充：SYNC 边界
	{"E2E-SYNC-006", "P1", ""},
	{"E2E-SYNC-009", "P1", ""},
	{"E2E-SYNC-010", "P1", ""},

	// 补充：ADOPT / PLAN
	{"E2E-ADOPT-008", "P1", ""},
	{"E2E-PLAN-007", "P0", ""},

	// 4.11 fault
	{"E2E-FAULT-001", "P0", ""},
	{"E2E-FAULT-002", "P0", ""},
	{"E2E-FAULT-003", "P0", ""},
	{"E2E-FAULT-004", "P0", ""},
	{"E2E-FAULT-005", "P0", ""},
	{"E2E-FAULT-006", "P0", ""},
	{"E2E-FAULT-007", "P0", ""},
	{"E2E-FAULT-008", "P0", ""},

	// 4.12 platform
	{"E2E-PLAT-001", "P0", ""},
	{"E2E-PLAT-002", "P0", ""},
	{"E2E-PLAT-003", "P0", ""},
	{"E2E-PLAT-004", "P0", ""},
	{"E2E-PLAT-005", "P1", ""},
	{"E2E-PLAT-006", "P0", ""},

	// 4.13 perf
	{"E2E-PERF-001", "P0", ""},
	{"E2E-PERF-002", "P0", ""},
	{"E2E-PERF-003", "P1", "perf"},
	{"E2E-PERF-004", "P0", ""},
}

// IDs 便于按族筛选。
func IDs(prefix string) []string {
	var out []string
	for _, c := range Registry {
		if len(prefix) <= len(c.ID) && c.ID[:len(prefix)] == prefix {
			out = append(out, c.ID)
		}
	}
	return out
}
