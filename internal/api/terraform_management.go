package api

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

const TerraformManagementHeader = "X-RWX-Terraform-Management"

func (c Client) ClaimTerraformManagement(ctx context.Context, endpoint string) error {
	target, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("invalid Terraform management endpoint: %w", err)
	}
	target.RawPath = target.EscapedPath() + "/terraform_management"
	target.Path += "/terraform_management"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target.String(), nil)
	if err != nil {
		return fmt.Errorf("unable to create HTTP request: %w", err)
	}
	resp, err := c.RoundTrip(req)
	if err != nil {
		return fmt.Errorf("terraform management claim failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("terraform management claim failed: %w", responseError(resp))
	}
	return nil
}
