package controller_test

import (
	"context"
	"testing"

	extensionsv1alpha1 "github.com/kplane-dev/extensions/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func testScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := extensionsv1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}
	return s
}

func TestPlatformExtensionRoundtrip(t *testing.T) {
	ctx := context.Background()
	s := testScheme(t)

	pe := &extensionsv1alpha1.PlatformExtension{
		ObjectMeta: metav1.ObjectMeta{Name: "exe-dev"},
		Spec: extensionsv1alpha1.PlatformExtensionSpec{
			DisplayName: "exe.dev",
			Description: "Linux VMs",
			Tags:        []string{"VMs", "Sandboxes"},
			CRDs:        []string{"https://example.com/exevms.yaml"},
		},
	}

	c := fake.NewClientBuilder().WithScheme(s).Build()

	if err := c.Create(ctx, pe); err != nil {
		t.Fatalf("create: %v", err)
	}

	var got extensionsv1alpha1.PlatformExtension
	if err := c.Get(ctx, types.NamespacedName{Name: "exe-dev"}, &got); err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Spec.DisplayName != "exe.dev" {
		t.Errorf("DisplayName = %q, want exe.dev", got.Spec.DisplayName)
	}
	if len(got.Spec.CRDs) != 1 {
		t.Errorf("CRDs len = %d, want 1", len(got.Spec.CRDs))
	}
}
