// Package content defines the types for user-uploaded tools and scripts
// that the LLM agent can invoke.
package content

// ThirdPartyAPI describes an external HTTP API endpoint that can be called by the LLM agent.
// Users upload these definitions as YAML files.
type ThirdPartyAPI struct {
	Endpoint                string `json:"endpoint" yaml:"endpoint"`
	Description             string `json:"description" yaml:"description"`
	WaitForUserConfirm      bool   `json:"waitForUserConfirm" yaml:"waitForUserConfirm"`
	NeedClientProvideAPIKey bool   `json:"needClientProvideApiKey" yaml:"needClientProvideApiKey"`
	// Request describes the expected request format (natural language or schema).
	Request string `json:"request" yaml:"request"`
	// Response describes the expected response format (natural language or schema).
	Response string `json:"response" yaml:"response"`
}

// Script describes a user-uploaded executable script (e.g. Python) that the LLM agent can run.
type Script struct {
	Name        string `json:"name" yaml:"name"`
	Path        string `json:"path" yaml:"path"`
	Description string `json:"description" yaml:"description"`
	Language    string `json:"language" yaml:"language"` // e.g. "python"
	IsPublic    bool   `json:"isPublic" yaml:"isPublic"`
}
