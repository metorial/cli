package onboarding

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/metorial/cli/internal/auth"
)

func presentOnboardingError(err error, rootName string) error {
	apiError := &auth.Error{}
	if !errors.As(err, &apiError) {
		return err
	}

	switch apiError.ErrorCode {
	case "rate_limited":
		if apiError.RetryAfter > 0 {
			return fmt.Errorf(
				"metorial: too many organizations were created from this network recently. Try again in %s",
				(time.Duration(apiError.RetryAfter) * time.Second).String(),
			)
		}
		return fmt.Errorf("metorial: too many organizations were created from this network recently. Try again later")
	case "signup_blocked":
		return fmt.Errorf("metorial: signup is not available from this network.\nRun \"%s login\" to sign in with an existing Metorial account", rootName)
	case "signup_disabled":
		return fmt.Errorf("metorial: signup without an account is not enabled on this host.\nRun \"%s login\" to sign in with an existing Metorial account", rootName)
	case "invalid_claim_email", "invalid_email":
		if message := strings.TrimSpace(apiError.ErrorMessage); message != "" {
			return fmt.Errorf("metorial: %s", message)
		}
		if reason := strings.TrimSpace(apiError.Reason); reason != "" {
			return fmt.Errorf("metorial: this email address cannot be used to claim the organization (%s)", reason)
		}
		return fmt.Errorf("metorial: this email address cannot be used to claim the organization")
	case "claim_email_change_limit":
		return fmt.Errorf("metorial: the claim email for this organization was changed too many times. Open the claim link instead")
	case "claim_email_org_limit":
		return fmt.Errorf("metorial: this email address already has too many unclaimed organizations. Claim one of them first")
	case "already_claimed", "conflict":
		return fmt.Errorf("metorial: this organization was already claimed")
	case "unauthorized", "invalid_token", "invalid_grant":
		return fmt.Errorf("metorial: the saved profile for this organization is no longer valid")
	}

	if message := strings.TrimSpace(apiError.ErrorMessage); message != "" {
		return fmt.Errorf("metorial: %s", message)
	}

	return err
}
