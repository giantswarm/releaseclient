package validation

type kustomizationFile struct {
	CommonAnnotations map[string]string `yaml:"commonAnnotations"`
	Resources         []string          `yaml:"resources"`
	Transformers      []string          `yaml:"transformers"`
}
