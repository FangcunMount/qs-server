package role

import (
	"testing"
	"time"

	"github.com/fangcun-mount/qs-server/internal/apiserver/domain/user"
)

func TestNewAuditor(t *testing.T) {
	userID := user.NewUserID(1001)
	name := "张三"
	employeeID := "EMP001"

	auditor := NewAuditor(userID, name, employeeID)

	if auditor.GetUserID() != userID {
		t.Errorf("Expected UserID %v, got %v", userID, auditor.GetUserID())
	}

	if auditor.GetName() != name {
		t.Errorf("Expected Name %s, got %s", name, auditor.GetName())
	}

	if auditor.GetEmployeeID() != employeeID {
		t.Errorf("Expected EmployeeID %s, got %s", employeeID, auditor.GetEmployeeID())
	}

	// 默认状态应该是在职
	if auditor.GetStatus() != StatusOnDuty {
		t.Errorf("Expected Status %v, got %v", StatusOnDuty, auditor.GetStatus())
	}

	// 默认应该是活跃的
	if !auditor.IsActive() {
		t.Error("New auditor should be active")
	}

	// 默认应该可以审核
	if !auditor.CanAudit() {
		t.Error("New auditor should be able to audit")
	}
}

func TestAuditorWithDepartment(t *testing.T) {
	auditor := NewAuditor(user.NewUserID(1001), "张三", "EMP001")
	department := "人力资源部"

	auditor.WithDepartment(department)

	if auditor.GetDepartment() != department {
		t.Errorf("Expected Department %s, got %s", department, auditor.GetDepartment())
	}
}

func TestAuditorWithPosition(t *testing.T) {
	auditor := NewAuditor(user.NewUserID(1001), "张三", "EMP001")
	position := "高级审核员"

	auditor.WithPosition(position)

	if auditor.GetPosition() != position {
		t.Errorf("Expected Position %s, got %s", position, auditor.GetPosition())
	}
}

func TestAuditorWithStatus(t *testing.T) {
	auditor := NewAuditor(user.NewUserID(1001), "张三", "EMP001")

	tests := []struct {
		name      string
		status    Status
		isActive  bool
		canAudit  bool
	}{
		{"在职状态", StatusOnDuty, true, true},
		{"休假状态", StatusOnLeave, false, true},
		{"停职状态", StatusSuspended, false, false},
		{"离职状态", StatusResigned, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auditor.WithStatus(tt.status)

			if auditor.GetStatus() != tt.status {
				t.Errorf("Expected Status %v, got %v", tt.status, auditor.GetStatus())
			}

			if auditor.IsActive() != tt.isActive {
				t.Errorf("Expected IsActive %v, got %v", tt.isActive, auditor.IsActive())
			}

			if auditor.CanAudit() != tt.canAudit {
				t.Errorf("Expected CanAudit %v, got %v", tt.canAudit, auditor.CanAudit())
			}
		})
	}
}

func TestAuditorWithHiredAt(t *testing.T) {
	auditor := NewAuditor(user.NewUserID(1001), "张三", "EMP001")
	hiredAt := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)

	auditor.WithHiredAt(hiredAt)

	if !auditor.GetHiredAt().Equal(hiredAt) {
		t.Errorf("Expected HiredAt %v, got %v", hiredAt, auditor.GetHiredAt())
	}
}

func TestAuditorChainedMethods(t *testing.T) {
	userID := user.NewUserID(1001)
	auditor := NewAuditor(userID, "张三", "EMP001").
		WithDepartment("人力资源部").
		WithPosition("高级审核员").
		WithStatus(StatusOnDuty).
		WithHiredAt(time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC))

	if auditor.GetDepartment() != "人力资源部" {
		t.Errorf("Expected Department '人力资源部', got '%s'", auditor.GetDepartment())
	}

	if auditor.GetPosition() != "高级审核员" {
		t.Errorf("Expected Position '高级审核员', got '%s'", auditor.GetPosition())
	}

	if auditor.GetStatus() != StatusOnDuty {
		t.Errorf("Expected Status %v, got %v", StatusOnDuty, auditor.GetStatus())
	}
}

func TestStatusString(t *testing.T) {
	tests := []struct {
		status   Status
		expected string
	}{
		{StatusOnDuty, "on_duty"},
		{StatusOnLeave, "on_leave"},
		{StatusSuspended, "suspended"},
		{StatusResigned, "resigned"},
		{Status(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.status.String(); got != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, got)
			}
		})
	}
}

func TestStatusValue(t *testing.T) {
	tests := []struct {
		status   Status
		expected uint8
	}{
		{StatusOnDuty, 1},
		{StatusOnLeave, 2},
		{StatusSuspended, 3},
		{StatusResigned, 4},
	}

	for _, tt := range tests {
		t.Run(tt.status.String(), func(t *testing.T) {
			if got := tt.status.Value(); got != tt.expected {
				t.Errorf("Expected %d, got %d", tt.expected, got)
			}
		})
	}
}
