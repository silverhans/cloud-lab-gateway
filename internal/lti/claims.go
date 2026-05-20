// Package lti contains shared LTI 1.3 claim schemas.
package lti

import (
	"errors"

	"github.com/golang-jwt/jwt/v5"
)

const (
	ClaimDeploymentID = "https://purl.imsglobal.org/spec/lti/claim/deployment_id"
	ClaimMessageType  = "https://purl.imsglobal.org/spec/lti/claim/message_type"
	ClaimVersion      = "https://purl.imsglobal.org/spec/lti/claim/version"
	ClaimRoles        = "https://purl.imsglobal.org/spec/lti/claim/roles"
	ClaimContext      = "https://purl.imsglobal.org/spec/lti/claim/context"
	ClaimResourceLink = "https://purl.imsglobal.org/spec/lti/claim/resource_link"
	ClaimCustom       = "https://purl.imsglobal.org/spec/lti/claim/custom"

	MessageTypeResourceLink = "LtiResourceLinkRequest"
	Version13               = "1.3.0"

	RoleLearner    = "http://purl.imsglobal.org/vocab/lis/v2/membership#Learner"
	RoleInstructor = "http://purl.imsglobal.org/vocab/lis/v2/membership#Instructor"
)

// LaunchClaims is the subset of LTI 1.3 Resource Link Launch claims shared by
// the emulator and the future gateway-side verifier.
type LaunchClaims struct {
	jwt.RegisteredClaims

	AuthorizedParty string   `json:"azp,omitempty"`
	Nonce           string   `json:"nonce"`
	DeploymentID    string   `json:"https://purl.imsglobal.org/spec/lti/claim/deployment_id"`
	MessageType     string   `json:"https://purl.imsglobal.org/spec/lti/claim/message_type"`
	Version         string   `json:"https://purl.imsglobal.org/spec/lti/claim/version"`
	Roles           []string `json:"https://purl.imsglobal.org/spec/lti/claim/roles"`
	Context         Context  `json:"https://purl.imsglobal.org/spec/lti/claim/context"`
	ResourceLink    Resource `json:"https://purl.imsglobal.org/spec/lti/claim/resource_link"`
	Custom          Custom   `json:"https://purl.imsglobal.org/spec/lti/claim/custom"`
	Email           string   `json:"email,omitempty"`
	Name            string   `json:"name,omitempty"`
}

// Context identifies the LMS course/context for a launch.
type Context struct {
	ID    string `json:"id"`
	Label string `json:"label,omitempty"`
	Title string `json:"title,omitempty"`
}

// Resource identifies the launched LMS resource link.
type Resource struct {
	ID    string `json:"id"`
	Title string `json:"title,omitempty"`
}

// Custom carries deployment-specific launch metadata.
type Custom struct {
	LabTemplateSlug string `json:"lab_template_slug,omitempty"`
}

// ValidateResourceLink checks required LTI 1.3 Resource Link fields. Signature,
// expiry, nonce replay and audience verification belong to the adapter verifier.
func (c LaunchClaims) ValidateResourceLink() error {
	switch {
	case c.Issuer == "":
		return errors.New("lti: issuer is required")
	case c.Subject == "":
		return errors.New("lti: subject is required")
	case len(c.Audience) == 0:
		return errors.New("lti: audience is required")
	case c.Nonce == "":
		return errors.New("lti: nonce is required")
	case c.DeploymentID == "":
		return errors.New("lti: deployment id is required")
	case c.MessageType != MessageTypeResourceLink:
		return errors.New("lti: unsupported message type")
	case c.Version != Version13:
		return errors.New("lti: unsupported version")
	case len(c.Roles) == 0:
		return errors.New("lti: at least one role is required")
	case c.Context.ID == "":
		return errors.New("lti: context id is required")
	case c.ResourceLink.ID == "":
		return errors.New("lti: resource link id is required")
	default:
		return nil
	}
}
