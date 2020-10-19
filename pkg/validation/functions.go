package validation

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/giantswarm/apiextensions/v2/pkg/apis/release/v1alpha1"
	"github.com/giantswarm/microerror"
	"github.com/giantswarm/versionbundle"
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/validation"
	"sigs.k8s.io/yaml"

	"github.com/giantswarm/releaseclient/pkg/filesystem"
	"github.com/giantswarm/releaseclient/pkg/key"
	requests2 "github.com/giantswarm/releaseclient/pkg/requests"
)

// To reuse versionbundle.ValidateIndexReleases, the slice of Releases must first be
// converted into a slice of versionbundle.IndexRelease.
func releasesToIndex(releases []v1alpha1.Release) []versionbundle.IndexRelease {
	var indexReleases []versionbundle.IndexRelease
	for _, release := range releases {
		var apps []versionbundle.App
		for _, app := range release.Spec.Apps {
			indexApp := versionbundle.App{
				App:              app.Name,
				ComponentVersion: app.ComponentVersion,
				Version:          app.Version,
			}
			apps = append(apps, indexApp)
		}
		var authorities []versionbundle.Authority
		for _, component := range release.Spec.Components {
			indexAuthority := versionbundle.Authority{
				Name:    component.Name,
				Version: component.Version,
			}
			authorities = append(authorities, indexAuthority)
		}
		indexRelease := versionbundle.IndexRelease{
			Active:      release.Spec.State == "active",
			Apps:        apps,
			Authorities: authorities,
			Date:        release.Spec.Date.Time,
			Version:     release.Name,
		}
		indexReleases = append(indexReleases, indexRelease)
	}
	return indexReleases
}

func validateRequests(fs filesystem.Filesystem, provider string) error {
	requests := requests2.Requests{}

	{
		requestsData, err := fs.ReadFile(filepath.Join(provider, key.RequestsFilename))
		if err != nil {
			return microerror.Mask(err)
		}

		err = requests.Load(requestsData)
		if err != nil {
			return microerror.Mask(err)
		}
	}

	releases, err := fs.FindReleases(provider, false)
	if err != nil {
		return microerror.Mask(err)
	}

	for _, release := range releases {
		err = requests.Check(release)
	}

	return nil
}

func validateReleaseNotes(fs filesystem.Filesystem, provider string) error {
	releases, err := fs.FindReleases(provider, false)
	if err != nil {
		return microerror.Mask(err)
	}

	for _, release := range releases {
		// Check that the version in the first line of the release notes is correct.
		{
			releaseNotesData, err := fs.ReadFile(filepath.Join(provider, release.Name, key.ReadmeFilename))
			if err != nil {
				return microerror.Mask(fmt.Errorf("missing file for %s release %s: %s", provider, release.Name, err))
			}
			releaseNotesLines := strings.Split(string(releaseNotesData), "\n")
			if len(releaseNotesLines) == 0 || !strings.Contains(releaseNotesLines[0], strings.TrimPrefix(release.Name, "v")) {
				return microerror.Mask(fmt.Errorf("expected release notes for %s release %s to contain the release version on the first line", provider, release.Name))
			}
		}
	}

	return nil
}

func validateReadme(fs filesystem.Filesystem, provider string) error {
	// Load the README so we can check links for each release.
	var readmeContent string
	{
		readmeContentBytes, err := fs.ReadFile(key.ReadmeFilename)
		if err != nil {
			return microerror.Mask(err)
		}
		readmeContent = string(readmeContentBytes)
	}

	releases, err := fs.FindReleases(provider, false)
	if err != nil {
		return microerror.Mask(err)
	}

	for _, release := range releases {
		// Check that the README links to the release.
		if !strings.Contains(readmeContent, fmt.Sprintf("https://github.com/giantswarm/releaseclient/tree/master/%s/%s", provider, release.Name)) {
			return microerror.Mask(fmt.Errorf("expected link in %s to %s release %s", key.ReadmeFilename, provider, release.Name))
		}
	}

	archived, err := fs.FindReleases(provider, true)
	if err != nil {
		return microerror.Mask(err)
	}

	for _, release := range archived {
		// Check that the README links to the release.
		if !strings.Contains(readmeContent, fmt.Sprintf("https://github.com/giantswarm/releases/tree/master/%s/archived/%s", provider, release.Name)) {
			return microerror.Mask(fmt.Errorf("expected link in %s to archived %s release %s", key.ReadmeFilename, provider, release.Name))
		}
	}

	return nil
}

func validateReleasesAgainstCRD(fs filesystem.Filesystem, provider string) error {
	releases, err := fs.FindReleases(provider, false)
	if err != nil {
		return microerror.Mask(err)
	}

	crd := v1alpha1.NewReleaseCRD()

	for _, crdVersion := range crd.Spec.Versions {
		var v apiextensions.CustomResourceValidation
		// Convert the CRD validation into the version-independent form.
		err := v1.Convert_v1_CustomResourceValidation_To_apiextensions_CustomResourceValidation(crdVersion.Schema, &v, nil)
		if err != nil {
			return microerror.Mask(err)
		}

		validator, _, err := validation.NewSchemaValidator(&v)
		if err != nil {
			return microerror.Mask(err)
		}

		for _, release := range releases {
			result := validator.Validate(release)
			if len(result.Errors) > 0 {
				message := fmt.Sprintf("invalid release: %#v\n", release)
				for i, err := range result.Errors {
					message += fmt.Sprintf("validation error %d: %#v\n", i, err)
				}
			}
		}
	}

	return nil
}

func validateVersionBundle(fs filesystem.Filesystem, provider string) error {
	releases, err := fs.FindReleases(provider, false)
	if err != nil {
		return microerror.Mask(err)
	}

	// Ensure that releases are unique.
	indexReleases := releasesToIndex(releases)
	err = versionbundle.ValidateIndexReleases(indexReleases)
	if err != nil {
		return microerror.Mask(err)
	}

	return nil
}

func validateKustomization(fs filesystem.Filesystem, provider string) error {
	releases, err := fs.FindReleases(provider, false)
	if err != nil {
		return microerror.Mask(err)
	}

	providerResources := map[string]bool{}
	{
		var providerKustomization kustomizationFile
		providerKustomizationData, err := fs.ReadFile(filepath.Join(provider, key.KustomizationFilename))
		if err != nil {
			return microerror.Mask(err)
		}
		err = yaml.UnmarshalStrict(providerKustomizationData, &providerKustomization)
		if err != nil {
			return microerror.Mask(err)
		}
		for _, resource := range providerKustomization.Resources {
			providerResources[resource] = false
		}
	}

	for _, release := range releases {
		// Check that the release is registered in the main provider kustomization.yaml resources.
		if _, ok := providerResources[release.Name]; !ok {
			return microerror.Mask(fmt.Errorf("release %s not registered in %s/%s", release.Name, provider, key.KustomizationFilename))
		}
		providerResources[release.Name] = true

		// Check that the release-specific kustomization.yaml file points to the release manifest.
		{
			releaseKustomizationData, err := fs.ReadFile(filepath.Join(provider, release.Name, key.KustomizationFilename))
			if err != nil {
				return microerror.Mask(fmt.Errorf("missing file for %s release %s: %s", provider, release.Name, err))
			}
			var releaseKustomization kustomizationFile
			err = yaml.UnmarshalStrict(releaseKustomizationData, &releaseKustomization)
			if len(releaseKustomization.Resources) != 1 || releaseKustomization.Resources[0] != key.ReleaseFilename {
				return microerror.Mask(fmt.Errorf("%s for %s release %s should contain only one resource, \"%s\"", key.KustomizationFilename, provider, release.Name, key.ReleaseFilename))
			}
		}
	}

	// Check for extra resources in provider kustomization.yaml that don't have a corresponding release.
	for release, processed := range providerResources {
		if !processed {
			return microerror.Mask(fmt.Errorf("release %s registered in %s/%s resources but not found", release, provider, key.KustomizationFilename))
		}
	}

	return nil
}

func Validate(fs filesystem.Filesystem, provider string) error {
	validations := []func(fs filesystem.Filesystem, provider string) error {
		validateRequests,
		validateReleaseNotes,
		validateReadme,
		validateReleasesAgainstCRD,
		validateVersionBundle,
		validateKustomization,
	}

	for _, v := range validations {
		err := v(fs, provider)
		if err != nil {
			return microerror.Mask(err)
		}
	}

	return nil
}
