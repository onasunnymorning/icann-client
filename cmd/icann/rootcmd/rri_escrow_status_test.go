package rootcmd

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/onasunnymorning/icann-client/rri"
)

const escrowReportsBody = `<rdeReports:reports xmlns:rdeReports="urn:ietf:params:xml:ns:rdeReports-1.0"
 xmlns:rdeReport="urn:ietf:params:xml:ns:rdeReport-1.0" xmlns:rdeHeader="urn:ietf:params:xml:ns:rdeHeader-1.0">
<rdeReports:receivedReport><rdeReports:received>2026-10-01T01:34:13.741Z</rdeReports:received>
<rdeReport:report><rdeReport:id>r1</rdeReport:id><rdeReport:version>1</rdeReport:version>
<rdeReport:crDate>2026-10-01T01:20:00Z</rdeReport:crDate><rdeReport:kind>FULL</rdeReport:kind>
<rdeReport:watermark>2026-10-01T00:00:00Z</rdeReport:watermark>
<rdeHeader:header><rdeHeader:tld>example</rdeHeader:tld></rdeHeader:header></rdeReport:report>
</rdeReports:receivedReport></rdeReports:reports>`

func TestEscrowStatusDefaultsToHead(t *testing.T) {
	t.Cleanup(func() { flagDetails, flagDate = false, "" })
	_, paths, err := runGetCmd(t, rriEscrowStatusCmd, []string{"--date", "2026-10-01"}, func(w http.ResponseWriter, r *http.Request) {})
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 || paths[0] != "HEAD /info/report/registry-escrow-report/example/2026-10-01" {
		t.Errorf("paths = %v", paths)
	}
}

func TestEscrowStatusDetails(t *testing.T) {
	t.Cleanup(func() { flagDetails, flagDate = false, "" })
	out, paths, err := runGetCmd(t, rriEscrowStatusCmd, []string{"--date", "2026-10-01", "--details"}, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(escrowReportsBody))
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 || paths[0] != "GET /info/report/registry-escrow-report/example/2026-10-01" {
		t.Errorf("paths = %v", paths)
	}
	var got struct {
		Reports []struct {
			Received string `json:"received"`
			Report   struct{ ID string }
		}
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if len(got.Reports) != 1 || got.Reports[0].Received != "2026-10-01T01:34:13.741Z" || got.Reports[0].Report.ID != "r1" {
		t.Errorf("output = %s", out)
	}
}

func TestEscrowStatusDetailsBeforeDeployment(t *testing.T) {
	t.Cleanup(func() { flagDetails, flagDate = false, "" })
	_, _, err := runGetCmd(t, rriEscrowStatusCmd, []string{"--date", "2020-01-01", "--details"}, func(w http.ResponseWriter, r *http.Request) {
		respondCode(w, 200, rri.ResultDateBeforeGET, "earlier than the production deployment")
	})
	if code, ok := rri.ResultCodeOf(err); !ok || code != rri.ResultDateBeforeGET {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(err.Error(), "hint: ICANN serves report and notification details") {
		t.Errorf("err lacks the hint: %v", err)
	}
}

func TestEscrowNotificationsCmd(t *testing.T) {
	t.Cleanup(func() { flagDate = "" })
	out, paths, err := runGetCmd(t, rriEscrowNotificationsCmd, []string{"--date", "2026-10-01"}, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<n:notifications xmlns:n="urn:ietf:params:xml:ns:rdeNotifications-1.0" xmlns:d="urn:ietf:params:xml:ns:rdeNotification-1.0">
<n:receivedNotification><n:received>2026-10-01T01:00:31.0Z</n:received><d:notification>
<d:deaName>Agent</d:deaName><d:version>1</d:version><d:repDate>2026-10-01</d:repDate><d:status>DRFN</d:status><d:lastFullDate>2026-09-28</d:lastFullDate>
</d:notification></n:receivedNotification></n:notifications>`))
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 || paths[0] != "GET /info/report/escrow-agent-notification/example/2026-10-01" {
		t.Errorf("paths = %v", paths)
	}
	if !strings.Contains(string(out), `"status": "DRFN"`) || !strings.Contains(string(out), `"received": "2026-10-01T01:00:31Z"`) {
		t.Errorf("output = %s", out)
	}
}

func TestEscrowNotificationsBadDate(t *testing.T) {
	t.Cleanup(func() { flagDate = "" })
	_, _, err := runGetCmd(t, rriEscrowNotificationsCmd, []string{"--date", "10/01/2026"}, func(w http.ResponseWriter, r *http.Request) {})
	if err == nil || !strings.Contains(err.Error(), "YYYY-MM-DD") {
		t.Fatalf("err = %v", err)
	}
}

func TestEscrowNotificationStatusCmd(t *testing.T) {
	out, paths, err := runGetCmd(t, rriEscrowNotificationStatusCmd, nil, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<s:summary xmlns:s="urn:ietf:params:xml:ns:rriReporting-1.0" xmlns:h="urn:ietf:params:xml:ns:rdeHeader-1.0">
<h:tld>example</h:tld><s:creationDate>2026-01-02T12:00:30Z</s:creationDate><s:statusReports><s:statusReport>
<s:type>DEA_Notification</s:type><s:enabled>true</s:enabled><s:status>unsatisfactory</s:status>
<s:issues><s:issue date="2026-01-01" description="No_Report_Received"/></s:issues></s:statusReport></s:statusReports></s:summary>`))
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 || paths[0] != "GET /info/status/escrow-agent-notification/example" {
		t.Errorf("paths = %v", paths)
	}
	if !strings.Contains(string(out), `"status": "unsatisfactory"`) || !strings.Contains(string(out), `"description": "No_Report_Received"`) {
		t.Errorf("output = %s", out)
	}
}

func TestEscrowNotificationStatusAlias(t *testing.T) {
	found, _, err := rriEscrowCmd.Find([]string{"dea-status"})
	if err != nil || found != rriEscrowNotificationStatusCmd {
		t.Fatalf("dea-status resolves to %v, %v", found, err)
	}
}
