package controller_test

import (
	"context"
	"testing"

	extensionsv1alpha1 "github.com/kplane-dev/extensions/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestEnabledExtension_PlatformExtensionRefResolves(t *testing.T) {
	ctx := context.Background()
	s := testScheme(t)

	pe := &extensionsv1alpha1.PlatformExtension{
		ObjectMeta: metav1.ObjectMeta{Name: "exe-dev"},
		Spec: extensionsv1alpha1.PlatformExtensionSpec{
			DisplayName: "exe.dev",
			CRDs:        []string{"https://example.com/exevms.yaml"},
		},
	}
	ee := &extensionsv1alpha1.EnabledExtension{
		ObjectMeta: metav1.ObjectMeta{Name: "exe-dev", Namespace: "default"},
		Spec: extensionsv1alpha1.EnabledExtensionSpec{
			ExtensionRef:  extensionsv1alpha1.ExtensionRef{Kind: "PlatformExtension", Name: "exe-dev"},
			ControlPlanes: []string{"test"},
		},
	}

	c := fake.NewClientBuilder().WithScheme(s).WithObjects(pe, ee).Build()

	// Confirm the VCP client sees both objects — prerequisite for reconciler.
	var gotPE extensionsv1alpha1.PlatformExtension
	if err := c.Get(ctx, types.NamespacedName{Name: "exe-dev"}, &gotPE); err != nil {
		t.Fatalf("get PlatformExtension: %v", err)
	}
	if len(gotPE.Spec.CRDs) != 1 || gotPE.Spec.CRDs[0] != "https://example.com/exevms.yaml" {
		t.Errorf("CRDs = %v, want [https://example.com/exevms.yaml]", gotPE.Spec.CRDs)
	}
}

func TestEnabledExtension_UserExtensionRefResolves(t *testing.T) {
	ctx := context.Background()
	s := testScheme(t)

	ext := &extensionsv1alpha1.Extension{
		ObjectMeta: metav1.ObjectMeta{Name: "my-ext", Namespace: "default"},
		Spec: extensionsv1alpha1.ExtensionSpec{
			DisplayName: "My Extension",
			CRDs:        []string{"https://example.com/my-crd.yaml"},
		},
	}
	ee := &extensionsv1alpha1.EnabledExtension{
		ObjectMeta: metav1.ObjectMeta{Name: "my-ext", Namespace: "default"},
		Spec: extensionsv1alpha1.EnabledExtensionSpec{
			ExtensionRef:  extensionsv1alpha1.ExtensionRef{Kind: "Extension", Name: "my-ext"},
			ControlPlanes: []string{"*"},
		},
	}

	c := fake.NewClientBuilder().WithScheme(s).WithObjects(ext, ee).Build()

	var got extensionsv1alpha1.Extension
	if err := c.Get(ctx, types.NamespacedName{Name: "my-ext", Namespace: "default"}, &got); err != nil {
		t.Fatalf("get Extension: %v", err)
	}
	if got.Spec.CRDs[0] != "https://example.com/my-crd.yaml" {
		t.Errorf("CRD = %q, want https://example.com/my-crd.yaml", got.Spec.CRDs[0])
	}
}

func TestEnabledExtension_WildcardControlPlanes(t *testing.T) {
	ee := &extensionsv1alpha1.EnabledExtension{
		Spec: extensionsv1alpha1.EnabledExtensionSpec{
			ControlPlanes: []string{"*"},
		},
	}
	if len(ee.Spec.ControlPlanes) != 1 || ee.Spec.ControlPlanes[0] != "*" {
		t.Errorf("wildcard not set correctly: %v", ee.Spec.ControlPlanes)
	}
}

func TestEnabledExtension_SpecificControlPlanes(t *testing.T) {
	s := testScheme(t)
	ctx := context.Background()

	ee := &extensionsv1alpha1.EnabledExtension{
		ObjectMeta: metav1.ObjectMeta{Name: "ee", Namespace: "default"},
		Spec: extensionsv1alpha1.EnabledExtensionSpec{
			ExtensionRef:  extensionsv1alpha1.ExtensionRef{Kind: "PlatformExtension", Name: "exe-dev"},
			ControlPlanes: []string{"staging", "production"},
		},
	}

	c := fake.NewClientBuilder().WithScheme(s).WithObjects(ee).Build()

	var got extensionsv1alpha1.EnabledExtension
	if err := c.Get(ctx, types.NamespacedName{Name: "ee", Namespace: "default"}, &got); err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(got.Spec.ControlPlanes) != 2 {
		t.Errorf("ControlPlanes = %v, want [staging production]", got.Spec.ControlPlanes)
	}
}
