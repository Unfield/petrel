package mail

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"
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

// checkFolders makes, renames and deletes a folder, and reads a message's
// original, on a real server.
func checkFolders(t *testing.T, p Provider, anyMessage string) {
	t.Helper()
	ctx := context.Background()
	fp := p.(FolderProvider)
	name := "Petrel folder " + time.Now().Format("150405.000")
	id, err := fp.CreateMailbox(ctx, name, "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	child, err := fp.CreateMailbox(ctx, "Inside", id)
	if err != nil {
		t.Fatalf("create inside: %v", err)
	}
	if id, err = fp.RenameMailbox(ctx, id, name+" renamed"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	boxes, _ := p.Mailboxes(ctx)
	found := false
	for _, b := range boxes {
		found = found || b.Path == name+" renamed"
	}
	if !found {
		t.Errorf("renamed folder not listed: %+v", boxes)
	}
	_ = child
	for _, b := range boxes {
		if strings.HasPrefix(b.Path, name+" renamed") && b.ID != id {
			if err := fp.DeleteMailbox(ctx, b.ID); err != nil {
				t.Errorf("delete inside: %v", err)
			}
		}
	}
	if err := fp.DeleteMailbox(ctx, id); err != nil {
		t.Errorf("delete: %v", err)
	}
	if anyMessage != "" {
		r, err := p.(RawProvider).Raw(ctx, anyMessage)
		if err != nil {
			t.Fatalf("raw: %v", err)
		}
		b, _ := io.ReadAll(r)
		r.Close()
		if !bytes.Contains(b, []byte("Subject:")) {
			t.Errorf("raw: %.200s", b)
		}
	}
}
