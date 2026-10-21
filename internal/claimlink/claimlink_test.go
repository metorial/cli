package claimlink

import (
	"testing"
	"time"
)

// These vectors are shared with the server tests (cliClaimLink.test.ts).
const (
	testToken      = "mt_claim_testtoken"
	testIdentifier = "c94352902e507e"
	testClaimLink  = "eu1_c94352902e507e_1800000000_314636815b9894fe7fc230c4cd7d58ca"
	testNoRegion   = "c94352902e507e_1800000000_30928112e94cbf26fc06c938a5b2b64c"
)

func TestIdentifierMatchesServer(t *testing.T) {
	if got := Identifier(testToken); got != testIdentifier {
		t.Fatalf("Identifier() = %q, want %q", got, testIdentifier)
	}
}

func TestSignMatchesServer(t *testing.T) {
	expiresAt := time.Unix(1800000000, 0)

	if got := Sign(testToken, PurposeClaim, "eu1", expiresAt); got != testClaimLink {
		t.Fatalf("Sign() = %q, want %q", got, testClaimLink)
	}
	if got := Sign(testToken, PurposeClaim, "", expiresAt); got != testNoRegion {
		t.Fatalf("Sign() without region = %q, want %q", got, testNoRegion)
	}
}

func TestSignBindsPurposeRegionAndToken(t *testing.T) {
	expiresAt := time.Unix(1800000000, 0)

	if Sign(testToken, "unsubscribe", "eu1", expiresAt) == testClaimLink {
		t.Fatal("expected the purpose to change the signature")
	}
	if Sign(testToken, PurposeClaim, "us1", expiresAt) == testClaimLink {
		t.Fatal("expected the region to change the link")
	}
	if Sign("mt_claim_other", PurposeClaim, "eu1", expiresAt) == testClaimLink {
		t.Fatal("expected the claim token to change the signature")
	}
}

func TestRegion(t *testing.T) {
	if got := Region(testClaimLink); got != "eu1" {
		t.Fatalf("Region() = %q, want eu1", got)
	}
	for _, token := range []string{testNoRegion, "expired", ""} {
		if got := Region(token); got != "" {
			t.Fatalf("Region(%q) = %q, want empty", token, got)
		}
	}
}
