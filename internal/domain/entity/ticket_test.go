package entity

import "testing"

func TestAllowedTicketTransition(t *testing.T) {
	cases := []struct {
		current string
		next    string
		allow   bool
	}{
		// open
		{TicketStatusOpen, TicketStatusInProgress, true},
		{TicketStatusOpen, TicketStatusResolved, false},
		{TicketStatusOpen, TicketStatusClosed, false},
		{TicketStatusOpen, TicketStatusReopened, false},
		{TicketStatusOpen, TicketStatusOpen, false},

		// in_progress
		{TicketStatusInProgress, TicketStatusResolved, true},
		{TicketStatusInProgress, TicketStatusClosed, false},
		{TicketStatusInProgress, TicketStatusOpen, false},

		// resolved
		{TicketStatusResolved, TicketStatusInProgress, true},
		{TicketStatusResolved, TicketStatusClosed, true},
		{TicketStatusResolved, TicketStatusOpen, false},
		{TicketStatusResolved, TicketStatusReopened, false},

		// closed
		{TicketStatusClosed, TicketStatusReopened, true},
		{TicketStatusClosed, TicketStatusOpen, false},
		{TicketStatusClosed, TicketStatusInProgress, false},
		{TicketStatusClosed, TicketStatusResolved, false},
		{TicketStatusClosed, TicketStatusClosed, false},

		// reopened
		{TicketStatusReopened, TicketStatusInProgress, true},
		{TicketStatusReopened, TicketStatusClosed, false},
		{TicketStatusReopened, TicketStatusResolved, false},
		{TicketStatusReopened, TicketStatusOpen, false},

		// invalid inputs
		{"unknown", TicketStatusOpen, false},
		{TicketStatusOpen, "unknown", false},
		{"", "", false},
	}
	for _, tc := range cases {
		got := AllowedTicketTransition(tc.current, tc.next)
		if got != tc.allow {
			t.Errorf("AllowedTicketTransition(%q -> %q) = %v, want %v", tc.current, tc.next, got, tc.allow)
		}
	}
}

func TestTicket_CanTransitionTo(t *testing.T) {
	ticket := &Ticket{Status: TicketStatusOpen}
	if !ticket.CanTransitionTo(TicketStatusInProgress) {
		t.Fatal("open -> in_progress should be allowed")
	}
	if ticket.CanTransitionTo(TicketStatusResolved) {
		t.Fatal("open -> resolved should NOT be allowed")
	}
}

func TestIsValidTicketStatusAndPriority(t *testing.T) {
	if !IsValidTicketStatus(TicketStatusInProgress) {
		t.Error("in_progress should be valid status")
	}
	if IsValidTicketStatus("nope") {
		t.Error("'nope' should be invalid status")
	}
	if !IsValidTicketPriority(TicketPriorityUrgent) {
		t.Error("urgent should be valid priority")
	}
	if IsValidTicketPriority("emergency") {
		t.Error("'emergency' should be invalid priority")
	}
}
