package rri

import (
	"net/http"
	"strings"
	"testing"
)

const notificationStatusOK = `<?xml version="1.0" encoding="UTF-8"?>
<rriReporting:summary xmlns:rriReporting="urn:ietf:params:xml:ns:rriReporting-1.0" xmlns:rdeHeader="urn:ietf:params:xml:ns:rdeHeader-1.0">
  <rdeHeader:tld>example</rdeHeader:tld>
  <rriReporting:creationDate>2026-01-02T12:00:30.101Z</rriReporting:creationDate>
  <rriReporting:depositSchedule>Daily</rriReporting:depositSchedule>
  <rriReporting:lastFullDate>2026-01-01</rriReporting:lastFullDate>
  <rriReporting:statusReports>
    <rriReporting:statusReport>
      <rriReporting:type>DEA_Notification</rriReporting:type>
      <rriReporting:enabled>true</rriReporting:enabled>
      <rriReporting:status>ok</rriReporting:status>
    </rriReporting:statusReport>
  </rriReporting:statusReports>
  <rriReporting:timestamp>2026-01-02T12:00:00.000Z</rriReporting:timestamp>
</rriReporting:summary>`

const notificationStatusIssues = `<?xml version="1.0" encoding="UTF-8"?>
<rriReporting:summary xmlns:rriReporting="urn:ietf:params:xml:ns:rriReporting-1.0" xmlns:rdeHeader="urn:ietf:params:xml:ns:rdeHeader-1.0">
  <rdeHeader:tld>example</rdeHeader:tld>
  <rriReporting:creationDate>2026-01-02T12:00:30.101Z</rriReporting:creationDate>
  <rriReporting:depositSchedule>Daily</rriReporting:depositSchedule>
  <rriReporting:lastFullDate>2026-01-01</rriReporting:lastFullDate>
  <rriReporting:statusReports>
    <rriReporting:statusReport>
      <rriReporting:type>DEA_Notification</rriReporting:type>
      <rriReporting:enabled>true</rriReporting:enabled>
      <rriReporting:status>unsatisfactory</rriReporting:status>
      <rriReporting:issues>
        <rriReporting:issue date="2026-01-01" description="No_Report_Received" />
        <rriReporting:issue date="2025-12-30" description="Invalid_Deposit_Full" />
      </rriReporting:issues>
    </rriReporting:statusReport>
  </rriReporting:statusReports>
  <rriReporting:timestamp>2026-01-02T12:00:00.000Z</rriReporting:timestamp>
</rriReporting:summary>`

func serveBody(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) }
}

func TestGetEscrowNotificationStatusRequestShape(t *testing.T) {
	var method, path, accept string
	c := newTestRRI(t, func(w http.ResponseWriter, r *http.Request) {
		method, path, accept = r.Method, r.URL.Path, r.Header.Get("Accept")
		w.Write([]byte(notificationStatusOK))
	})
	if _, err := c.GetEscrowNotificationStatus(t.Context()); err != nil {
		t.Fatal(err)
	}
	if method != http.MethodGet || path != "/info/status/escrow-agent-notification/"+c.Config().TLD || accept != "" {
		t.Errorf("request = %s %s Accept=%q", method, path, accept)
	}
}

func TestGetEscrowNotificationStatusXML(t *testing.T) {
	c := newTestRRI(t, serveBody(notificationStatusOK))
	got, err := c.GetEscrowNotificationStatus(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if got.TLD != "example" || got.Status != "ok" || got.DepositSchedule != "Daily" || got.LastFullDate != "2026-01-01" ||
		got.Created != "2026-01-02T12:00:30.101Z" || got.Enabled == nil || !*got.Enabled {
		t.Errorf("got %+v", got)
	}
	if got.Issues == nil || len(got.Issues) != 0 {
		t.Errorf("issues = %#v, want empty non-nil", got.Issues)
	}
}

func TestGetEscrowNotificationStatusXMLIssues(t *testing.T) {
	c := newTestRRI(t, serveBody(notificationStatusIssues))
	got, err := c.GetEscrowNotificationStatus(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	want := []NotificationIssue{
		{"2026-01-01", IssueNoReportReceived},
		{"2025-12-30", IssueInvalidDepositFull},
	}
	if got.Status != "unsatisfactory" || len(got.Issues) != 2 || got.Issues[0] != want[0] || got.Issues[1] != want[1] {
		t.Errorf("got %+v", got)
	}
}

// ICANN serves JSON on the sibling registry status interface, so the same
// shape is accepted here rather than assuming the draft's XML.
func TestGetEscrowNotificationStatusJSON(t *testing.T) {
	c := newTestRRI(t, serveBody(`{"tld":{"name":"example"},"paths":[{"path":"Full","status":"ok"},{"path":"Dea","status":"unsatisfactory"}],"created":"2026-09-15T00:44:03.230Z"}`))
	got, err := c.GetEscrowNotificationStatus(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if got.TLD != "example" || got.Status != "unsatisfactory" || got.Created != "2026-09-15T00:44:03.230Z" {
		t.Errorf("got %+v", got)
	}
}

func TestGetEscrowNotificationStatusUnrecognised(t *testing.T) {
	for name, body := range map[string]string{
		"json without Dea":   `{"tld":{"name":"example"},"paths":[{"path":"Full","status":"ok"}]}`,
		"xml without report": strings.Replace(notificationStatusOK, "DEA_Notification", "Other", 1),
		"junk":               "<oops",
	} {
		t.Run(name, func(t *testing.T) {
			c := newTestRRI(t, serveBody(body))
			_, err := c.GetEscrowNotificationStatus(t.Context())
			if err == nil || !strings.Contains(err.Error(), "HTTP 200") {
				t.Fatalf("err = %v, want a decode error naming HTTP 200", err)
			}
		})
	}
}

func TestGetEscrowNotificationStatusRejected(t *testing.T) {
	c := newTestRRI(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	if _, err := c.GetEscrowNotificationStatus(t.Context()); err == nil {
		t.Fatal("want an error for HTTP 403")
	}
}
