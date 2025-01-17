package types_test

import (
	"fmt"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"

	"github.com/flexkube/libflexkube/internal/util"
	"github.com/flexkube/libflexkube/internal/utiltest"
	"github.com/flexkube/libflexkube/pkg/types"
)

func TestCertificateParse(t *testing.T) {
	t.Parallel()

	type Foo struct {
		Bar types.Certificate `json:"bar"`
	}

	cases := map[string]struct {
		YAML  string
		Error bool
	}{
		"bad": {
			"bar: doh",
			true,
		},
		"good": {
			fmt.Sprintf("bar: |\n%s", util.Indent(strings.TrimSpace(utiltest.GenerateX509Certificate(t)), "  ")),
			false,
		},
	}

	for n, testCase := range cases {
		testCase := testCase

		t.Run(n, func(t *testing.T) {
			t.Parallel()

			bar := &Foo{}

			err := yaml.Unmarshal([]byte(testCase.YAML), bar)

			if testCase.Error && err == nil {
				t.Fatalf("Expected error and didn't get any.")
			}

			if !testCase.Error && err != nil {
				t.Fatalf("Didn't expect error, got: %v", err)
			}

			if err == nil && bar.Bar == "" {
				t.Fatalf("Didn't get any error, but field is empty")
			}

			if err != nil && bar.Bar != "" {
				t.Fatalf("Got error and still got some content")
			}
		})
	}
}

func TestCertificatePickNil(t *testing.T) {
	t.Parallel()

	var c types.Certificate

	if c.Pick(types.Certificate("bar"), types.Certificate("baz")) != "bar" {
		t.Fatalf("First non empty certificate should be picked")
	}
}

func TestCertificatePick(t *testing.T) {
	t.Parallel()

	d := types.Certificate("foo")

	if d.Pick(types.Certificate("baz")) != "foo" {
		t.Fatalf("First non empty certificate should be picked")
	}
}
