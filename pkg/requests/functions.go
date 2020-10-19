package requests

import (
	"fmt"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/giantswarm/apiextensions/v2/pkg/apis/release/v1alpha1"
	"github.com/giantswarm/microerror"
	"sigs.k8s.io/yaml"
)

type Requests struct {
	requests []releaseRequest
}

func (r Requests) Load(data []byte) error {
	var file requestsFile
	err := yaml.UnmarshalStrict(data, &file)
	if err != nil {
		return microerror.Mask(err)
	}
	r.requests = file.Releases
	return nil
}

func (r Requests) Check(release v1alpha1.Release) error {
	// Check that all active releases contain all requested component versions.
	if release.Spec.State == "active" {
		requests, err := findMatchingRequests(release.Name, r.requests)
		if err != nil {
			return microerror.Mask(err)
		}

		var unsatisfiedRequests []string
		for _, request := range requests {
			componentsSatisfied, actualComponentVersion, err := componentListSatisfiesRequest(request, release.Spec.Components)
			if err != nil {
				return microerror.Mask(err)
			}

			appsSatisfied, actualAppVersion, err := appListSatisfiesRequest(request, release.Spec.Apps)
			if err != nil {
				return microerror.Mask(err)
			}

			if !componentsSatisfied && !appsSatisfied {
				// Either components or apps were not satisfied. Use the 'actual' version which isn't empty.
				actual := actualComponentVersion
				if actual == "" {
					actual = actualAppVersion
				}

				unsatisfied := fmt.Sprintf("requested: %s: %s \tactual: %s", request.Name, request.Version, actual)
				unsatisfiedRequests = append(unsatisfiedRequests, unsatisfied)
			}
		}

		if len(unsatisfiedRequests) > 0 {
			msg := fmt.Sprintf("Release %s does not meet the requested version requirements:\n%s", release.Name, strings.Join(unsatisfiedRequests, ",\n"))
			return microerror.Mask(fmt.Errorf(msg))
		}
	}

	return nil
}

// appListSatisfiesRequest determines whether the given request is satisfied in the given app list.
// It returns a boolean value for whether the request is satisfied as well as
// a string containing the actual app version which satisfies the request.
func appListSatisfiesRequest(request versionRequest, appList []v1alpha1.ReleaseSpecApp) (bool, string, error) {
	var actual string
	for _, app := range appList {
		if app.Name == request.Name {
			actual = app.Version
			actualMatchesRequested, err := versionMatches(actual, request.Version)
			if err != nil {
				return false, actual, microerror.Mask(err)
			}

			if actualMatchesRequested {
				return true, actual, nil
			}

			break // No need to keep searching for this component.
		}
	}
	return false, actual, nil
}

// componentListSatisfiesRequest determines whether the given request is satisfied in the given component list.
// It returns a boolean value for whether the request is satisfied as well as
// a string containing the actual component version which satisfies the request.
func componentListSatisfiesRequest(request versionRequest, componentList []v1alpha1.ReleaseSpecComponent) (bool, string, error) {
	var actual string
	for _, component := range componentList {
		if component.Name == request.Name {
			actual = component.Version
			actualMatchesRequested, err := versionMatches(actual, request.Version)
			if err != nil {
				return false, actual, microerror.Mask(err)
			}

			if actualMatchesRequested {
				return true, actual, nil
			}

			break // No need to keep searching for this component.
		}
	}
	return false, actual, nil
}

// findMatchingRequests searches the given array of releaseRequests
// for requests which apply to the given release version.
func findMatchingRequests(release string, requests []releaseRequest) ([]versionRequest, error) {
	var requestList []versionRequest
	for _, request := range requests {

		// See whether this request applies to the current release version.
		match, err := versionMatches(release, request.Name)
		if err != nil {
			return nil, microerror.Mask(err)
		}

		if match {
			for _, component := range request.Requests {
				releaseIsExcluded := false
				if component.Exceptions != nil {
					// Check the excluded releases for this component to see if our release is there.
					for _, e := range component.Exceptions {
						releaseIsExcluded, err = versionMatches(e.Version, request.Name)
						if err != nil {
							return nil, microerror.Mask(err)
						}
					}
				}

				if !releaseIsExcluded {
					requestList = append(requestList, component)
				}
			}
		}
	}
	return requestList, nil
}

// versionMatches compares the given version with the given semver
// constraint pattern and returns whether it matches.
func versionMatches(version string, pattern string) (bool, error) {
	c, err := semver.NewConstraint(pattern)
	if err != nil {
		return false, fmt.Errorf("release names for requests must be valid semver constraints: %s", err)
	}

	v, err := semver.NewVersion(version)
	if err != nil {
		return false, fmt.Errorf("release names must be valid semver: %s: %s", err, version)
	}

	return c.Check(v), nil
}
