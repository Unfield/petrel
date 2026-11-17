package mail

import (
	"context"
	"testing"
)

// checkQuota expects sam's 100 MB quota from scripts/stalwart-test.sh.
func checkQuota(t *testing.T, p Provider) {
	t.Helper()
	qp, ok := p.(QuotaProvider)
	if !ok {
		t.Fatal("no Quota method")
	}
	q, err := qp.Quota(context.Background())
	if err != nil {
		t.Fatalf("quota: %v", err)
	}
	if q.Limit != 100<<20 || q.Used < 0 {
		t.Errorf("quota: %+v, want a 100 MB limit", q)
	}
}
