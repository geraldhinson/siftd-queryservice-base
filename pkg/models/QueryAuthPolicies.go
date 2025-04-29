package models

import "github.com/geraldhinson/siftd-base/pkg/security"

type QueryFileAuthPolicies struct {
	Realm        string
	AuthType     security.AuthTypes
	Timeout      security.AuthTimeout
	ApprovedList []string
}

// this is a mapping between the authRequired string in the queries files and the actual authModel
// policy (inputs) that the author of this query service wants to use for any query tagged with that string
// in the queries files.
// This mapping is typically defined in the "main" package of a query service.
type QueryFileAuthPoliciesList map[string]QueryFileAuthPolicies
