package api

import "fmt"

type Config struct {
	AccessToken string
	Host        string
	Version     string
	Pulumi      bool
}

func (c Config) Validate() error {
	if c.AccessToken == "" {
		return fmt.Errorf("missing access token")
	}

	if c.Host == "" {
		return fmt.Errorf("missing host")
	}

	if c.Version == "" {
		return fmt.Errorf("missing version")
	}

	return nil
}
