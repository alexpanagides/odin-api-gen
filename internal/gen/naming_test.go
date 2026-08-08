package gen

import "testing"

func TestSnakeCase(t *testing.T) {
	cases := map[string]string{
		"start_date":    "start_date",
		"updatedSince":  "updated_since",
		"TimeZone":      "time_zone",
		"map":           "map_", // Odin keyword escaped
		"needs review?": "needs_review",
	}
	for in, want := range cases {
		if got := SnakeCase(in); got != want {
			t.Errorf("SnakeCase(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAdaCase(t *testing.T) {
	cases := map[string]string{
		"TransactionAccount":    "Transaction_Account",
		"BudgetAnalysisPackage": "Budget_Analysis_Package",
		"TimeZone":              "Time_Zone",
		"no-interest":           "No_Interest",
		"each weekday":          "Each_Weekday",
	}
	for in, want := range cases {
		if got := AdaCase(in); got != want {
			t.Errorf("AdaCase(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestProcName(t *testing.T) {
	cases := map[string]string{
		"Get a transaction":                 "get_transaction",
		"Get the authorised user":           "get_authorised_user",
		"Lists attachments in user":         "list_attachments_in_user",
		"List events in user.":              "list_events_in_user",
		"Assigns attachment to transaction": "assign_attachment_to_transaction",
	}
	for in, want := range cases {
		if got := ProcName(in); got != want {
			t.Errorf("ProcName(%q) = %q, want %q", in, got, want)
		}
	}
}
