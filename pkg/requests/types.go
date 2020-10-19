package requests

// requestException represents a single release exception to a request.
type requestException struct {
	Version string `yaml:"releaseVersion" json:"releaseVersion"`
	Reason  string `yaml:"reason"`
}

// versionRequest represents a specific requested component name and version.
type versionRequest struct {
	Issue      string             `yaml:"issue"`
	Name       string             `yaml:"name"`
	Version    string             `yaml:"version"`
	Exceptions []requestException `yaml:"except,omitempty" json:"except,omitempty"`
}

// releaseRequest is one release pattern with associated requests.
type releaseRequest struct {
	Name     string           `yaml:"name"`
	Requests []versionRequest `yaml:"requests"`
}

type requestsFile struct {
	Releases []releaseRequest `yaml:"releases"`
}

