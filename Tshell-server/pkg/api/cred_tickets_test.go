package api

import (
	"strings"
	"testing"

	credpkg "BackendTemplate/pkg/cred"
	"BackendTemplate/pkg/database"
)

// TestTicketArtifactIngest 票据入库：ccache 与 LSA 两种来源要区分开
// （`kerberos:ccache` / `kerberos:lsa`），LSA 条目 note 里带上服务名与 ptt 提示。
func TestTicketArtifactIngest(t *testing.T) {
	setupCredTest(t)

	art := credpkg.TicketArtifact{
		Host: "WS01",
		Caches: []credpkg.TicketCache{
			{
				File: "kirbi/01-krbtgt_CORP_LOCAL.kirbi", Source: "lsa",
				Server:    "krbtgt/CORP.LOCAL",
				Principal: "alice@CORP.LOCAL", Realm: "CORP.LOCAL",
				Tickets: 1, Expires: "2026-10-20T08:00:00Z",
			},
			{
				File: "tickets/krb5cc_1000", Version: 4,
				Principal: "bob@CORP.LOCAL", Realm: "CORP.LOCAL",
				Tickets: 3, Expires: "2026-10-20T09:00:00Z",
			},
		},
	}
	if n := ingestTicketArtifact("uid-win", art); n != 2 {
		t.Fatalf("应入库 2 条票据，实际 %d", n)
	}

	var loot []database.Loot
	database.Engine.Where("uid = ?", "uid-win").Asc("id").Find(&loot)
	if len(loot) != 2 {
		t.Fatalf("应登记 2 条 loot，实际 %d: %+v", len(loot), loot)
	}

	lsa := loot[0]
	if lsa.Type != "ticket" || lsa.Source != "kerberos:lsa" ||
		lsa.User != "alice@CORP.LOCAL" || lsa.Realm != "CORP.LOCAL" ||
		lsa.Secret != "kirbi/01-krbtgt_CORP_LOCAL.kirbi" {
		t.Fatalf("LSA 票据条目不符: %+v", lsa)
	}
	if !strings.Contains(lsa.Note, "krbtgt/CORP.LOCAL") || !strings.Contains(lsa.Note, "ptt") {
		t.Fatalf("LSA 条目 note 应含服务名与 ptt 提示: %q", lsa.Note)
	}

	ccache := loot[1]
	if ccache.Source != "kerberos:ccache" || ccache.User != "bob@CORP.LOCAL" || ccache.Secret != "tickets/krb5cc_1000" {
		t.Fatalf("ccache 条目不符: %+v", ccache)
	}
	if !strings.Contains(ccache.Note, "3 张票据") || !strings.Contains(ccache.Note, "kinit") {
		t.Fatalf("ccache 条目 note 不符: %q", ccache.Note)
	}
}
