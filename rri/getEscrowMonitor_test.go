package rri

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	base "github.com/onasunnymorning/icann-client/client"
)

// monitorClient points a client for TLD "test" at a stub server and records the
// request the server saw.
func monitorClient(t *testing.T, h http.HandlerFunc) (*Client, *http.Request) {
	t.Helper()
	var seen http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = *r.Clone(r.Context())
		h(w, r)
	}))
	t.Cleanup(srv.Close)
	c, err := New(base.Config{TLD: "test", AuthType: base.AUTH_TYPE_BASIC, Username: "u", Password: "p"})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.WithBaseURL(srv.URL); err != nil {
		t.Fatal(err)
	}
	return c, &seen
}

func serveFile(t *testing.T, name string) http.HandlerFunc {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		w.Write(b)
	}
}

var monitorDate = time.Date(2025, 10, 17, 0, 0, 0, 0, time.UTC)

func TestGetRyEscrowReports(t *testing.T) {
	c, seen := monitorClient(t, serveFile(t, "escrow-reports.xml"))
	got, err := c.GetRyEscrowReports(context.Background(), monitorDate)
	if err != nil {
		t.Fatal(err)
	}
	if seen.Method != http.MethodGet || seen.URL.Path != "/info/report/registry-escrow-report/test/2025-10-17" {
		t.Errorf("request = %s %s", seen.Method, seen.URL.Path)
	}
	if seen.Header.Get("Accept") != "" {
		t.Errorf("Accept = %q, want none", seen.Header.Get("Accept"))
	}
	if len(got.Reports) != 2 {
		t.Fatalf("reports = %d, want 2", len(got.Reports))
	}
	r := got.Reports[0]
	if want := time.Date(2025, 10, 17, 1, 34, 13, 741e6, time.UTC); !r.Received.Equal(want) {
		t.Errorf("received = %v, want %v", r.Received, want)
	}
	if r.Report.ID != "20251017002" || r.Report.Kind != KindFull || r.Report.TLD != "test" || len(r.Report.Counts) != 7 {
		t.Errorf("report = %+v", r.Report)
	}
	if want := time.Date(2025, 10, 17, 1, 20, 0, 0, time.UTC); !r.Report.CrDate.Equal(want) {
		t.Errorf("crDate = %v, want %v", r.Report.CrDate, want)
	}
	if got.Reports[1].Report.ID != "20251017001" {
		t.Errorf("second id = %q", got.Reports[1].Report.ID)
	}
}

func TestGetRyEscrowReportsDefaultNamespace(t *testing.T) {
	doc := `<reports xmlns="urn:ietf:params:xml:ns:rdeReports-1.0"><receivedReport>
<received>2025-10-17T01:34:13Z</received>
<report xmlns="urn:ietf:params:xml:ns:rdeReport-1.0" xmlns:h="urn:ietf:params:xml:ns:rdeHeader-1.0">
<id>x1</id><version>1</version><crDate>2025-10-17T01:20:00Z</crDate><kind>DIFF</kind>
<watermark>2025-10-17T00:00:00Z</watermark><h:header><h:tld>test</h:tld></h:header></report>
</receivedReport></reports>`
	c, _ := monitorClient(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(doc)) })
	got, err := c.GetRyEscrowReports(context.Background(), monitorDate)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Reports) != 1 || got.Reports[0].Report.ID != "x1" || got.Reports[0].Report.Kind != KindDiff {
		t.Errorf("got %+v", got)
	}
}

func TestGetRyEscrowReportsNotFoundIsEmpty(t *testing.T) {
	c, _ := monitorClient(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotFound) })
	got, err := c.GetRyEscrowReports(context.Background(), monitorDate)
	if err != nil {
		t.Fatal(err)
	}
	if got.Reports == nil || len(got.Reports) != 0 {
		t.Errorf("reports = %#v, want empty non-nil", got.Reports)
	}
}

func TestGetRyEscrowReportsErrorPageIsError(t *testing.T) {
	for name, body := range map[string]string{
		"html":         "<html><body>Not Found</body></html>",
		"unauthorized": `{"message":"Unauthorized"}`,
	} {
		t.Run(name, func(t *testing.T) {
			c, _ := monitorClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				w.Write([]byte(body))
			})
			if _, err := c.GetRyEscrowReports(context.Background(), monitorDate); err == nil {
				t.Fatal("want an error, got an empty answer")
			}
		})
	}
}

func TestGetRyEscrowReportsDateBeforeDeployment(t *testing.T) {
	c, _ := monitorClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		w.Write([]byte(`<response xmlns="urn:ietf:params:xml:ns:iirdea-1.0"><result code="2214"><msg>The queried date is earlier than the production deployment of this feature.</msg></result></response>`))
	})
	_, err := c.GetRyEscrowReports(context.Background(), monitorDate)
	if code, ok := ResultCodeOf(err); !ok || code != ResultDateBeforeGET {
		t.Fatalf("err = %v, want result code 2214", err)
	}
	if ResultHint(ResultDateBeforeGET) == "" {
		t.Error("2214 has no hint")
	}
}

func TestGetRyEscrowReportsMalformed(t *testing.T) {
	c, _ := monitorClient(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("<oops")) })
	_, err := c.GetRyEscrowReports(context.Background(), monitorDate)
	if err == nil || !strings.Contains(err.Error(), "HTTP 200") {
		t.Fatalf("err = %v, want a decode error naming HTTP 200", err)
	}
}

func TestGetRyEscrowReportsServerError(t *testing.T) {
	c, _ := monitorClient(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusInternalServerError) })
	if _, err := c.GetRyEscrowReports(context.Background(), monitorDate); err == nil {
		t.Fatal("want an error")
	}
}

func TestGetEscrowNotifications(t *testing.T) {
	c, seen := monitorClient(t, serveFile(t, "escrow-notifications.xml"))
	got, err := c.GetEscrowNotifications(context.Background(), monitorDate)
	if err != nil {
		t.Fatal(err)
	}
	if seen.Method != http.MethodGet || seen.URL.Path != "/info/report/escrow-agent-notification/test/2025-10-17" {
		t.Errorf("request = %s %s", seen.Method, seen.URL.Path)
	}
	if len(got.Notifications) != 2 {
		t.Fatalf("notifications = %d, want 2", len(got.Notifications))
	}
	v := got.Notifications[0]
	if v.Status != "DVPN" || v.DeaName != "Escrow Agent Inc." || v.LastFullDate != "2025-10-17" || v.RepDate != "2025-10-17" {
		t.Errorf("DVPN = %+v", v)
	}
	if want := time.Date(2025, 10, 17, 6, 30, 31, 0, time.UTC); !v.Received.Equal(want) {
		t.Errorf("received = %v, want %v", v.Received, want)
	}
	if v.REDate == nil || v.VADate == nil || v.Report == nil || v.Report.ID != "20251017001" {
		t.Errorf("DVPN optional fields = %+v", v)
	}
	d := got.Notifications[1]
	if d.Status != "DRFN" || d.LastFullDate != "2025-10-14" || d.REDate != nil || d.VADate != nil || d.Report != nil {
		t.Errorf("DRFN = %+v", d)
	}
}

func TestGetEscrowNotificationsEmptyAndRejected(t *testing.T) {
	c, _ := monitorClient(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotFound) })
	got, err := c.GetEscrowNotifications(context.Background(), monitorDate)
	if err != nil || got.Notifications == nil || len(got.Notifications) != 0 {
		t.Fatalf("got %+v, %v; want empty list", got, err)
	}

	c, _ = monitorClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<response xmlns="urn:ietf:params:xml:ns:iirdea-1.0"><result code="2214"><msg>m</msg></result></response>`))
	})
	if _, err := c.GetEscrowNotifications(context.Background(), monitorDate); !IsRejected(err) {
		t.Fatalf("err = %v, want a rejection", err)
	}
}
