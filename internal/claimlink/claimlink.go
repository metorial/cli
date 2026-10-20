package claimlink

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"time"
)

const (
	PurposeClaim = "claim"

	version          = "v1"
	identifierLength = 14
	signatureLength  = 32
)

var (
	regionPattern = regexp.MustCompile(`^[a-z0-9-]{1,32}$`)
	tokenPattern  = regexp.MustCompile(`^(?:([a-z0-9-]{1,32})_)?[0-9a-f]{14}_\d{1,12}_[0-9a-f]{32}$`)
)

func Identifier(claimToken string) string {
	sum := sha256.Sum256([]byte("metorial-claim-identifier|" + claimToken))
	return hex.EncodeToString(sum[:])[:identifierLength]
}

// Region returns the region a signed link was minted in, or "" when it has none.
func Region(linkToken string) string {
	match := tokenPattern.FindStringSubmatch(linkToken)
	if match == nil {
		return ""
	}
	return match[1]
}

func Sign(claimToken, purpose, region string, expiresAt time.Time) string {
	if !regionPattern.MatchString(region) {
		region = ""
	}

	identifier := Identifier(claimToken)
	expiresAtSeconds := expiresAt.Unix()

	mac := hmac.New(sha256.New, []byte(claimToken))
	_, _ = fmt.Fprintf(mac, "%s|%s|%s|%s|%d", version, purpose, region, identifier, expiresAtSeconds)
	signature := hex.EncodeToString(mac.Sum(nil))[:signatureLength]

	token := identifier + "_" + strconv.FormatInt(expiresAtSeconds, 10) + "_" + signature
	if region != "" {
		token = region + "_" + token
	}
	return token
}
