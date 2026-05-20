package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/cloud-lab-gateway/gateway/cmd/moodle-emulator/templates"
	"github.com/cloud-lab-gateway/gateway/internal/lti"
)

const clientID = "cloud-lab-gateway"

type launchPayload struct {
	IDToken string
	State   string
	Nonce   string
}

func (s *signer) issueLaunch(now time.Time, issuer string, user templates.User, course templates.Course, lab templates.Lab) (launchPayload, error) {
	nonce, err := randomHex(32)
	if err != nil {
		return launchPayload{}, err
	}
	state, err := randomHex(16)
	if err != nil {
		return launchPayload{}, err
	}

	claims := lti.LaunchClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    issuer,
			Subject:   "user-" + user.ExternalID,
			Audience:  jwt.ClaimStrings{clientID},
			ExpiresAt: jwt.NewNumericDate(now.Add(5 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
		AuthorizedParty: clientID,
		Nonce:           nonce,
		DeploymentID:    "emu-deployment-1",
		MessageType:     lti.MessageTypeResourceLink,
		Version:         lti.Version13,
		Roles:           []string{roleURI(user.Role)},
		Context: lti.Context{
			ID:    course.ExternalID,
			Label: course.ExternalID,
			Title: course.Title,
		},
		ResourceLink: lti.Resource{
			ID:    lab.Slug,
			Title: lab.Title,
		},
		Custom: lti.Custom{
			LabTemplateSlug: lab.Slug,
		},
		Email: user.ExternalID + "@emulator.local",
		Name:  user.Name,
	}
	if err := claims.ValidateResourceLink(); err != nil {
		return launchPayload{}, err
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = s.kid
	signed, err := token.SignedString(s.key)
	if err != nil {
		return launchPayload{}, fmt.Errorf("moodle-emulator: sign launch: %w", err)
	}
	return launchPayload{IDToken: signed, State: state, Nonce: nonce}, nil
}

func roleURI(role string) string {
	if role == "Instructor" {
		return lti.RoleInstructor
	}
	return lti.RoleLearner
}

func randomHex(bytes int) (string, error) {
	buf := make([]byte, bytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("moodle-emulator: random bytes: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
