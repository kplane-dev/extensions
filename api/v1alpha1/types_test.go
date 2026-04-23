package v1alpha1_test

import (
	"testing"

	"github.com/kplane-dev/extensions/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestSchemeRegistration(t *testing.T) {
	s := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}

	wantGV := schema.GroupVersion{Group: "extensions.kplane.dev", Version: "v1alpha1"}

	for _, kind := range []string{"PlatformExtension", "PlatformExtensionList", "Extension", "ExtensionList", "EnabledExtension", "EnabledExtensionList"} {
		gvk := wantGV.WithKind(kind)
		if !s.Recognizes(gvk) {
			t.Errorf("scheme does not recognize %s", gvk)
		}
	}
}

func TestPlatformExtensionDeepCopy(t *testing.T) {
	pe := &v1alpha1.PlatformExtension{
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
		Spec: v1alpha1.PlatformExtensionSpec{
			DisplayName: "Test",
			Tags:        []string{"a", "b"},
			CRDs:        []string{"https://example.com/crd.yaml"},
		},
	}

	copy := pe.DeepCopy()
	copy.Spec.Tags[0] = "mutated"
	copy.Spec.CRDs[0] = "https://mutated.example.com/crd.yaml"

	if pe.Spec.Tags[0] != "a" {
		t.Error("DeepCopy did not isolate Tags slice")
	}
	if pe.Spec.CRDs[0] != "https://example.com/crd.yaml" {
		t.Error("DeepCopy did not isolate CRDs slice")
	}
}

func TestEnabledExtensionDeepCopy(t *testing.T) {
	ee := &v1alpha1.EnabledExtension{
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
		Spec: v1alpha1.EnabledExtensionSpec{
			ExtensionRef:  v1alpha1.ExtensionRef{Kind: "PlatformExtension", Name: "exe-dev"},
			ControlPlanes: []string{"staging", "production"},
		},
	}

	copy := ee.DeepCopy()
	copy.Spec.ControlPlanes[0] = "mutated"

	if ee.Spec.ControlPlanes[0] != "staging" {
		t.Error("DeepCopy did not isolate ControlPlanes slice")
	}
}
