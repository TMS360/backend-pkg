package tests

import (
	"testing"
	"time"

	"github.com/TMS360/backend-pkg/auth"
)

// IssuedBeforeCutoff is the pure core of the revocation decision. These cases
// pin the two things that matter: an old token IS rejected, and clock skew never
// rejects a token that was actually issued after the cutoff.
func TestIssuedBeforeCutoff(t *testing.T) {
	cutoff := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name     string
		issuedAt time.Time
		want     bool
	}{
		{
			name:     "issued days before cutoff is revoked",
			issuedAt: cutoff.Add(-7 * 24 * time.Hour),
			want:     true,
		},
		{
			name:     "issued just before cutoff, beyond skew, is revoked",
			issuedAt: cutoff.Add(-auth.RevocationClockSkew - time.Second),
			want:     true,
		},
		{
			name:     "issued within skew before cutoff is NOT revoked (clock skew guard)",
			issuedAt: cutoff.Add(-auth.RevocationClockSkew + time.Second),
			want:     false,
		},
		{
			name:     "issued exactly at cutoff is NOT revoked",
			issuedAt: cutoff,
			want:     false,
		},
		{
			name:     "issued after cutoff (fresh refresh) is NOT revoked",
			issuedAt: cutoff.Add(time.Minute),
			want:     false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := auth.IssuedBeforeCutoff(tc.issuedAt, cutoff); got != tc.want {
				t.Fatalf("IssuedBeforeCutoff(%s, %s) = %v, want %v", tc.issuedAt, cutoff, got, tc.want)
			}
		})
	}
}

// The marker must outlive the longest access token that could still be
// presented, otherwise a revoked token could survive its own cutoff. Guards
// against someone shortening the TTL below the (historical) 7-day access token.
func TestTokensValidAfterTTLOutlivesLongestToken(t *testing.T) {
	const longestObservedAccessToken = 7 * 24 * time.Hour
	if auth.TokensValidAfterTTL <= longestObservedAccessToken {
		t.Fatalf("TokensValidAfterTTL %s must exceed the longest access-token life %s",
			auth.TokensValidAfterTTL, longestObservedAccessToken)
	}
}
