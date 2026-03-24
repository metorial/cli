package fetch

import (
	"net/url"
	"testing"
)

func TestResolveURL(t *testing.T) {
	t.Parallel()

	apiHost, err := url.Parse("https://api.metorial.com")
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}

	tests := []struct {
		name    string
		target  string
		want    string
		wantErr bool
	}{
		{name: "path", target: "/provider-listings", want: "https://api.metorial.com/provider-listings"},
		{name: "path without slash", target: "provider-listings", want: "https://api.metorial.com/provider-listings"},
		{name: "matching full url", target: "https://api.metorial.com/provider-listings", want: "https://api.metorial.com/provider-listings"},
		{name: "mismatched host", target: "https://example.com/provider-listings", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ResolveURL(apiHost, tt.target)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ResolveURL() error = nil, want error")
				}
				return
			}

			if err != nil {
				t.Fatalf("ResolveURL() error = %v", err)
			}

			if got.String() != tt.want {
				t.Fatalf("ResolveURL() = %q, want %q", got.String(), tt.want)
			}
		})
	}
}
