package controller

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
)

// RecorderCache memoizes one EventBroadcaster/Recorder per target apiserver
// (keyed by a stable fingerprint of the rest.Config) so we don't rebuild them
// on every reconcile.
type RecorderCache struct {
	scheme    *runtime.Scheme
	component string

	mu    sync.Mutex
	byKey map[string]*recorderEntry
}

type recorderEntry struct {
	recorder    record.EventRecorder
	broadcaster record.EventBroadcaster
}

func NewRecorderCache(scheme *runtime.Scheme, component string) *RecorderCache {
	return &RecorderCache{
		scheme:    scheme,
		component: component,
		byKey:     map[string]*recorderEntry{},
	}
}

func (c *RecorderCache) RecorderFor(cfg *rest.Config) (record.EventRecorder, error) {
	key := fingerprint(cfg)

	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.byKey[key]; ok {
		return e.recorder, nil
	}

	kc, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	bc := record.NewBroadcaster()
	bc.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kc.CoreV1().Events("")})
	rec := bc.NewRecorder(c.scheme, corev1.EventSource{Component: c.component})

	c.byKey[key] = &recorderEntry{recorder: rec, broadcaster: bc}
	return rec, nil
}

func (c *RecorderCache) Shutdown() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, e := range c.byKey {
		e.broadcaster.Shutdown()
	}
	c.byKey = nil
}

func fingerprint(cfg *rest.Config) string {
	h := sha256.New()
	h.Write([]byte(cfg.Host))
	h.Write([]byte(cfg.BearerToken))
	h.Write(cfg.CAData)
	h.Write(cfg.CertData)
	h.Write(cfg.KeyData)
	return hex.EncodeToString(h.Sum(nil))
}
