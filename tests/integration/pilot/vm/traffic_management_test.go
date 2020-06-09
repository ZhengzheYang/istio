package vm

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"istio.io/istio/pkg/test/echo/client"

	"istio.io/istio/pkg/config/protocol"
	"istio.io/istio/pkg/test/framework"
	"istio.io/istio/pkg/test/framework/components/echo"
	"istio.io/istio/pkg/test/framework/components/echo/echoboot"
	"istio.io/istio/pkg/test/framework/components/namespace"
	"istio.io/istio/pkg/test/util/file"
	"istio.io/istio/pkg/test/util/retry"
	"istio.io/istio/pkg/test/util/tmpl"
)

const (
	// Error threshold. For example, we expect 25% traffic, traffic distribution within [15%, 35%] is accepted.
	errorThreshold = 10.0
)

type VirtualServiceConfig struct {
	Name      string
	Host0     string
	Host1     string
	Host2     string
	Namespace string
	Weight0   int32
	Weight1   int32
	Weight2   int32
}

type DestinationRuleConfig struct {
	Name      string
	Host      string
	Namespace string
}

func TestVmTrafficManagement(t *testing.T) {
	framework.
		NewTest(t).
		Run(func(ctx framework.TestContext) {
			ns := namespace.NewOrFail(t, ctx, namespace.Config{
				Prefix: "traffic-management",
				Inject: true,
			})

			// build 4 instances with `a` as non-vm workload,
			// and `b`, `c`, `d`, `e` as vm-workload
			var instances [5]echo.Instance
			echoboot.NewBuilderOrFail(t, ctx).
				With(&instances[0], echoConfig(ns, "a", false)).
				With(&instances[1], echoConfig(ns, "b", true)).
				With(&instances[2], echoConfig(ns, "c", true)).
				With(&instances[3], echoConfig(ns, "d", true)).
				With(&instances[4], echoConfig(ns, "e", true)).
				BuildOrFail(t)

			//	Virtual service topology
			//
			//						 a
			//						|-------|
			//						| Host0 |
			//						|-------|
			//							|
			//							|
			//							|
			//		-------------------------------
			//		|weight1	  |weight2	      |weight3
			//		|b			  |c			  |d
			//	|-----------|	|-----------|	|-----------|
			//	| Host0(vm) |	| Host1(vm)	|	| Host2(vm) |
			//	|-----------|	|-----------|	|-----------|
			//
			//
			weights := map[string][]int32{
				"20-80":    {20, 80},
				"50-50":    {50, 50},
				"33-33-34": {33, 33, 34},
			}
			hosts := []string{"b", "c", "d"}
			for k, v := range weights {
				ctx.NewSubTest(k).
					Run(func(ctx framework.TestContext) {
						v = append(v, make([]int32, 3-len(v))...)

						vsc := VirtualServiceConfig{
							"traffic-shifting-rule",
							hosts[0],
							hosts[1],
							hosts[2],
							ns.Name(),
							v[0],
							v[1],
							v[2],
						}

						deployment := tmpl.EvaluateOrFail(t, file.AsStringOrFail(t, "testdata/traffic-shifting.yaml"), vsc)
						ctx.ApplyConfigOrFail(t, ns.Name(), deployment)

						sendTrafficAndCheckDistribution(t, 100, instances[0], instances[1], hosts, v, errorThreshold)
					})
			}

			ctx.NewSubTest("DestinationRuleTest").
				Run(func(ctx framework.TestContext) {
					drc := DestinationRuleConfig{
						"circuit-breaking-rule",
						"e",
						ns.Name(),
					}

					deployment := tmpl.EvaluateOrFail(t, file.AsStringOrFail(t, "testdata/circuit-breaking.yaml"), drc)
					ctx.ApplyConfigOrFail(t, ns.Name(), deployment)

				 	// send requests and expect Code 503
					sendTrafficAndCheck503(t, 100, instances[0], instances[4])
				})
		})
}

func echoConfig(ns namespace.Instance, name string, vm bool) echo.Config {
	return echo.Config{
		Service:   name,
		Namespace: ns,
		Ports: []echo.Port{
			{
				Name:     "http",
				Protocol: protocol.HTTP,
				// We use a port > 1024 to not require root
				InstancePort: 8090,
				ServicePort:  8090,
			},
		},
		Subsets:    []echo.SubsetConfig{{}},
		Pilot:      p,
		DeployAsVM: vm,
	}
}

func sendTraffic(t *testing.T, batchSize int, from, to echo.Instance) client.ParsedResponses {
	t.Helper()
	// Send `batchSize` requests and return the response from the request
	var response client.ParsedResponses
	retry.UntilSuccessOrFail(t, func() error {
		resp, err := from.Call(echo.CallOptions{
			Target:   to,
			PortName: "http",
			Count:    batchSize,
		})
		if err != nil {
			return fmt.Errorf("error during call: %v", err)
		}
		response = resp
		return nil
	}, retry.Delay(time.Second))
	return response
}

func sendTrafficAndCheckDistribution(t *testing.T, batchSize int, from, to echo.Instance, hosts []string, weight []int32, errorThreshold float64) {
	t.Helper()
	// Send `batchSize` requests and ensure they are distributed as expected.
	retry.UntilSuccessOrFail(t, func() error {
		resp := sendTraffic(t, batchSize, from, to)
		var totalRequests int
		hitCount := map[string]int{}
		for _, r := range resp {
			for _, h := range hosts {
				if strings.HasPrefix(r.Hostname, h+"-") {
					hitCount[h]++
					totalRequests++
					break
				}
			}
		}

		for i, v := range hosts {
			percentOfTrafficToHost := float64(hitCount[v]) * 100.0 / float64(totalRequests)
			deltaFromExpected := math.Abs(float64(weight[i]) - percentOfTrafficToHost)
			if errorThreshold-deltaFromExpected < 0 {
				return fmt.Errorf("unexpected traffic weight for host %v. Expected %d%%, got %g%% (thresold: %g%%)",
					v, weight[i], percentOfTrafficToHost, errorThreshold)
			}
			t.Logf("Got expected traffic weight for host %v. Expected %d%%, got %g%% (thresold: %g%%)",
				v, weight[i], percentOfTrafficToHost, errorThreshold)
		}
		return nil
	}, retry.Delay(time.Second))
}

func sendTrafficAndCheck503(t *testing.T, batchSize int, from, to echo.Instance) {
	t.Helper()
	// Send `batchSize` requests and ensure they are distributed as expected.
	retry.UntilSuccessOrFail(t, func() error {
		resp := sendTraffic(t, batchSize, from, to)
		var expectError bool
		for _, r := range resp {
			if r.Code == "503" {
				expectError = true
				break
			}
		}
		if expectError {
			return nil
		} else {
			return fmt.Errorf("expect 503 error but none found")
		}
	}, retry.Delay(time.Second))
}