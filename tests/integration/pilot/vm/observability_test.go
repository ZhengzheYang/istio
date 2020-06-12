package vm

import (
	"fmt"
	"istio.io/istio/pkg/test/framework/components/prometheus"
	"istio.io/istio/pkg/test/util/retry"
	"testing"
	"time"

	"istio.io/istio/pkg/test/framework"
	"istio.io/istio/pkg/test/framework/components/echo"
	"istio.io/istio/pkg/test/framework/components/echo/echoboot"
	"istio.io/istio/pkg/test/framework/components/namespace"
)

func TestVmMetrics(t *testing.T) {
	framework.
		NewTest(t).
		Features("observability.telemetry.stats.prometheus.http.nullvm").
		Run(func(ctx framework.TestContext) {
			ns = namespace.NewOrFail(t, ctx, namespace.Config{
				Prefix: "observability",
				Inject: true,
			})
			var client, vm echo.Instance
			echoboot.NewBuilderOrFail(t, ctx).
				With(&client, echoConfig(ns, "client", false)).
				With(&vm, echoConfig(ns, "vm", true)).
				BuildOrFail(t)
			promInst, err := prometheus.New(ctx, prometheus.Config{})
			if err != nil {
				t.Fatal(err)
			}
			//sourceQuery, destinationQuery, appQuery := buildQuery()
			query := buildQuery()
			retry.UntilSuccessOrFail(t, func() error {
				if _, err := client.Call(echo.CallOptions{
					Target:   vm,
					PortName: "http",
				}); err != nil {
					return err
				}
				// Query client side metrics
				//if err := promUtil.QueryPrometheus(t, query, promInst); err != nil {
				//	t.Logf("prometheus values for istio_requests_total: \n%s", util.PromDump(promInst, "istio_requests_total"))
				//	return err
				//}
				t.Logf("query prometheus with: %v", query)
				val, err := promInst.WaitForQuiesce(query)
				if err != nil {
					return err
				}
				got, err := promInst.Sum(val, nil)
				if err != nil {
					t.Logf("value: %s", val.String())
					return fmt.Errorf("could not find metric value: %v", err)
				}
				t.Logf("get value %v", got)
				if got != 1 {
					return fmt.Errorf("expect metric value: %v, got %v", 1, got)
				}
				//if err := promUtil.QueryPrometheus(t, destinationQuery, promInst); err != nil {
				//	t.Logf("prometheus values for istio_requests_total: \n%s", util.PromDump(promInst, "istio_requests_total"))
				//	return err
				//}
				//// This query will continue to increase due to readiness probe; don't wait for it to converge
				//if err := promUtil.QueryFirstPrometheus(t, appQuery, promInst); err != nil {
				//	t.Logf("prometheus values for istio_echo_http_requests_total: \n%s", util.PromDump(promInst, "istio_echo_http_requests_total"))
				//	return err
				//}
				return nil
			}, retry.Delay(3*time.Second), retry.Timeout(80*time.Second))
		})
}

func buildQuery() (query string) {
	query = `istio_requests_total{reporter="source",`
	//destinationQuery = `istio_requests_total{reporter="destination",`
	//labels := map[string]string{
	//	"request_protocol":               "http",
	//	"response_code":                  "200",
	//	"destination_app":                "vm",
	//	"destination_version":            "unknown",
	//	"destination_service":            "vm." + ns.Name() + ".svc.cluster.local",
	//	"destination_service_name":       "vm",
	//	"destination_workload_namespace": ns.Name(),
	//	"destination_service_namespace":  ns.Name(),
	//	"source_app":                     "client",
	//	"source_version":                 "v1",
	//	"source_workload":                "client-v1",
	//	"source_workload_namespace":      ns.Name(),
	//}
	labels := map[string]string{
		"request_protocol":               "http",
		"response_code":                  "200",
		"source_app":                     "client",
		"source_version":                 "v1",
		"source_workload":                "client-v1",
		"source_workload_namespace":      ns.Name(),
	}
	for k, v := range labels {
		query += fmt.Sprintf(`%s=%q,`, k, v)
	}
	query += "}"
	return
}
