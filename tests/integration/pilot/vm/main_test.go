package vm

import (
	"testing"

	"istio.io/istio/pkg/test/framework"
	"istio.io/istio/pkg/test/framework/components/istio"
	"istio.io/istio/pkg/test/framework/components/namespace"
	"istio.io/istio/pkg/test/framework/components/pilot"
	"istio.io/istio/pkg/test/framework/resource"
	"istio.io/istio/pkg/test/framework/resource/environment"
)

var p pilot.Instance
var ns namespace.Instance

// This tests VM mesh expansion. Rather than deal with the infra to get a real VM, we will use a pod
// with no Service, no DNS, no service account, etc to simulate a VM.
func TestMain(m *testing.M) {
	framework.
		NewSuite("vm_test", m).
		RequireSingleCluster().
		RequireEnvironment(environment.Kube).
		SetupOnEnv(environment.Kube, istio.Setup(nil, func(cfg *istio.Config) {
			cfg.ControlPlaneValues = `
values:
  global:
    meshExpansion:
      enabled: true`
		})).
		Setup(func(ctx resource.Context) (err error) {
			if p, err = pilot.New(ctx, pilot.Config{}); err != nil {
				return err
			}
			return nil
		}).
		Run()
}
