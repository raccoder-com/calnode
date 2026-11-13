package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListBookings_attendeeContext(t *testing.T) {
	for _, admin := range []bool{true, false} {
		name := "host"
		if admin {
			name = "admin"
		}
		t.Run(name, func(t *testing.T) {
			h, database, key, ownerID := setupWorkspaceWithDB(t)
			_, etID := seedEventTypeHTTP(t, h, key)
			if _, err := database.Exec(`UPDATE users SET is_admin = ?, is_owner = 0 WHERE id = ?`, admin, ownerID); err != nil {
				t.Fatal(err)
			}
			if _, err := database.Exec(`INSERT INTO users (id,email,name) VALUES ('other-host','other@example.com','Other Host')`); err != nil {
				t.Fatal(err)
			}
			seedBooking(t, database, "local-booking", etID, ownerID, "2027-07-01T10:00:00Z", "2027-07-01T10:30:00Z", "confirmed")
			seedBooking(t, database, "default-booking", etID, ownerID, "2027-07-02T10:00:00Z", "2027-07-02T10:30:00Z", "confirmed")
			seedBooking(t, database, "other-booking", etID, "other-host", "2027-07-03T10:00:00Z", "2027-07-03T10:30:00Z", "confirmed")
			if _, err := database.Exec(`INSERT INTO booking_attendees (id,booking_id,name,email,is_organizer,locale,iana_timezone) VALUES
				('local-attendee','local-booking','Test Guest','guest@example.com',1,'de','Europe/Berlin'),
				('extra-attendee','local-booking','Extra Guest','extra@example.com',0,'fr','Europe/Paris'),
				('other-attendee','other-booking','Other Guest','other-guest@example.com',1,'es','Europe/Madrid')`); err != nil {
				t.Fatal(err)
			}
			if _, err := database.Exec(`INSERT INTO booking_attendees (id,booking_id,name,email,is_organizer) VALUES ('default-attendee','default-booking','Default Guest','default@example.com',1)`); err != nil {
				t.Fatal(err)
			}

			for _, query := range []string{"", "?scope=all", "?when=past"} {
				req := authReq(http.MethodGet, "/v1/bookings"+query, "", key)
				rec := httptest.NewRecorder()
				h.RequireAuth(h.ListBookings)(rec, req)
				if rec.Code != http.StatusOK {
					t.Fatalf("GET bookings%s: %d - %s", query, rec.Code, rec.Body.String())
				}
				var out struct {
					Items []struct {
						ID        string `json:"id"`
						Attendees []struct {
							Name         string `json:"name"`
							Email        string `json:"email"`
							Locale       string `json:"locale"`
							IANATimezone string `json:"iana_timezone"`
						} `json:"attendees"`
					} `json:"items"`
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
					t.Fatal(err)
				}
				wantCount := 2
				if admin && query == "?scope=all" {
					wantCount = 3
				} else if query == "?when=past" {
					wantCount = 0
					if out.Items == nil {
						t.Error("empty items must remain a JSON array")
					}
				}
				if len(out.Items) != wantCount {
					t.Fatalf("GET bookings%s: got %d items, want %d", query, len(out.Items), wantCount)
				}
				for _, item := range out.Items {
					if len(item.Attendees) != 1 {
						t.Fatalf("booking %s: got %d attendees, want only organizer", item.ID, len(item.Attendees))
					}
					att := item.Attendees[0]
					wantName, wantEmail, wantLocale, wantTZ := "Test Guest", "guest@example.com", "de", "Europe/Berlin"
					if item.ID == "default-booking" {
						wantName, wantEmail, wantLocale, wantTZ = "Default Guest", "default@example.com", "en", "UTC"
					} else if item.ID == "other-booking" {
						if !admin || query != "?scope=all" {
							t.Fatal("another host's booking escaped visibility filtering")
						}
						wantName, wantEmail, wantLocale, wantTZ = "Other Guest", "other-guest@example.com", "es", "Europe/Madrid"
					}
					if att.Name != wantName || att.Email != wantEmail || att.Locale != wantLocale || att.IANATimezone != wantTZ {
						t.Errorf("booking %s: got attendee %+v, want %q/%q/%q/%q", item.ID, att, wantName, wantEmail, wantLocale, wantTZ)
					}
				}
			}
			rec := httptest.NewRecorder()
			h.RequireAuth(h.ListBookings)(rec, httptest.NewRequest(http.MethodGet, "/v1/bookings", nil))
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("unauthenticated list: got %d, want 401", rec.Code)
			}
		})
	}
}
