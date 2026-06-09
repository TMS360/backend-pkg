package relaypayments

import (
	"context"
	"net/http"
	"net/url"
)

// ListFuelPolicies returns every fuel policy configured for the organization.
// Policies are read-only via API and must be managed inside the Relay portal.
func (c *Client) ListFuelPolicies(ctx context.Context) ([]FuelPolicy, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, "/fuel/policies/", nil, nil)
	if err != nil {
		return nil, err
	}
	var out []FuelPolicy
	if err := decodeJSON(resp, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetFuelPolicy returns a single fuel policy by Relay external ID.
func (c *Client) GetFuelPolicy(ctx context.Context, id string) (*FuelPolicy, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, "/fuel/policies/"+url.PathEscape(id), nil, nil)
	if err != nil {
		return nil, err
	}
	var out FuelPolicy
	if err := decodeJSON(resp, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetPolicyAssignment returns the current fuel policy assignment for a driver,
// including remaining usage on each limit.
func (c *Client) GetPolicyAssignment(ctx context.Context, driverID string) (*PolicyAssignmentFuelPolicy, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, "/fuel/policies/policy-assignments/"+url.PathEscape(driverID), nil, nil)
	if err != nil {
		return nil, err
	}
	var out PolicyAssignmentFuelPolicy
	if err := decodeJSON(resp, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// AssignPolicy assigns a fuel policy to a driver. Per Relay docs, assignment
// resets usage counters to 0.
func (c *Client) AssignPolicy(ctx context.Context, assignment DriverPolicyAssignment) (*PolicyAssignmentFuelPolicy, error) {
	resp, err := c.doRequest(ctx, http.MethodPost, "/fuel/policies/policy-assignments/", nil, assignment)
	if err != nil {
		return nil, err
	}
	var out PolicyAssignmentFuelPolicy
	if err := decodeJSON(resp, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdatePolicyAssignment toggles a driver's policy assignment on or off
// without resetting usage amounts.
func (c *Client) UpdatePolicyAssignment(ctx context.Context, id string, body UpdatePolicyAssignment) (*PolicyAssignmentFuelPolicy, error) {
	resp, err := c.doRequest(ctx, http.MethodPut, "/fuel/policies/policy-assignments/"+url.PathEscape(id), nil, body)
	if err != nil {
		return nil, err
	}
	var out PolicyAssignmentFuelPolicy
	if err := decodeJSON(resp, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeletePolicyAssignment removes a driver's policy assignment; the driver will
// be unable to fuel with a policy code afterwards.
func (c *Client) DeletePolicyAssignment(ctx context.Context, id string) error {
	resp, err := c.doRequest(ctx, http.MethodDelete, "/fuel/policies/policy-assignments/"+url.PathEscape(id), nil, nil)
	if err != nil {
		return err
	}
	return decodeJSON(resp, nil)
}
