package lti

import (
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

func TestLaunchClaimsValidateResourceLink(t *testing.T) {
	t.Parallel()

	claims := LaunchClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:   "https://moodle-emulator.local",
			Subject:  "user-student-001",
			Audience: jwt.ClaimStrings{"cloud-lab-gateway"},
		},
		AuthorizedParty: "cloud-lab-gateway",
		Nonce:           "nonce",
		DeploymentID:    "emu-deployment-1",
		MessageType:     MessageTypeResourceLink,
		Version:         Version13,
		Roles:           []string{RoleLearner},
		Context:         Context{ID: "linux-101"},
		ResourceLink:    Resource{ID: "linux-basics-1"},
	}
	if err := claims.ValidateResourceLink(); err != nil {
		t.Fatalf("validate: %v", err)
	}
}
